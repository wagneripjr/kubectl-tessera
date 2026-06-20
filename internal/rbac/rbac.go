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
type Spec struct {
	BaseName      string
	Namespace     string // ServiceAccount namespace (and Role/RoleBinding namespace when not cluster-scoped)
	ClusterScoped bool
	Rules         []rbacv1.PolicyRule
	Labels        map[string]string
	Annotations   map[string]string
}

// Created records what Create made, enough for token minting and rollback.
type Created struct {
	ServiceAccountName      string
	ServiceAccountNamespace string
	ClusterScoped           bool
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

	objectMeta := func(namespaced bool) metav1.ObjectMeta {
		m := metav1.ObjectMeta{Name: spec.BaseName, Labels: spec.Labels, Annotations: spec.Annotations}
		if namespaced {
			m.Namespace = spec.Namespace
		}
		return m
	}

	sa := &corev1.ServiceAccount{ObjectMeta: objectMeta(true)}
	if _, err := cs.CoreV1().ServiceAccounts(spec.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		return Created{}, fmt.Errorf("creating service account %s/%s: %w", spec.Namespace, spec.BaseName, err)
	}
	undo = append(undo, func() { _ = cs.CoreV1().ServiceAccounts(spec.Namespace).Delete(ctx, spec.BaseName, foreground()) })

	subjects := []rbacv1.Subject{{Kind: "ServiceAccount", Name: spec.BaseName, Namespace: spec.Namespace}}

	if spec.ClusterScoped {
		cr := &rbacv1.ClusterRole{ObjectMeta: objectMeta(false), Rules: spec.Rules}
		if _, err := cs.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{}); err != nil {
			rollback()
			return Created{}, fmt.Errorf("creating cluster role %s: %w", spec.BaseName, err)
		}
		undo = append(undo, func() { _ = cs.RbacV1().ClusterRoles().Delete(ctx, spec.BaseName, foreground()) })

		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: objectMeta(false),
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: "ClusterRole", Name: spec.BaseName},
			Subjects:   subjects,
		}
		if _, err := cs.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
			rollback()
			return Created{}, fmt.Errorf("creating cluster role binding %s: %w", spec.BaseName, err)
		}
	} else {
		role := &rbacv1.Role{ObjectMeta: objectMeta(true), Rules: spec.Rules}
		if _, err := cs.RbacV1().Roles(spec.Namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil {
			rollback()
			return Created{}, fmt.Errorf("creating role %s/%s: %w", spec.Namespace, spec.BaseName, err)
		}
		undo = append(undo, func() { _ = cs.RbacV1().Roles(spec.Namespace).Delete(ctx, spec.BaseName, foreground()) })

		rb := &rbacv1.RoleBinding{
			ObjectMeta: objectMeta(true),
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: "Role", Name: spec.BaseName},
			Subjects:   subjects,
		}
		if _, err := cs.RbacV1().RoleBindings(spec.Namespace).Create(ctx, rb, metav1.CreateOptions{}); err != nil {
			rollback()
			return Created{}, fmt.Errorf("creating role binding %s/%s: %w", spec.Namespace, spec.BaseName, err)
		}
	}

	return Created{
		ServiceAccountName:      spec.BaseName,
		ServiceAccountNamespace: spec.Namespace,
		ClusterScoped:           spec.ClusterScoped,
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
		swallow(cs.RbacV1().RoleBindings(c.ServiceAccountNamespace).Delete(ctx, c.ServiceAccountName, foreground()))
		swallow(cs.RbacV1().Roles(c.ServiceAccountNamespace).Delete(ctx, c.ServiceAccountName, foreground()))
	}
	swallow(cs.CoreV1().ServiceAccounts(c.ServiceAccountNamespace).Delete(ctx, c.ServiceAccountName, foreground()))
	return errors.Join(errs...)
}

func foreground() metav1.DeleteOptions {
	return metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)}
}
