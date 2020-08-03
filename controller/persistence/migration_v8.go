package persistence

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/storage/boltz"
	log "github.com/sirupsen/logrus"
)

func (m *Migrations) denormalizePolicies(step *boltz.MigrationStep) {
	log := pfxlog.Logger()

	err := m.stores.EdgeRouterPolicy.CheckIntegrity(step.Ctx.Tx(), true, func(err error, fixed bool) {
		log.WithError(err).Debugf("updating edge router policies. Fixed? %v", fixed)
	})

	if step.SetError(err) {
		return
	}

	err = m.stores.ServiceEdgeRouterPolicy.CheckIntegrity(step.Ctx.Tx(), true, func(err error, fixed bool) {
		log.WithError(err).Debugf("updating service edge router policies. Fixed? %v", fixed)
	})

	if step.SetError(err) {
		return
	}

	err = m.stores.ServicePolicy.CheckIntegrity(step.Ctx.Tx(), true, func(err error, fixed bool) {
		log.WithError(err).Debugf("updating service policies. Fixed? %v", fixed)
	})

	if step.SetError(err) {
		return
	}
}

func (m *Migrations) fixNameIndices(step *boltz.MigrationStep) {
	c := m.stores.Service.GetNameIndex().(boltz.Constraint)
	step.SetError(c.CheckIntegrity(step.Ctx.Tx(), true, func(err error, fixed bool) {
		log.WithError(err).Debugf("Fixing service name index. Fixed? %v", fixed)
	}))

	c = m.stores.Router.GetNameIndex().(boltz.Constraint)
	step.SetError(c.CheckIntegrity(step.Ctx.Tx(), true, func(err error, fixed bool) {
		log.WithError(err).Debugf("Fixing router name index. Fixed? %v", fixed)
	}))
}
