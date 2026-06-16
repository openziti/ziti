/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package model

import (
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

// RoleAttributeKind identifies the entity target class whose role-attribute
// usage is being queried (e.g., "identity" means attributes that label
// identities and are referenced by identity-role fields on policies).
type RoleAttributeKind string

const (
	RoleAttributeKindIdentity     RoleAttributeKind = "identity"
	RoleAttributeKindEdgeRouter   RoleAttributeKind = "edgeRouter"
	RoleAttributeKindService      RoleAttributeKind = "service"
	RoleAttributeKindPostureCheck RoleAttributeKind = "postureCheck"
)

// RoleAttributeSource names one source population that references a role
// attribute: either the attribute's home-entity collection, or a policy
// collection that references it.
type RoleAttributeSource string

const (
	RoleAttributeSourceIdentities                RoleAttributeSource = "identities"
	RoleAttributeSourceEdgeRouters               RoleAttributeSource = "edgeRouters"
	RoleAttributeSourceServices                  RoleAttributeSource = "services"
	RoleAttributeSourcePostureChecks             RoleAttributeSource = "postureChecks"
	RoleAttributeSourceServicePolicies           RoleAttributeSource = "servicePolicies"
	RoleAttributeSourceEdgeRouterPolicies        RoleAttributeSource = "edgeRouterPolicies"
	RoleAttributeSourceServiceEdgeRouterPolicies RoleAttributeSource = "serviceEdgeRouterPolicies"
)

// RoleAttributeSourceUsage is a count of entities (and optionally the entity
// ids themselves) from one source that use a particular role attribute.
type RoleAttributeSourceUsage struct {
	Count int64
	Ids   []string
}

// RoleAttributeUsage is a role-attribute value paired with its per-source
// usage breakdown across the sources relevant to its RoleAttributeKind.
type RoleAttributeUsage struct {
	RoleAttribute string
	Usage         map[RoleAttributeSource]*RoleAttributeSourceUsage
}

// roleAttributeSource binds a source name to the boltz SetReadIndex that
// serves it.
type roleAttributeSource struct {
	name  RoleAttributeSource
	index boltz.SetReadIndex
}

// sourcesFor returns the ordered list of (name, index) pairs that contribute
// to the given RoleAttributeKind. The order is stable so that the response
// shape is predictable across calls.
func sourcesFor(env Env, kind RoleAttributeKind) []roleAttributeSource {
	stores := env.GetStores()
	switch kind {
	case RoleAttributeKindIdentity:
		return []roleAttributeSource{
			{RoleAttributeSourceIdentities, stores.Identity.GetRoleAttributesIndex()},
			{RoleAttributeSourceServicePolicies, stores.ServicePolicy.GetIdentityRoleAttributesIndex()},
			{RoleAttributeSourceEdgeRouterPolicies, stores.EdgeRouterPolicy.GetIdentityRoleAttributesIndex()},
		}
	case RoleAttributeKindEdgeRouter:
		return []roleAttributeSource{
			{RoleAttributeSourceEdgeRouters, stores.EdgeRouter.GetRoleAttributesIndex()},
			{RoleAttributeSourceEdgeRouterPolicies, stores.EdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()},
			{RoleAttributeSourceServiceEdgeRouterPolicies, stores.ServiceEdgeRouterPolicy.GetEdgeRouterRoleAttributesIndex()},
		}
	case RoleAttributeKindService:
		return []roleAttributeSource{
			{RoleAttributeSourceServices, stores.Service.GetRoleAttributesIndex()},
			{RoleAttributeSourceServicePolicies, stores.ServicePolicy.GetServiceRoleAttributesIndex()},
			{RoleAttributeSourceServiceEdgeRouterPolicies, stores.ServiceEdgeRouterPolicy.GetServiceRoleAttributesIndex()},
		}
	case RoleAttributeKindPostureCheck:
		return []roleAttributeSource{
			{RoleAttributeSourcePostureChecks, stores.PostureCheck.GetRoleAttributesIndex()},
			{RoleAttributeSourceServicePolicies, stores.ServicePolicy.GetPostureCheckRoleAttributesIndex()},
		}
	}
	return nil
}

// QueryRoleAttributeUsage returns the set of distinct role-attribute values
// that appear on any of the sources for kind, along with per-source counts
// (and optionally the backing entity ids when includeIds is true). The
// predicate/sort/limit/offset semantics match the existing
// QueryRoleAttributes endpoints: the predicate is evaluated against the
// attribute value under the symbol name `id`.
func QueryRoleAttributeUsage(env Env, kind RoleAttributeKind, queryString string, includeIds bool) ([]*RoleAttributeUsage, *models.QueryMetaData, error) {
	sources := sourcesFor(env, kind)
	if len(sources) == 0 {
		return nil, nil, errors.Errorf("unknown role attribute kind: %s", kind)
	}

	indexStore := env.GetStores().Index
	query, err := ast.Parse(indexStore, queryString)
	if err != nil {
		return nil, nil, err
	}

	cursorProvider := func(tx *bbolt.Tx, forward bool) ast.SetCursor {
		set := ast.NewTreeSet(forward)
		seen := make(map[string]struct{})
		for _, src := range sources {
			if src.index == nil {
				continue
			}
			src.index.ReadKeys(tx, func(val []byte) {
				if _, ok := seen[string(val)]; ok {
					return
				}
				seen[string(val)] = struct{}{}
				set.Add(append([]byte(nil), val...))
			})
		}
		// An empty TreeSet has a nil root; TreeSet.ToCursor would panic
		// dereferencing it. A kind with no role attributes (fresh or sparse
		// controller) is a legitimate state, so return an empty cursor.
		if set.Size() == 0 {
			return ast.NewEmptyCursor()
		}
		return set.ToCursor()
	}

	// A single read transaction covers both the attribute query and the
	// per-source usage reads so the counts are guaranteed consistent with the
	// returned attribute list, even under concurrent writes.
	var count int64
	var results []*RoleAttributeUsage
	err = env.GetDb().View(func(tx *bbolt.Tx) error {
		attrs, c, err := indexStore.QueryWithCursorC(tx, cursorProvider, query)
		if err != nil {
			return err
		}
		count = c

		results = make([]*RoleAttributeUsage, 0, len(attrs))
		for _, attr := range attrs {
			usage := make(map[RoleAttributeSource]*RoleAttributeSourceUsage, len(sources))
			for _, src := range sources {
				entry := &RoleAttributeSourceUsage{}
				if src.index != nil {
					src.index.Read(tx, []byte(attr), func(val []byte) {
						entry.Count++
						if includeIds {
							entry.Ids = append(entry.Ids, string(val))
						}
					})
				}
				usage[src.name] = entry
			}
			results = append(results, &RoleAttributeUsage{
				RoleAttribute: attr,
				Usage:         usage,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	qmd := &models.QueryMetaData{
		Count:            count,
		Limit:            *query.GetLimit(),
		Offset:           *query.GetSkip(),
		FilterableFields: indexStore.GetPublicSymbols(),
	}
	return results, qmd, nil
}
