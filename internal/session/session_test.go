package session

import (
	"context"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/wagneripjr/kubectl-tessera/internal/labels"
)

// managedRole builds a tessera-managed Role for a session, mirroring what rbac.Create
// stamps: managed-by + owner + session-id labels and the expires-at annotation.
func managedRole(name, ns, owner, sessionID, expiresAt string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels: map[string]string{
				labels.ManagedByKey: labels.ManagedByValue,
				labels.OwnerKey:     owner,
				labels.SessionIDKey: sessionID,
			},
			Annotations: map[string]string{labels.ExpiresAtKey: expiresAt},
		},
		Rules: rules,
	}
}

func readPods() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}}}
}

func TestListEmptyReturnsNonNilEmptySlice(t *testing.T) {
	cs := fake.NewSimpleClientset()
	got, err := List(context.Background(), cs)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Fatal("expected a non-nil slice so JSON renders [] not null")
	}
	if len(got) != 0 {
		t.Errorf("expected no sessions, got %d", len(got))
	}
}

func TestListDerivesSessionFieldsFromManagedRole(t *testing.T) {
	exp := time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339)
	cs := fake.NewSimpleClientset(
		managedRole("r1", "team-a", "alice", "sess-1", exp, readPods()),
	)
	got, err := List(context.Background(), cs)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 session, got %d: %+v", len(got), got)
	}
	d := got[0]
	if d.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", d.SessionID, "sess-1")
	}
	if d.Owner != "alice" {
		t.Errorf("Owner = %q, want %q", d.Owner, "alice")
	}
	if d.ExpiresAt != exp {
		t.Errorf("ExpiresAt = %q, want %q", d.ExpiresAt, exp)
	}
	if d.Scope != "get,list,watch:pods" {
		t.Errorf("Scope = %q, want %q", d.Scope, "get,list,watch:pods")
	}
	if len(d.Namespaces) != 1 || d.Namespaces[0] != "team-a" {
		t.Errorf("Namespaces = %v, want [team-a]", d.Namespaces)
	}
}

func TestListGroupsAndSortsBySessionID(t *testing.T) {
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	cs := fake.NewSimpleClientset(
		managedRole("r-b", "ns", "bob", "sess-b", exp, readPods()),
		managedRole("r-a", "ns", "alice", "sess-a", exp, readPods()),
	)
	got, err := List(context.Background(), cs)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(got))
	}
	if got[0].SessionID != "sess-a" || got[1].SessionID != "sess-b" {
		t.Errorf("expected sessions sorted by id [sess-a sess-b], got [%s %s]", got[0].SessionID, got[1].SessionID)
	}
}

func TestListIgnoresUnmanagedObjects(t *testing.T) {
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	unmanaged := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "ns"}, // no managed-by label
		Rules:      readPods(),
	}
	cs := fake.NewSimpleClientset(
		managedRole("mine", "ns", "alice", "sess-1", exp, readPods()),
		unmanaged,
	)
	got, err := List(context.Background(), cs)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].SessionID != "sess-1" {
		t.Errorf("expected only the managed session, got %+v", got)
	}
}

func TestListSummarizesClusterScopedSessionFromClusterRole(t *testing.T) {
	exp := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cr1",
			Labels: map[string]string{
				labels.ManagedByKey: labels.ManagedByValue,
				labels.OwnerKey:     "carol",
				labels.SessionIDKey: "sess-cluster",
			},
			Annotations: map[string]string{labels.ExpiresAtKey: exp},
		},
		Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"nodes"}, Verbs: []string{"get", "list"}}},
	}
	cs := fake.NewSimpleClientset(cr)
	got, err := List(context.Background(), cs)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 cluster-scoped session, got %d", len(got))
	}
	if got[0].Scope != "get,list:nodes" {
		t.Errorf("Scope = %q, want %q", got[0].Scope, "get,list:nodes")
	}
	if len(got[0].Namespaces) != 0 {
		t.Errorf("expected no namespaces for a cluster-scoped session, got %v", got[0].Namespaces)
	}
}
