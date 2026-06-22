package preflight

import (
	"context"
	"fmt"
	"io"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Attribute struct {
	Verb      string
	Group     string
	Resource  string
	Namespace string
	Name      string
}

type Decision struct {
	Attribute Attribute
	Allowed   bool
	Reason    string
}

type Result struct {
	Decisions []Decision
	Denied    []Decision
}

func (r Result) AllAllowed() bool { return len(r.Denied) == 0 }

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

func RenderTable(w io.Writer, r Result) {
	_, _ = fmt.Fprintln(w, "tessera: pre-flight authorization (SelfSubjectAccessReview):")
	for _, d := range r.Decisions {
		verdict := "ALLOWED"
		if !d.Allowed {
			verdict = "DENIED"
		}
		_, _ = fmt.Fprintf(w, "  %-7s %s %s%s%s", verdict, d.Attribute.Verb, d.Attribute.Resource,
			scopeSuffix(d.Attribute), nameSuffix(d.Attribute))
		if !d.Allowed && d.Reason != "" {
			_, _ = fmt.Fprintf(w, "  (%s)", d.Reason)
		}
		_, _ = fmt.Fprintln(w)
	}
}

func RenderMissingCreate(w io.Writer, denied []Decision) {
	seen := map[string]bool{}
	for _, d := range denied {
		if seen[d.Attribute.Resource] {
			continue
		}
		seen[d.Attribute.Resource] = true
		_, _ = fmt.Fprintf(w, "tessera: missing verb: create on %s\n", d.Attribute.Resource)
	}
	_, _ = fmt.Fprintln(w, "tessera: an administrator must grant create on these resources (or bind on a curated role).")
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
