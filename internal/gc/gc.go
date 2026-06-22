package gc

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/wagneripjr/kubectl-tessera/internal/labels"
)

type Result struct {
	Scanned            int
	Deleted            int
	SkippedFresh       int
	SkippedUnparseable int
}

func Sweep(ctx context.Context, cs kubernetes.Interface, now time.Time) (Result, error) {
	var res Result
	var errs []error
	lo := metav1.ListOptions{LabelSelector: labels.ManagedSelector()}

	consider := func(om metav1.ObjectMeta, del func() error) {
		res.Scanned++
		v := om.Annotations[labels.ExpiresAtKey]
		exp, err := time.Parse(time.RFC3339, v)
		if v == "" || err != nil {
			res.SkippedUnparseable++
			return
		}
		if !now.After(exp) {
			res.SkippedFresh++
			return
		}
		if e := del(); e != nil && !apierrors.IsNotFound(e) {
			errs = append(errs, fmt.Errorf("deleting %s/%s: %w", om.Namespace, om.Name, e))
			return
		}
		res.Deleted++
	}

	if crbs, err := cs.RbacV1().ClusterRoleBindings().List(ctx, lo); err != nil {
		errs = append(errs, fmt.Errorf("listing clusterrolebindings: %w", err))
	} else {
		for i := range crbs.Items {
			om := crbs.Items[i].ObjectMeta
			consider(om, func() error { return cs.RbacV1().ClusterRoleBindings().Delete(ctx, om.Name, foreground()) })
		}
	}

	if rbs, err := cs.RbacV1().RoleBindings(metav1.NamespaceAll).List(ctx, lo); err != nil {
		errs = append(errs, fmt.Errorf("listing rolebindings: %w", err))
	} else {
		for i := range rbs.Items {
			om := rbs.Items[i].ObjectMeta
			consider(om, func() error { return cs.RbacV1().RoleBindings(om.Namespace).Delete(ctx, om.Name, foreground()) })
		}
	}

	if crs, err := cs.RbacV1().ClusterRoles().List(ctx, lo); err != nil {
		errs = append(errs, fmt.Errorf("listing clusterroles: %w", err))
	} else {
		for i := range crs.Items {
			om := crs.Items[i].ObjectMeta
			consider(om, func() error { return cs.RbacV1().ClusterRoles().Delete(ctx, om.Name, foreground()) })
		}
	}

	if roles, err := cs.RbacV1().Roles(metav1.NamespaceAll).List(ctx, lo); err != nil {
		errs = append(errs, fmt.Errorf("listing roles: %w", err))
	} else {
		for i := range roles.Items {
			om := roles.Items[i].ObjectMeta
			consider(om, func() error { return cs.RbacV1().Roles(om.Namespace).Delete(ctx, om.Name, foreground()) })
		}
	}

	if sas, err := cs.CoreV1().ServiceAccounts(metav1.NamespaceAll).List(ctx, lo); err != nil {
		errs = append(errs, fmt.Errorf("listing serviceaccounts: %w", err))
	} else {
		for i := range sas.Items {
			om := sas.Items[i].ObjectMeta
			consider(om, func() error { return cs.CoreV1().ServiceAccounts(om.Namespace).Delete(ctx, om.Name, foreground()) })
		}
	}

	return res, errors.Join(errs...)
}

func foreground() metav1.DeleteOptions {
	return metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)}
}
