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

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/openziti/ziti/v2/controller/xt"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	"go.etcd.io/bbolt"
)

// Snapshot types for deterministic JSON output

type snapshotService struct {
	Id                 string   `json:"id"`
	Name               string   `json:"name"`
	TerminatorStrategy string   `json:"terminatorStrategy"`
	MaxIdleTime        int64    `json:"maxIdleTime"`
	IsFabricOnly       bool     `json:"isFabricOnly"`
	EncryptionRequired bool     `json:"encryptionRequired"`
	RoleAttributes     []string `json:"roleAttributes"`
	Configs            []string `json:"configs"`
	Terminators        []string `json:"terminators"`
	// Denormalized links (should be empty for fabric-only)
	DialIdentities    []string `json:"dialIdentities"`
	BindIdentities    []string `json:"bindIdentities"`
	EdgeRouters       []string `json:"edgeRouters"`
	ServicePolicies   []string `json:"servicePolicies"`
	ServiceERPolicies []string `json:"serviceEdgeRouterPolicies"`
	// identityServices back-link set; moved out of the edge sub-bucket by the v45 migration.
	IdentityServices []string `json:"identityServices"`
	// Forward refcounted denorm link counts (relatedId -> count), read raw from the bbolt buckets.
	// These prove M4: a migration that corrupts a count (e.g. 2 -> 1) keeps the same ids and access
	// but would drop access on a later single-policy removal, which id-only snapshots miss.
	DialIdentityCounts map[string]int32 `json:"dialIdentityCounts"`
	BindIdentityCounts map[string]int32 `json:"bindIdentityCounts"`
	EdgeRouterCounts   map[string]int32 `json:"edgeRouterCounts"`
}

type snapshotIdentity struct {
	Id             string   `json:"id"`
	Name           string   `json:"name"`
	IsAdmin        bool     `json:"isAdmin"`
	RoleAttributes []string `json:"roleAttributes"`
	DialServices   []string `json:"dialServices"`
	BindServices   []string `json:"bindServices"`
	// ServiceConfigs: serviceId -> configTypeId -> configId. Per-identity config overrides; the
	// override data lives in the identity bucket and must survive the service migration intact.
	ServiceConfigs map[string]map[string]string `json:"serviceConfigs"`
	// Reverse refcounted denorm link counts (serviceId -> count). The reverse side lives on the
	// identity bucket (untouched by the migration); it must match the forward side on the service.
	DialServiceCounts map[string]int32 `json:"dialServiceCounts"`
	BindServiceCounts map[string]int32 `json:"bindServiceCounts"`
}

type snapshotConfig struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	// Services referencing this config (plain FK link, not refcounted).
	Services []string `json:"services"`
	// identityServices compound links (identity+service pairs that override this config).
	IdentityServices []string `json:"identityServices"`
}

type snapshotServicePolicy struct {
	Id            string   `json:"id"`
	Name          string   `json:"name"`
	PolicyType    string   `json:"policyType"`
	Semantic      string   `json:"semantic"`
	IdentityRoles []string `json:"identityRoles"`
	ServiceRoles  []string `json:"serviceRoles"`
	Services      []string `json:"services"`
	Identities    []string `json:"identities"`
}

type snapshotSERP struct {
	Id              string   `json:"id"`
	Name            string   `json:"name"`
	Semantic        string   `json:"semantic"`
	ServiceRoles    []string `json:"serviceRoles"`
	EdgeRouterRoles []string `json:"edgeRouterRoles"`
	Services        []string `json:"services"`
	EdgeRouters     []string `json:"edgeRouters"`
}

type snapshotEdgeRouter struct {
	Id             string   `json:"id"`
	Name           string   `json:"name"`
	RoleAttributes []string `json:"roleAttributes"`
	Services       []string `json:"services"`
	// Reverse (edge-router -> service) refcounts. The edge router is a child store of routers, so its
	// services field lives at routers/<id>/edge/services.
	ServiceCounts map[string]int32 `json:"serviceCounts"`
}

type snapshotTerminator struct {
	Id      string `json:"id"`
	Service string `json:"service"`
	Router  string `json:"router"`
	Binding string `json:"binding"`
	Address string `json:"address"`
}

type policyEvalEntry struct {
	IdentityId string `json:"identityId"`
	ServiceId  string `json:"serviceId"`
	IsDialable bool   `json:"isDialable"`
	IsBindable bool   `json:"isBindable"`
}

type snapshot struct {
	FabricServices            []snapshotService       `json:"fabricServices"`
	EdgeServices              []snapshotService       `json:"edgeServices"`
	Identities                []snapshotIdentity      `json:"identities"`
	Configs                   []snapshotConfig        `json:"configs"`
	ServicePolicies           []snapshotServicePolicy `json:"servicePolicies"`
	ServiceEdgeRouterPolicies []snapshotSERP          `json:"serviceEdgeRouterPolicies"`
	EdgeRouters               []snapshotEdgeRouter    `json:"edgeRouters"`
	Terminators               []snapshotTerminator    `json:"terminators"`
	PolicyEvaluation          []policyEvalEntry       `json:"policyEvaluation"`
	// RoleAttributeIndexQueries maps a role-attribute value to the edge service ids returned by an
	// index-backed `anyOf(roleAttributes) = "<value>"` query. This exercises the role-attributes set
	// index (rebuilt by the v45 migration) rather than the entity field, so a missing/empty index is
	// caught instead of silently passing.
	RoleAttributeIndexQueries map[string][]string `json:"roleAttributeIndexQueries"`
}

func runQuery(dbPath, outputPath string) error {
	xt.GlobalRegistry().RegisterFactory(xt_smartrouting.NewFactory())

	boltDb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	defer boltDb.Close()

	stores, err := db.InitStores(boltDb, command.NoOpRateLimiter{}, nil)
	if err != nil {
		return fmt.Errorf("failed to init stores: %w", err)
	}

	if err := db.RunMigrations(boltDb, stores, nil); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	closeNotify := make(chan struct{})
	defer close(closeNotify)
	if err := stores.EventualEventer.Start(closeNotify); err != nil {
		return fmt.Errorf("failed to start eventual eventer: %w", err)
	}

	snap, err := querySnapshot(boltDb, stores)
	if err != nil {
		return fmt.Errorf("failed to query: %w", err)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	fmt.Printf("Snapshot written to %s (%d bytes)\n", outputPath, len(data))
	fmt.Printf("  Fabric services: %d\n", len(snap.FabricServices))
	fmt.Printf("  Edge services: %d\n", len(snap.EdgeServices))
	fmt.Printf("  Identities: %d\n", len(snap.Identities))
	fmt.Printf("  Service policies: %d\n", len(snap.ServicePolicies))
	fmt.Printf("  Service edge router policies: %d\n", len(snap.ServiceEdgeRouterPolicies))
	fmt.Printf("  Edge routers: %d\n", len(snap.EdgeRouters))
	fmt.Printf("  Terminators: %d\n", len(snap.Terminators))
	fmt.Printf("  Policy evaluations: %d\n", len(snap.PolicyEvaluation))
	return nil
}

func querySnapshot(boltDb boltz.Db, stores *db.Stores) (*snapshot, error) {
	snap := &snapshot{}

	err := boltDb.View(func(tx *bbolt.Tx) error {
		// Query all services
		serviceIds, _, err := stores.Service.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query services: %w", err)
		}
		sort.Strings(serviceIds)

		for _, id := range serviceIds {
			svc, _, err := stores.Service.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load service %s: %w", id, err)
			}

			s := snapshotService{
				Id:                 svc.Id,
				Name:               svc.Name,
				TerminatorStrategy: svc.TerminatorStrategy,
				MaxIdleTime:        int64(svc.MaxIdleTime),
				IsFabricOnly:       svc.IsFabricOnly,
				EncryptionRequired: svc.EncryptionRequired,
				RoleAttributes:     sortedOrEmpty(svc.RoleAttributes),
				Configs:            sortedOrEmpty(svc.Configs),
				Terminators:        sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.EntityTypeTerminators)),
				DialIdentities:     sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.FieldEdgeServiceDialIdentities)),
				BindIdentities:     sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.FieldEdgeServiceBindIdentities)),
				EdgeRouters:        sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.FieldEdgeRouters)),
				ServicePolicies:    sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.EntityTypeServicePolicies)),
				ServiceERPolicies:  sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.EntityTypeServiceEdgeRouterPolicies)),
				IdentityServices:   sortedOrEmpty(stores.Service.GetRelatedEntitiesIdList(tx, id, db.FieldServiceIdentityService)),
			}
			s.DialIdentityCounts = linkCounts(tx, s.DialIdentities, db.RootBucket, db.EntityTypeServices, id, db.FieldEdgeServiceDialIdentities)
			s.BindIdentityCounts = linkCounts(tx, s.BindIdentities, db.RootBucket, db.EntityTypeServices, id, db.FieldEdgeServiceBindIdentities)
			s.EdgeRouterCounts = linkCounts(tx, s.EdgeRouters, db.RootBucket, db.EntityTypeServices, id, db.FieldEdgeRouters)

			if svc.IsFabricOnly {
				snap.FabricServices = append(snap.FabricServices, s)
			} else {
				snap.EdgeServices = append(snap.EdgeServices, s)
			}
		}

		// Query identities
		identityIds, _, err := stores.Identity.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query identities: %w", err)
		}
		sort.Strings(identityIds)

		for _, id := range identityIds {
			ident, _, err := stores.Identity.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load identity %s: %w", id, err)
			}

			dialServices := sortedOrEmpty(stores.Identity.GetRelatedEntitiesIdList(tx, id, db.FieldIdentityDialServices))
			bindServices := sortedOrEmpty(stores.Identity.GetRelatedEntitiesIdList(tx, id, db.FieldIdentityBindServices))
			snap.Identities = append(snap.Identities, snapshotIdentity{
				Id:                ident.Id,
				Name:              ident.Name,
				IsAdmin:           ident.IsAdmin,
				RoleAttributes:    sortedOrEmpty(ident.RoleAttributes),
				DialServices:      dialServices,
				BindServices:      bindServices,
				ServiceConfigs:    ident.ServiceConfigs,
				DialServiceCounts: linkCounts(tx, dialServices, db.RootBucket, db.EntityTypeIdentities, id, db.FieldIdentityDialServices),
				BindServiceCounts: linkCounts(tx, bindServices, db.RootBucket, db.EntityTypeIdentities, id, db.FieldIdentityBindServices),
			})
		}

		// Configs (captures the identityServices compound links that track per-identity overrides)
		configIds, _, err := stores.Config.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query configs: %w", err)
		}
		sort.Strings(configIds)

		for _, id := range configIds {
			config, _, err := stores.Config.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load config %s: %w", id, err)
			}
			snap.Configs = append(snap.Configs, snapshotConfig{
				Id:               config.Id,
				Name:             config.Name,
				Services:         sortedOrEmpty(stores.Config.GetRelatedEntitiesIdList(tx, id, db.EntityTypeServices)),
				IdentityServices: sortedOrEmpty(stores.Config.GetRelatedEntitiesIdList(tx, id, db.FieldConfigIdentityService)),
			})
		}

		// Query service policies
		spIds, _, err := stores.ServicePolicy.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query service policies: %w", err)
		}
		sort.Strings(spIds)

		for _, id := range spIds {
			sp, _, err := stores.ServicePolicy.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load service policy %s: %w", id, err)
			}

			snap.ServicePolicies = append(snap.ServicePolicies, snapshotServicePolicy{
				Id:            sp.Id,
				Name:          sp.Name,
				PolicyType:    string(sp.PolicyType),
				Semantic:      sp.Semantic,
				IdentityRoles: sortedOrEmpty(sp.IdentityRoles),
				ServiceRoles:  sortedOrEmpty(sp.ServiceRoles),
				Services:      sortedOrEmpty(stores.ServicePolicy.GetRelatedEntitiesIdList(tx, id, db.EntityTypeServices)),
				Identities:    sortedOrEmpty(stores.ServicePolicy.GetRelatedEntitiesIdList(tx, id, db.EntityTypeIdentities)),
			})
		}

		// Query service edge router policies
		serpIds, _, err := stores.ServiceEdgeRouterPolicy.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query SERPs: %w", err)
		}
		sort.Strings(serpIds)

		for _, id := range serpIds {
			serp, _, err := stores.ServiceEdgeRouterPolicy.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load SERP %s: %w", id, err)
			}

			snap.ServiceEdgeRouterPolicies = append(snap.ServiceEdgeRouterPolicies, snapshotSERP{
				Id:              serp.Id,
				Name:            serp.Name,
				Semantic:        serp.Semantic,
				ServiceRoles:    sortedOrEmpty(serp.ServiceRoles),
				EdgeRouterRoles: sortedOrEmpty(serp.EdgeRouterRoles),
				Services:        sortedOrEmpty(stores.ServiceEdgeRouterPolicy.GetRelatedEntitiesIdList(tx, id, db.EntityTypeServices)),
				EdgeRouters:     sortedOrEmpty(stores.ServiceEdgeRouterPolicy.GetRelatedEntitiesIdList(tx, id, db.EntityTypeRouters)),
			})
		}

		// Query edge routers
		erIds, _, err := stores.EdgeRouter.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query edge routers: %w", err)
		}
		sort.Strings(erIds)

		for _, id := range erIds {
			er, _, err := stores.EdgeRouter.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load edge router %s: %w", id, err)
			}

			erServices := sortedOrEmpty(stores.EdgeRouter.GetRelatedEntitiesIdList(tx, id, db.EntityTypeServices))
			snap.EdgeRouters = append(snap.EdgeRouters, snapshotEdgeRouter{
				Id:             er.Id,
				Name:           er.Name,
				RoleAttributes: sortedOrEmpty(er.RoleAttributes),
				Services:       erServices,
				ServiceCounts:  linkCounts(tx, erServices, db.RootBucket, db.EntityTypeRouters, id, db.EdgeBucket, db.EntityTypeServices),
			})
		}

		// Query terminators
		termIds, _, err := stores.Terminator.QueryIds(tx, "true limit none")
		if err != nil {
			return fmt.Errorf("failed to query terminators: %w", err)
		}
		sort.Strings(termIds)

		for _, id := range termIds {
			term, _, err := stores.Terminator.FindById(tx, id)
			if err != nil {
				return fmt.Errorf("failed to load terminator %s: %w", id, err)
			}

			snap.Terminators = append(snap.Terminators, snapshotTerminator{
				Id:      term.Id,
				Service: term.Service,
				Router:  term.Router,
				Binding: term.Binding,
				Address: term.Address,
			})
		}

		// Policy evaluation: for each identity x service, check dial/bind
		for _, identId := range identityIds {
			for _, svcId := range serviceIds {
				isDialable := stores.Service.IsDialableByIdentity(tx, svcId, identId)
				isBindable := stores.Service.IsBindableByIdentity(tx, svcId, identId)
				if isDialable || isBindable {
					snap.PolicyEvaluation = append(snap.PolicyEvaluation, policyEvalEntry{
						IdentityId: identId,
						ServiceId:  svcId,
						IsDialable: isDialable,
						IsBindable: isBindable,
					})
				}
			}
		}

		// Role-attribute index queries: read the role-attributes set index directly (not the entity
		// field) for each distinct role value, so the index rebuilt by the v45 migration is exercised.
		roleValues := map[string]struct{}{}
		for _, svc := range snap.EdgeServices {
			for _, r := range svc.RoleAttributes {
				roleValues[r] = struct{}{}
			}
		}
		roleIndex := stores.Service.GetRoleAttributesIndex()
		snap.RoleAttributeIndexQueries = map[string][]string{}
		for role := range roleValues {
			var ids []string
			roleIndex.Read(tx, []byte(role), func(val []byte) {
				ids = append(ids, string(val))
			})
			snap.RoleAttributeIndexQueries[role] = sortedOrEmpty(ids)
		}

		return nil
	})

	return snap, err
}

// linkCounts reads the refcounted denorm link counts directly from the bbolt bucket at the given
// path (no store API), returning relatedId -> count for the given related ids. The path is variadic
// so it can reach child-store fields (e.g. an edge router's services field lives at
// routers/<id>/edge/services).
func linkCounts(tx *bbolt.Tx, relatedIds []string, path ...string) map[string]int32 {
	result := map[string]int32{}
	bucket := boltz.Path(tx, path...)
	if bucket == nil {
		return result
	}
	for _, relatedId := range relatedIds {
		if c := bucket.GetLinkCount(boltz.TypeString, []byte(relatedId)); c != nil {
			result[relatedId] = *c
		}
	}
	return result
}

func sortedOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	sort.Strings(s)
	return s
}
