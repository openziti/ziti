package persistence

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/storage/ast"
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

			name := edgeRouterBucket.GetStringOrError(FieldName)
			createdAt := edgeRouterBucket.GetTimeOrError(boltz.FieldCreatedAt)
			updatedAt := edgeRouterBucket.GetTimeOrError(boltz.FieldUpdatedAt)
			tags := edgeRouterBucket.GetMap(boltz.FieldTags)

			router = &db.Router{
				BaseExtEntity: boltz.BaseExtEntity{
					Id:        routerId,
					CreatedAt: createdAt,
					UpdatedAt: updatedAt,
					Tags:      tags,
					Migrate:   true,
				},
				Name:        name,
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
		log.Debugf("moving edgeRouters for %v %v to routers", store.GetSingularEntityType(), id)
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

func (m *Migrations) moveTransitRouters(step *boltz.MigrationStep) {
	log := pfxlog.Logger()
	copyAll := func([]string) bool {
		return true
	}
	store := m.stores.TransitRouter
	for cursor := store.IterateValidIds(step.Ctx.Tx(), ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		id := cursor.Current()
		log.Debugf("moving transitRouter data for %v to %v bucket", string(id), TransitRouterPath)
		routerBucket := store.GetEntityBucket(step.Ctx.Tx(), id)
		edgeBucket := routerBucket.GetBucket(EdgeBucket)
		if edgeBucket == nil {
			continue
		}
		transitRouterBucket := routerBucket.GetOrCreateBucket(TransitRouterPath)
		if step.SetError(transitRouterBucket.GetError()) {
			return
		}
		if step.SetError(transitRouterBucket.Copy(edgeBucket, copyAll)) {
			return
		}
		routerBucket.DeleteEntity(EdgeBucket)
		if step.SetError(routerBucket.GetError()) {
			return
		}
	}
}

func (m *Migrations) copyNamesToParent(step *boltz.MigrationStep, store boltz.CrudStore) {
	ids, _, err := store.QueryIds(step.Ctx.Tx(), "true")
	if step.SetError(err) {
		return
	}

	log := pfxlog.Logger()

	for _, id := range ids {
		log.Debugf("copying %v edge name to fabric for id: %v", store.GetSingularEntityType(), id)
		edgeBucket := store.GetEntityBucket(step.Ctx.Tx(), []byte(id))
		parentBucket := store.GetParentStore().GetEntityBucket(step.Ctx.Tx(), []byte(id))

		name := edgeBucket.GetString(FieldName)
		if name != nil {
			parentBucket.SetString(FieldName, *name, nil)
			log.Debugf("copied %v edge name of %v to fabric for id %v", store.GetSingularEntityType(), *name, id)
		}
	}
}

func (m *Migrations) fixAuthenticatorCertFingerprints(step *boltz.MigrationStep) {
	ids, _, err := m.stores.Authenticator.QueryIds(step.Ctx.Tx(), "true")
	if step.SetError(err) {
		return
	}
	for _, id := range ids {
		bucket := m.stores.Authenticator.GetEntityBucket(step.Ctx.Tx(), []byte(id))
		fp := bucket.GetString(FieldAuthenticatorCertFingerprint)
		if fp != nil && len(*fp) > 0 {
			updateFp := strings.Replace(strings.ToLower(*fp), ":", "", -1)
			bucket.SetString(FieldAuthenticatorCertFingerprint, updateFp, nil)
			if step.SetError(bucket.GetError()) {
				return
			}
		}
	}
}
