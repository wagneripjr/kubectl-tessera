package rbac

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

const (
	testName    = "tessera-alice-1a2b3c4d"
	testNS      = "prod"
	managedBy   = "app.kubernetes.io/managed-by"
	sessionKey  = "tessera.adustio.com/session-id"
	expiresAtKy = "tessera.adustio.com/expires-at"
)

func newSpec(clusterScoped bool) Spec {
	return Spec{
		BaseName:      testName,
		Namespace:     testNS,
		ClusterScoped: clusterScoped,
		Rules:         []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}},
		Labels:        map[string]string{managedBy: "kubectl-tessera", sessionKey: "1a2b3c4d"},
		Annotations:   map[string]string{expiresAtKy: "2026-06-19T12:00:00Z"},
	}
}

func TestCreateStampsAndWiresTheSet(t *testing.T) {
	cases := []struct {
		name          string
		clusterScoped bool
		check         func(t *testing.T, ctx context.Context, cs kubernetes.Interface)
	}{
		{
			name:          "namespaced set uses Role and RoleBinding",
			clusterScoped: false,
			check: func(t *testing.T, ctx context.Context, cs kubernetes.Interface) {
				role, err := cs.RbacV1().Roles(testNS).Get(ctx, testName, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("role not created: %v", err)
				}
				if !reflect.DeepEqual(role.Rules, newSpec(false).Rules) {
					t.Fatalf("role.Rules = %+v, want %+v", role.Rules, newSpec(false).Rules)
				}
				rb, err := cs.RbacV1().RoleBindings(testNS).Get(ctx, testName, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("role binding not created: %v", err)
				}
				if rb.RoleRef.Kind != "Role" || rb.RoleRef.Name != testName {
					t.Fatalf("roleRef = %+v, want Role/%s", rb.RoleRef, testName)
				}
				assertBoundToServiceAccount(t, rb.Subjects)
			},
		},
		{
			name:          "cluster-scoped set uses ClusterRole and ClusterRoleBinding",
			clusterScoped: true,
			check: func(t *testing.T, ctx context.Context, cs kubernetes.Interface) {
				cr, err := cs.RbacV1().ClusterRoles().Get(ctx, testName, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("cluster role not created: %v", err)
				}
				if cr.Labels[sessionKey] != "1a2b3c4d" {
					t.Fatalf("cluster role missing session label: %+v", cr.Labels)
				}
				crb, err := cs.RbacV1().ClusterRoleBindings().Get(ctx, testName, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("cluster role binding not created: %v", err)
				}
				if crb.RoleRef.Kind != "ClusterRole" || crb.RoleRef.Name != testName {
					t.Fatalf("roleRef = %+v, want ClusterRole/%s", crb.RoleRef, testName)
				}
				assertBoundToServiceAccount(t, crb.Subjects)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			ctx := context.Background()

			created, err := Create(ctx, cs, newSpec(tc.clusterScoped))
			if err != nil {
				t.Fatalf("Create returned error: %v", err)
			}
			if created.ServiceAccountName != testName || created.ServiceAccountNamespace != testNS {
				t.Fatalf("created = %+v, want SA %s/%s", created, testNS, testName)
			}
			sa, err := cs.CoreV1().ServiceAccounts(testNS).Get(ctx, testName, metav1.GetOptions{})
			if err != nil {
				t.Fatalf("service account not created: %v", err)
			}
			if sa.Labels[managedBy] != "kubectl-tessera" || sa.Annotations[expiresAtKy] == "" {
				t.Fatalf("service account missing labels/annotations: %+v", sa.ObjectMeta)
			}
			tc.check(t, ctx, cs)
		})
	}
}

func assertBoundToServiceAccount(t *testing.T, subjects []rbacv1.Subject) {
	t.Helper()
	if len(subjects) != 1 || subjects[0].Kind != "ServiceAccount" || subjects[0].Name != testName || subjects[0].Namespace != testNS {
		t.Fatalf("subjects = %+v, want the created service account", subjects)
	}
}

func TestCreateRollsBackWhenBindingFails(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()
	cs.PrependReactor("create", "rolebindings", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("forbidden: cannot create rolebindings")
	})

	_, err := Create(ctx, cs, newSpec(false))
	if err == nil {
		t.Fatal("expected Create to fail when the role binding cannot be created")
	}

	assertGone(t, ctx, cs, "service account", func() error {
		_, e := cs.CoreV1().ServiceAccounts(testNS).Get(ctx, testName, metav1.GetOptions{})
		return e
	})
	assertGone(t, ctx, cs, "role", func() error {
		_, e := cs.RbacV1().Roles(testNS).Get(ctx, testName, metav1.GetOptions{})
		return e
	})
}

func TestRollbackRemovesTheWholeSet(t *testing.T) {
	cs := fake.NewSimpleClientset()
	ctx := context.Background()

	created, err := Create(ctx, cs, newSpec(false))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := Rollback(ctx, cs, created); err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}

	assertGone(t, ctx, cs, "service account", func() error {
		_, e := cs.CoreV1().ServiceAccounts(testNS).Get(ctx, testName, metav1.GetOptions{})
		return e
	})
	assertGone(t, ctx, cs, "role", func() error {
		_, e := cs.RbacV1().Roles(testNS).Get(ctx, testName, metav1.GetOptions{})
		return e
	})
	assertGone(t, ctx, cs, "role binding", func() error {
		_, e := cs.RbacV1().RoleBindings(testNS).Get(ctx, testName, metav1.GetOptions{})
		return e
	})
}

func assertGone(t *testing.T, _ context.Context, _ kubernetes.Interface, what string, get func() error) {
	t.Helper()
	if err := get(); !apierrors.IsNotFound(err) {
		t.Fatalf("%s should have been removed, got err=%v", what, err)
	}
}
