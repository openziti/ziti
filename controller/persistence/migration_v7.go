package persistence

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/stringz"
	"github.com/pkg/errors"
	"strings"
)

const (
	migrationEntityTypeEdgeRouters = "edgeRouters"
)

func (m *Migrations) moveEdgeRoutersUnderFabricRouters(step *boltz.MigrationStep) {
	rootBucket := step.Ctx.Tx().Bucket([]byte(boltz.RootBucket))
	if rootBucket == nil {
		step.SetError(errors.New("root bucket not found!"))
		return
	}

	log := pfxlog.Logger()

	edgeRoutersBucket := rootBucket.Bucket([]byte(migrationEntityTypeEdgeRouters))
	if edgeRoutersBucket == nil {
		log.Debugf("old edge routers bucket not found. skipping edge router migration")
		return
	}

	cursor := edgeRoutersBucket.Cursor()

	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		routerId := string(key)
		log.Debugf("converting edge router %v", routerId)

		edgeRouterBucket := boltz.Path(step.Ctx.Tx(), boltz.RootBucket, migrationEntityTypeEdgeRouters, routerId)

		router, _ := m.stores.Router.LoadOneById(step.Ctx.Tx(), routerId)
		if router == nil {
			fingerprint := edgeRouterBucket.GetString("fingerprint")
			if fingerprint != nil {
				cfp := strings.Replace(strings.ToLower(*fingerprint), ":", "", -1)
				fingerprint = &cfp
			}

			createdAt := edgeRouterBucket.GetTimeOrError(boltz.FieldCreatedAt)
			updatedAt := edgeRouterBucket.GetTimeOrError(boltz.FieldUpdatedAt)
			tags := edgeRouterBucket.GetMap(boltz.FieldTags)

			router = &db.Router{
				BaseExtEntity: boltz.BaseExtEntity{
					Id: routerId,
					ExtEntityFields: boltz.ExtEntityFields{
						CreatedAt: createdAt,
						UpdatedAt: updatedAt,
						Tags:      tags,
						Migrate:   true,
					},
				},
				Fingerprint: fingerprint,
			}
			if step.SetError(m.stores.Router.Create(step.Ctx, router)) {
				return
			}
		}

		edgeBucket := boltz.GetOrCreatePath(step.Ctx.Tx(), boltz.RootBucket, db.EntityTypeRouters, routerId, EdgeBucket)
		if edgeBucket.HasError() {
			step.SetError(edgeBucket.Err)
			return
		}

		err := edgeBucket.Copy(edgeRouterBucket, func(path []string) bool {
			return !stringz.Contains([]string{boltz.FieldId, boltz.FieldCreatedAt, boltz.FieldUpdatedAt, boltz.FieldTags, db.FieldRouterFingerprint}, path[0])
		})

		if step.SetError(err) {
			return
		}
	}

	m.fixPolicyEdgeRouterReferences(step, m.stores.EdgeRouterPolicy)
	m.fixPolicyEdgeRouterReferences(step, m.stores.ServiceEdgeRouterPolicy)
}

func (m *Migrations) fixPolicyEdgeRouterReferences(step *boltz.MigrationStep, store boltz.CrudStore) {
	ids, _, err := store.QueryIds(step.Ctx.Tx(), "true")
	if step.SetError(err) {
		return
	}

	log := pfxlog.Logger()

	copyAll := func([]string) bool {
		return true
	}

	for _, id := range ids {
		log.Debugf("converting %v %v", store.GetSingularEntityType(), id)
		policyBucket := store.GetEntityBucket(step.Ctx.Tx(), []byte(id))
		edgeRoutersBucket := policyBucket.GetBucket(migrationEntityTypeEdgeRouters)
		if edgeRoutersBucket == nil {
			continue
		}
		routersBucket := policyBucket.GetOrCreateBucket(db.EntityTypeRouters)
		if step.SetError(routersBucket.GetError()) {
			return
		}
		if step.SetError(routersBucket.Copy(edgeRoutersBucket, copyAll)) {
			return
		}
	}
}
