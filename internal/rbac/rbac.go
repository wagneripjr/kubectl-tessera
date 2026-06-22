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

type Spec struct {
	BaseName      string
	Namespace     string
	Namespaces    []string
	ClusterScoped bool
	Rules         []rbacv1.PolicyRule
	Labels        map[string]string
	Annotations   map[string]string
}

type Created struct {
	ServiceAccountName      string
	ServiceAccountNamespace string
	ClusterScoped           bool
	BindingNamespaces       []string
}

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
