// Package rbac creates the managed ServiceAccount, (Cluster)Role and
// (Cluster)RoleBinding set as the invoking user, with reverse-order rollback on
// partial failure. Creation must never use a privileged context or impersonation
// (NFR-002, ADR-005). See FR-004, FR-005.
package rbac

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const rbacAPIGroup = "rbac.authorization.k8s.io"

// Spec describes the managed RBAC set for a single mint. Every object shares
// BaseName, Labels and Annotations. ClusterScoped selects (Cluster)Role/(Cluster)RoleBinding.
//
// Namespace is the ServiceAccount's home namespace. Namespaces lists the namespaces to place
// a Role+RoleBinding in (FR-017): the ONE ServiceAccount in Namespace is the subject of every
// binding, so a single minted token reaches each listed namespace. When Namespaces is empty
// it defaults to [Namespace] (the single-namespace case). Namespaces is ignored when
// ClusterScoped is set — a cluster-wide binding already spans every namespace.
type Spec struct {
	BaseName      string
	Namespace     string
	Namespaces    []string
	ClusterScoped bool
	Rules         []rbacv1.PolicyRule
	Labels        map[string]string
	Annotations   map[string]string
}

// Created records what Create made, enough for token minting and rollback. For a namespaced
// set BindingNamespaces lists every namespace a Role+RoleBinding was created in, so Rollback
// reverses each one; it is empty for a cluster-scoped set.
type Created struct {
	ServiceAccountName      string
	ServiceAccountNamespace string
	ClusterScoped           bool
	BindingNamespaces       []string
}

// Create makes the RBAC set in order (SA → (Cluster)Role → (Cluster)RoleBinding)
// using cs (the invoking user's clientset). On any failure it rolls the
// already-created objects back in reverse order and returns the original error.
func Create(ctx context.Context, cs kubernetes.Interface, spec Spec) (Created, error) {
	var undo []func()
	rollback := func() {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
	}

	objectMetaIn := func(namespace string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Name: spec.BaseName, Namespace: namespace, Labels: spec.Labels, Annotations: spec.Annotations}
	}

	sa := &corev1.ServiceAccount{ObjectMeta: objectMetaIn(spec.Namespace)}
	if _, err := cs.CoreV1().ServiceAccounts(spec.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		return Created{}, fmt.Errorf("creating service account %s/%s: %w", spec.Namespace, spec.BaseName, err)
	}
	undo = append(undo, func() { _ = cs.CoreV1().ServiceAccounts(spec.Namespace).Delete(ctx, spec.BaseName, foreground()) })

	// The ONE ServiceAccount is the subject of every binding, whether namespaced (one per
	// requested namespace, FR-017) or cluster-wide (one ClusterRoleBinding).
	subjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: spec.BaseName, Namespace: spec.Namespace}}

	var bindingNamespaces []string
	if spec.ClusterScoped {
		cr := &rbacv1.ClusterRole{ObjectMeta: objectMetaIn(""), Rules: spec.Rules}
		if _, err := cs.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{}); err != nil {
			rollback()
			return Created{}, fmt.Errorf("creating cluster role %s: %w", spec.BaseName, err)
		}
		undo = append(undo, func() { _ = cs.RbacV1().ClusterRoles().Delete(ctx, spec.BaseName, foreground()) })

		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: objectMetaIn(""),
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: "ClusterRole", Name: spec.BaseName},
			Subjects:   subjects,
		}
		if _, err := cs.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
			rollback()
			return Created{}, fmt.Errorf("creating cluster role binding %s: %w", spec.BaseName, err)
		}
	} else {
		// FR-017: a Role+RoleBinding per requested namespace, each bound to the single SA.
		// Defaults to the SA's own namespace for the ordinary single-namespace case.
		nsList := spec.Namespaces
		if len(nsList) == 0 {
			nsList = []string{spec.Namespace}
		}
		for _, ns := range nsList {
			role := &rbacv1.Role{ObjectMeta: objectMetaIn(ns), Rules: spec.Rules}
			if _, err := cs.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil {
				rollback()
				return Created{}, fmt.Errorf("creating role %s/%s: %w", ns, spec.BaseName, err)
			}
			undo = append(undo, func() { _ = cs.RbacV1().Roles(ns).Delete(ctx, spec.BaseName, foreground()) })

			rb := &rbacv1.RoleBinding{
				ObjectMeta: objectMetaIn(ns),
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: "Role", Name: spec.BaseName},
				Subjects:   subjects,
			}
			if _, err := cs.RbacV1().RoleBindings(ns).Create(ctx, rb, metav1.CreateOptions{}); err != nil {
				rollback()
				return Created{}, fmt.Errorf("creating role binding %s/%s: %w", ns, spec.BaseName, err)
			}
			undo = append(undo, func() { _ = cs.RbacV1().RoleBindings(ns).Delete(ctx, spec.BaseName, foreground()) })
			bindingNamespaces = append(bindingNamespaces, ns)
		}
	}

	return Created{
		ServiceAccountName:      spec.BaseName,
		ServiceAccountNamespace: spec.Namespace,
		ClusterScoped:           spec.ClusterScoped,
		BindingNamespaces:       bindingNamespaces,
	}, nil
}

// Rollback deletes a previously-created set in reverse order (binding → role → SA)
// with foreground propagation. Used by the orchestrator when a later step (token
// or kubeconfig) fails after a successful Create. NotFound is not an error.
func Rollback(ctx context.Context, cs kubernetes.Interface, c Created) error {
	var errs []error
	swallow := func(err error) {
		if err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, err)
		}
	}

	if c.ClusterScoped {
		swallow(cs.RbacV1().ClusterRoleBindings().Delete(ctx, c.ServiceAccountName, foreground()))
		swallow(cs.RbacV1().ClusterRoles().Delete(ctx, c.ServiceAccountName, foreground()))
	} else {
		// Reverse every per-namespace binding (FR-017). Default to the SA's namespace for a
		// Created built before BindingNamespaces was tracked.
		nsList := c.BindingNamespaces
		if len(nsList) == 0 {
			nsList = []string{c.ServiceAccountNamespace}
		}
		for i := len(nsList) - 1; i >= 0; i-- {
			ns := nsList[i]
			swallow(cs.RbacV1().RoleBindings(ns).Delete(ctx, c.ServiceAccountName, foreground()))
			swallow(cs.RbacV1().Roles(ns).Delete(ctx, c.ServiceAccountName, foreground()))
		}
	}
	swallow(cs.CoreV1().ServiceAccounts(c.ServiceAccountNamespace).Delete(ctx, c.ServiceAccountName, foreground()))
	return errors.Join(errs...)
}

func foreground() metav1.DeleteOptions {
	return metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)}
}
