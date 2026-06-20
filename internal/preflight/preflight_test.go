package preflight

import (
	"context"
	"fmt"
	"strings"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// allowVerbsReactor makes SelfSubjectAccessReview.Create echo back an Allowed
// decision for any verb in allow, and a denied decision (with a reason) otherwise.
func allowVerbsReactor(allow ...string) clienttesting.ReactionFunc {
	allowed := map[string]bool{}
	for _, v := range allow {
		allowed[v] = true
	}
	return func(action clienttesting.Action) (bool, runtime.Object, error) {
		ssar := action.(clienttesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		verb := ssar.Spec.ResourceAttributes.Verb
		out := ssar.DeepCopy()
		if allowed[verb] {
			out.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: true}
		} else {
			out.Status = authorizationv1.SubjectAccessReviewStatus{Allowed: false, Reason: "no RBAC rule grants " + verb}
		}
		return true, out, nil
	}
}

func TestCheckAggregatesDecisions(t *testing.T) {
	cases := []struct {
		name           string
		allow          []string
		attrs          []Attribute
		wantAllAllowed bool
		wantDenied     int
	}{
		{
			name:           "all requested verbs allowed",
			allow:          []string{"get", "list", "watch"},
			attrs:          []Attribute{{Verb: "get", Resource: "pods", Namespace: "prod"}, {Verb: "list", Resource: "pods", Namespace: "prod"}},
			wantAllAllowed: true,
			wantDenied:     0,
		},
		{
			name:           "one denied verb collected",
			allow:          []string{"get"},
			attrs:          []Attribute{{Verb: "get", Resource: "pods", Namespace: "prod"}, {Verb: "delete", Resource: "pods", Namespace: "prod"}},
			wantAllAllowed: false,
			wantDenied:     1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := fake.NewSimpleClientset()
			cs.PrependReactor("create", "selfsubjectaccessreviews", allowVerbsReactor(tc.allow...))

			res, err := Check(context.Background(), cs, tc.attrs)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if res.AllAllowed() != tc.wantAllAllowed {
				t.Fatalf("AllAllowed() = %t, want %t (denied=%+v)", res.AllAllowed(), tc.wantAllAllowed, res.Denied)
			}
			if len(res.Denied) != tc.wantDenied {
				t.Fatalf("denied = %+v, want %d", res.Denied, tc.wantDenied)
			}
			for _, d := range res.Denied {
				if d.Reason == "" {
					t.Fatalf("denied decision %+v has no reason", d)
				}
			}
		})
	}
}

func TestCheckPropagatesTransportError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "selfsubjectaccessreviews", func(clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("connection refused")
	})

	_, err := Check(context.Background(), cs, []Attribute{{Verb: "get", Resource: "pods", Namespace: "prod"}})
	if err == nil {
		t.Fatal("expected Check to propagate the transport error")
	}
}

func TestRenderTableShowsAllowedAndDenied(t *testing.T) {
	res := Result{
		Decisions: []Decision{
			{Attribute: Attribute{Verb: "get", Resource: "pods", Namespace: "prod"}, Allowed: true},
			{Attribute: Attribute{Verb: "delete", Resource: "pods", Namespace: "prod"}, Allowed: false, Reason: "denied by RBAC"},
		},
		Denied: []Decision{
			{Attribute: Attribute{Verb: "delete", Resource: "pods", Namespace: "prod"}, Allowed: false, Reason: "denied by RBAC"},
		},
	}
	var sb strings.Builder
	RenderTable(&sb, res)
	out := sb.String()
	for _, want := range []string{"ALLOWED", "DENIED", "delete", "pods"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}
