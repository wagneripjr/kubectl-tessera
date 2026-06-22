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

type Descriptor struct {
	SessionID      string   `json:"sessionID"`
	Owner          string   `json:"owner"`
	Scope          string   `json:"scope"`
	ExpiresAt      string   `json:"expiresAt"`
	Namespaces     []string `json:"namespaces,omitempty"`
	KubeconfigPath string   `json:"kubeconfigPath,omitempty"`
	CreatedObjects []string `json:"createdObjects,omitempty"`
}

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

func summarizeRules(rules []rbacv1.PolicyRule) string {
	segs := make([]string, 0, len(rules))
	for _, r := range rules {
		segs = append(segs, strings.Join(r.Verbs, ",")+":"+strings.Join(r.Resources, ","))
	}
	return strings.Join(segs, ";")
}
