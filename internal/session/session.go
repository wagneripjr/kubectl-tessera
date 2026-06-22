// Package session reads the tessera-managed object population back into active-session
// descriptors for `tessera ls` (FR-012). The Descriptor is also the shape mint and
// dry-run render under -o json (FR-015). Selection is the same managed-by contract gc
// uses (ADR-008): only objects carrying the managed-by label are ever considered.
package session

import (
	"context"
	"sort"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/wagneripjr/kubectl-tessera/internal/labels"
)

// Descriptor is the canonical session shape rendered by ls (always) and by mint/dry-run
// under -o json (the optional fields). Slice fields are omitempty so an ls record stays
// minimal while a mint record can carry the kubeconfig path and created-object list.
type Descriptor struct {
	SessionID      string   `json:"sessionID"`
	Owner          string   `json:"owner"`
	Scope          string   `json:"scope"`
	ExpiresAt      string   `json:"expiresAt"`
	Namespaces     []string `json:"namespaces,omitempty"`
	KubeconfigPath string   `json:"kubeconfigPath,omitempty"`
	CreatedObjects []string `json:"createdObjects,omitempty"`
}

// List returns one Descriptor per active managed session, derived from the Roles and
// ClusterRoles tessera created (they carry the full label/annotation set AND the policy
// rules needed for the scope summary). The result is always a non-nil slice (empty ⇒
// "[]" under JSON, FR-012) and is sorted by session-id for stable output.
func List(ctx context.Context, cs kubernetes.Interface) ([]Descriptor, error) {
	lo := metav1.ListOptions{LabelSelector: labels.ManagedSelector()}
	byID := map[string]*Descriptor{}

	roles, err := cs.RbacV1().Roles(metav1.NamespaceAll).List(ctx, lo)
	if err != nil {
		return nil, err
	}
	for i := range roles.Items {
		r := &roles.Items[i]
		d := upsert(byID, r.Labels, r.Annotations)
		d.Scope = summarizeRules(r.Rules)
		addNamespace(d, r.Namespace)
	}

	crs, err := cs.RbacV1().ClusterRoles().List(ctx, lo)
	if err != nil {
		return nil, err
	}
	for i := range crs.Items {
		cr := &crs.Items[i]
		d := upsert(byID, cr.Labels, cr.Annotations)
		d.Scope = summarizeRules(cr.Rules)
	}

	out := make([]Descriptor, 0, len(byID))
	for _, d := range byID {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return out, nil
}

// upsert finds-or-creates the descriptor for the session this object belongs to and
// fills in owner/expiry from its labels/annotations.
func upsert(byID map[string]*Descriptor, lbls, anns map[string]string) *Descriptor {
	id := lbls[labels.SessionIDKey]
	d, ok := byID[id]
	if !ok {
		d = &Descriptor{SessionID: id}
		byID[id] = d
	}
	if owner := lbls[labels.OwnerKey]; owner != "" {
		d.Owner = owner
	}
	if exp := anns[labels.ExpiresAtKey]; exp != "" {
		d.ExpiresAt = exp
	}
	return d
}

func addNamespace(d *Descriptor, ns string) {
	if ns == "" {
		return
	}
	for _, existing := range d.Namespaces {
		if existing == ns {
			return
		}
	}
	d.Namespaces = append(d.Namespaces, ns)
}

// summarizeRules renders the policy rules as "verbs:resources" segments joined by ";",
// preserving each rule's ordering (which mirrors what the operator requested).
func summarizeRules(rules []rbacv1.PolicyRule) string {
	segs := make([]string, 0, len(rules))
	for _, r := range rules {
		segs = append(segs, strings.Join(r.Verbs, ",")+":"+strings.Join(r.Resources, ","))
	}
	return strings.Join(segs, ";")
}
