package scope

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type PolicyRule = rbacv1.PolicyRule

type Request struct {
	Verbs         []string
	Resources     []string
	ResourceNames []string
	APIGroup      string
	ClusterScoped bool
	Namespace     string
}

type ResolvedResource struct {
	Resource   string
	Group      string
	Namespaced bool
}

type Resolution struct {
	Resources []ResolvedResource
	Rules     []PolicyRule
}

func Resolve(req Request, mapper meta.RESTMapper) (Resolution, error) {
	if len(req.Resources) == 0 {
		return Resolution{}, fmt.Errorf("at least one --resource is required")
	}
	if req.ClusterScoped && req.Namespace != "" {
		return Resolution{}, fmt.Errorf("--namespace cannot be used with cluster-scoped resources; omit -n")
	}

	resolved := make([]ResolvedResource, 0, len(req.Resources))
	for _, r := range req.Resources {
		gvr, err := mapper.ResourceFor(schema.GroupVersionResource{Group: req.APIGroup, Resource: r})
		if err != nil {
			if meta.IsNoMatchError(err) {
				return Resolution{}, fmt.Errorf("unknown resource %q: %w", r, err)
			}
			return Resolution{}, fmt.Errorf("resolving resource %q (set --api-group to disambiguate): %w", r, err)
		}
		gvk, err := mapper.KindFor(gvr)
		if err != nil {
			return Resolution{}, fmt.Errorf("resolving kind for %q: %w", r, err)
		}
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			return Resolution{}, fmt.Errorf("resolving mapping for %q: %w", r, err)
		}
		namespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace

		switch {
		case namespaced && req.ClusterScoped:
			return Resolution{}, fmt.Errorf("%q is namespaced; do not pass --cluster-scoped", gvr.Resource)
		case !namespaced && !req.ClusterScoped:
			return Resolution{}, fmt.Errorf("%q is cluster-scoped; pass --cluster-scoped and omit -n", gvr.Resource)
		}

		resolved = append(resolved, ResolvedResource{Resource: gvr.Resource, Group: gvr.Group, Namespaced: namespaced})
	}

	return Resolution{Resources: resolved, Rules: buildRules(resolved, req.Verbs, req.ResourceNames)}, nil
}

func buildRules(resolved []ResolvedResource, verbs, names []string) []PolicyRule {
	order := make([]string, 0)
	byGroup := make(map[string][]string)
	for _, r := range resolved {
		if _, seen := byGroup[r.Group]; !seen {
			order = append(order, r.Group)
		}
		byGroup[r.Group] = append(byGroup[r.Group], r.Resource)
	}
	rules := make([]PolicyRule, 0, len(order))
	for _, g := range order {
		rules = append(rules, PolicyRule{
			APIGroups:     []string{g},
			Resources:     byGroup[g],
			Verbs:         verbs,
			ResourceNames: names,
		})
	}
	return rules
}
