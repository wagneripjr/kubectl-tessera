package gc

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/wagneripjr/kubectl-tessera/internal/labels"
)

var (
	fixedNow = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	past     = fixedNow.Add(-time.Hour)
	future   = fixedNow.Add(time.Hour)
)

func TestSweepDeletesExpiredManagedSet(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	createSA(t, cs, meta("prod", "tessera-alice-1", managed, expiresAnn(past)))
	createRole(t, cs, meta("prod", "tessera-alice-1", managed, expiresAnn(past)))
	createRoleBinding(t, cs, meta("prod", "tessera-alice-1", managed, expiresAnn(past)))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 3 {
		t.Fatalf("Deleted = %d, want 3", res.Deleted)
	}
	assertGone(t, "service account", func() error { return getSA(ctx, cs, "prod", "tessera-alice-1") })
	assertGone(t, "role", func() error { return getRole(ctx, cs, "prod", "tessera-alice-1") })
	assertGone(t, "role binding", func() error { return getRoleBinding(ctx, cs, "prod", "tessera-alice-1") })
}

func TestSweepKeepsUnexpiredManagedSet(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	createSA(t, cs, meta("prod", "tessera-bob-1", managed, expiresAnn(future)))
	createRole(t, cs, meta("prod", "tessera-bob-1", managed, expiresAnn(future)))
	createRoleBinding(t, cs, meta("prod", "tessera-bob-1", managed, expiresAnn(future)))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 0 || res.SkippedFresh != 3 {
		t.Fatalf("Deleted/SkippedFresh = %d/%d, want 0/3", res.Deleted, res.SkippedFresh)
	}
	assertPresent(t, "service account", func() error { return getSA(ctx, cs, "prod", "tessera-bob-1") })
	assertPresent(t, "role", func() error { return getRole(ctx, cs, "prod", "tessera-bob-1") })
	assertPresent(t, "role binding", func() error { return getRoleBinding(ctx, cs, "prod", "tessera-bob-1") })
}

func TestSweepIgnoresUnmanagedObjects(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	createRoleBinding(t, cs, meta("prod", "someone-elses-binding", unmanaged, expiresAnn(past)))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Scanned != 0 {
		t.Fatalf("Scanned = %d, want 0 (unmanaged objects are invisible to the selector)", res.Scanned)
	}
	assertPresent(t, "unmanaged role binding", func() error { return getRoleBinding(ctx, cs, "prod", "someone-elses-binding") })
}

func TestSweepIsIdempotentOnSecondRun(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	createSA(t, cs, meta("prod", "tessera-carol-1", managed, expiresAnn(past)))

	if _, err := Sweep(ctx, cs, fixedNow); err != nil {
		t.Fatalf("first Sweep returned error: %v", err)
	}
	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("second Sweep returned error: %v", err)
	}
	if res.Deleted != 0 || res.Scanned != 0 {
		t.Fatalf("second run Deleted/Scanned = %d/%d, want 0/0 (no-op)", res.Deleted, res.Scanned)
	}
}

func TestSweepSkipsMissingExpiresAtAnnotation(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	createSA(t, cs, meta("prod", "tessera-dave-1", managed, nil))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 0 || res.SkippedUnparseable != 1 {
		t.Fatalf("Deleted/SkippedUnparseable = %d/%d, want 0/1", res.Deleted, res.SkippedUnparseable)
	}
	assertPresent(t, "service account", func() error { return getSA(ctx, cs, "prod", "tessera-dave-1") })
}

func TestSweepSkipsUnparseableExpiresAt(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	createSA(t, cs, meta("prod", "tessera-erin-1", managed, map[string]string{labels.ExpiresAtKey: "not-a-date"}))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 0 || res.SkippedUnparseable != 1 {
		t.Fatalf("Deleted/SkippedUnparseable = %d/%d, want 0/1", res.Deleted, res.SkippedUnparseable)
	}
	assertPresent(t, "service account", func() error { return getSA(ctx, cs, "prod", "tessera-erin-1") })
}

func TestSweepSpansAllNamespaces(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	createSA(t, cs, meta("team-a", "tessera-frank-1", managed, expiresAnn(past)))
	createSA(t, cs, meta("team-b", "tessera-frank-2", managed, expiresAnn(past)))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 2 {
		t.Fatalf("Deleted = %d, want 2", res.Deleted)
	}
	assertGone(t, "service account in team-a", func() error { return getSA(ctx, cs, "team-a", "tessera-frank-1") })
	assertGone(t, "service account in team-b", func() error { return getSA(ctx, cs, "team-b", "tessera-frank-2") })
}

func TestSweepDeletesClusterScopedSet(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	createClusterRole(t, cs, meta("", "tessera-grace-1", managed, expiresAnn(past)))
	createClusterRoleBinding(t, cs, meta("", "tessera-grace-1", managed, expiresAnn(past)))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 2 {
		t.Fatalf("Deleted = %d, want 2", res.Deleted)
	}
	assertGone(t, "cluster role", func() error { return getClusterRole(ctx, cs, "tessera-grace-1") })
	assertGone(t, "cluster role binding", func() error { return getClusterRoleBinding(ctx, cs, "tessera-grace-1") })
}

func TestSweepTreatsExactExpiryAsNotExpired(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	createSA(t, cs, meta("prod", "tessera-heidi-1", managed, expiresAnn(fixedNow)))

	res, err := Sweep(ctx, cs, fixedNow)
	if err != nil {
		t.Fatalf("Sweep returned error: %v", err)
	}
	if res.Deleted != 0 || res.SkippedFresh != 1 {
		t.Fatalf("Deleted/SkippedFresh = %d/%d, want 0/1", res.Deleted, res.SkippedFresh)
	}
	assertPresent(t, "service account", func() error { return getSA(ctx, cs, "prod", "tessera-heidi-1") })
}

const (
	managed   = true
	unmanaged = false
)

func meta(ns, name string, isManaged bool, ann map[string]string) metav1.ObjectMeta {
	lbls := map[string]string{labels.OwnerKey: "seed"}
	if isManaged {
		lbls = labels.Set("seed", "sess-"+name)
	}
	return metav1.ObjectMeta{Name: name, Namespace: ns, Labels: lbls, Annotations: ann}
}

func expiresAnn(at time.Time) map[string]string {
	return map[string]string{labels.ExpiresAtKey: at.UTC().Format(time.RFC3339)}
}

func createSA(t *testing.T, cs kubernetes.Interface, om metav1.ObjectMeta) {
	t.Helper()
	if _, err := cs.CoreV1().ServiceAccounts(om.Namespace).Create(context.Background(),
		&corev1.ServiceAccount{ObjectMeta: om}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding service account: %v", err)
	}
}

func createRole(t *testing.T, cs kubernetes.Interface, om metav1.ObjectMeta) {
	t.Helper()
	if _, err := cs.RbacV1().Roles(om.Namespace).Create(context.Background(),
		&rbacv1.Role{ObjectMeta: om}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding role: %v", err)
	}
}

func createRoleBinding(t *testing.T, cs kubernetes.Interface, om metav1.ObjectMeta) {
	t.Helper()
	if _, err := cs.RbacV1().RoleBindings(om.Namespace).Create(context.Background(),
		&rbacv1.RoleBinding{ObjectMeta: om}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding role binding: %v", err)
	}
}

func createClusterRole(t *testing.T, cs kubernetes.Interface, om metav1.ObjectMeta) {
	t.Helper()
	if _, err := cs.RbacV1().ClusterRoles().Create(context.Background(),
		&rbacv1.ClusterRole{ObjectMeta: om}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding cluster role: %v", err)
	}
}

func createClusterRoleBinding(t *testing.T, cs kubernetes.Interface, om metav1.ObjectMeta) {
	t.Helper()
	if _, err := cs.RbacV1().ClusterRoleBindings().Create(context.Background(),
		&rbacv1.ClusterRoleBinding{ObjectMeta: om}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding cluster role binding: %v", err)
	}
}

func getSA(ctx context.Context, cs kubernetes.Interface, ns, name string) error {
	_, err := cs.CoreV1().ServiceAccounts(ns).Get(ctx, name, metav1.GetOptions{})
	return err
}

func getRole(ctx context.Context, cs kubernetes.Interface, ns, name string) error {
	_, err := cs.RbacV1().Roles(ns).Get(ctx, name, metav1.GetOptions{})
	return err
}

func getRoleBinding(ctx context.Context, cs kubernetes.Interface, ns, name string) error {
	_, err := cs.RbacV1().RoleBindings(ns).Get(ctx, name, metav1.GetOptions{})
	return err
}

func getClusterRole(ctx context.Context, cs kubernetes.Interface, name string) error {
	_, err := cs.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
	return err
}

func getClusterRoleBinding(ctx context.Context, cs kubernetes.Interface, name string) error {
	_, err := cs.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
	return err
}

func assertGone(t *testing.T, what string, get func() error) {
	t.Helper()
	if err := get(); !apierrors.IsNotFound(err) {
		t.Fatalf("%s should have been deleted, got err=%v", what, err)
	}
}

func assertPresent(t *testing.T, what string, get func() error) {
	t.Helper()
	if err := get(); err != nil {
		t.Fatalf("%s should still exist, got err=%v", what, err)
	}
}
