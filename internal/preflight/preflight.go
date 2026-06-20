// Package preflight implements the SelfSubjectAccessReview authorization gate
// (authoritative, FR-003). One SSAR is issued per requested attribute, as the
// invoking user (no impersonation); any denial must abort the mint before any
// object is created. See ADR-006.
package preflight

import (
	"context"
	"fmt"
	"io"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Attribute is one (verb, resource, namespace, name) the operator must be allowed
// to perform. Group is the API group ("" for core); Name narrows to a single object.
type Attribute struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string
	Name      string
}

// Decision is the authorizer's verdict for one Attribute.
type Decision struct {
	Attribute Attribute
	Allowed   bool
	Reason    string
}

// Result aggregates every decision and the subset that was denied.
type Result struct {
	Decisions []Decision
	Denied    []Decision
}

// AllAllowed reports whether every requested attribute was permitted.
func (r Result) AllAllowed() bool { return len(r.Denied) == 0 }

// Check issues one SelfSubjectAccessReview per attribute using cs (the invoking
// user's clientset — never impersonation). A transport/API error short-circuits
// and is returned; authorization denials are collected into the Result, not an error.
func Check(ctx context.Context, cs kubernetes.Interface, attrs []Attribute) (Result, error) {
	var res Result
	for _, a := range attrs {
		ssar := &authorizationv1.SelfSubjectAccessReview{
			Spec: authorizationv1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &authorizationv1.ResourceAttributes{
					Verb:      a.Verb,
					Group:     a.Group,
					Resource:  a.Resource,
					Namespace: a.Namespace,
					Name:      a.Name,
				},
			},
		}
		out, err := cs.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, ssar, metav1.CreateOptions{})
		if err != nil {
			return Result{}, fmt.Errorf("self subject access review for %q %q: %w", a.Verb, a.Resource, err)
		}
		d := Decision{Attribute: a, Allowed: out.Status.Allowed, Reason: out.Status.Reason}
		res.Decisions = append(res.Decisions, d)
		if !d.Allowed {
			res.Denied = append(res.Denied, d)
		}
	}
	return res, nil
}

// RenderTable writes a human-readable allowed/denied table to w (stderr). The
// literal verdict markers ALLOWED/DENIED let callers and tests assert outcomes.
func RenderTable(w io.Writer, r Result) {
	fmt.Fprintln(w, "tessera: pre-flight authorization (SelfSubjectAccessReview):")
	for _, d := range r.Decisions {
		verdict := "ALLOWED"
		if !d.Allowed {
			verdict = "DENIED"
		}
		fmt.Fprintf(w, "  %-7s %s %s%s%s", verdict, d.Attribute.Verb, d.Attribute.Resource,
			scopeSuffix(d.Attribute), nameSuffix(d.Attribute))
		if !d.Allowed && d.Reason != "" {
			fmt.Fprintf(w, "  (%s)", d.Reason)
		}
		fmt.Fprintln(w)
	}
}

func scopeSuffix(a Attribute) string {
	if a.Namespace == "" {
		return " (cluster)"
	}
	return " in " + a.Namespace
}

func nameSuffix(a Attribute) string {
	if a.Name == "" {
		return ""
	}
	return "/" + a.Name
}
