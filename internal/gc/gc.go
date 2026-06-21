// Package gc sweeps expired managed RBAC object sets, selected by the
// app.kubernetes.io/managed-by=kubectl-tessera label and the expires-at annotation.
// It is idempotent and cron-safe (FR-011, NFR-005). See ADR-007.
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

// Result summarizes one sweep over the managed object population.
type Result struct {
	Scanned            int // managed objects examined across all kinds
	Deleted            int // objects issued a Delete because they were expired
	SkippedFresh       int // managed but not yet expired
	SkippedUnparseable int // managed but missing/unparseable expires-at (kept, fail-safe)
}

// Sweep lists every tessera-managed object across all namespaces, parses its
// expires-at annotation, and deletes the ones already expired (now > expires-at).
// Deletion is revoke-first (bindings before roles before service accounts) with
// foreground propagation, mirroring rbac.Rollback applied across the whole cluster.
// It is idempotent: a NotFound during delete is swallowed, so overlapping sweeps and
// re-runs are safe. now is injected so the caller controls the clock (and tests pin it).
//
// Fail-safe: an object whose expires-at is missing or unparseable is NEVER deleted —
// gc only removes what it can prove has expired (NFR-006). The returned error joins any
// non-NotFound list/delete failures; Result still reports the progress that was made.
func Sweep(ctx context.Context, cs kubernetes.Interface, now time.Time) (Result, error) {
	var res Result
	var errs []error
	lo := metav1.ListOptions{LabelSelector: labels.ManagedSelector()}

	// consider records the scan, decides expiry, and on a positive verdict runs del.
	// del returns the API error so NotFound can be swallowed (idempotency).
	consider := func(om metav1.ObjectMeta, del func() error) {
		res.Scanned++
		v := om.Annotations[labels.ExpiresAtKey]
		exp, err := time.Parse(time.RFC3339, v)
		if v == "" || err != nil {
			res.SkippedUnparseable++
			return
		}
		if !now.After(exp) { // strict: now == expires-at is NOT expired
			res.SkippedFresh++
			return
		}
		if e := del(); e != nil && !apierrors.IsNotFound(e) {
			errs = append(errs, fmt.Errorf("deleting %s/%s: %w", om.Namespace, om.Name, e))
			return
		}
		res.Deleted++
	}

	// Revoke-first order: ClusterRoleBinding, RoleBinding (grants) → ClusterRole, Role
	// (permissions) → ServiceAccount (identity). Bindings go first so a partial sweep can
	// never leave a live grant pointing at a half-deleted role/identity.
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

// foreground deletes with foreground propagation so dependents finalize before the
// owner is gone — the same policy rbac.Create/Rollback use.
func foreground() metav1.DeleteOptions {
	return metav1.DeleteOptions{PropagationPolicy: ptr.To(metav1.DeletePropagationForeground)}
}
