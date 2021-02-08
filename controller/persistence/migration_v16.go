package persistence

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

func (m *Migrations) removeOrphanedOttCaEnrollments(step *boltz.MigrationStep) {

	var enrollmentsToDelete []string

	filter, err := ast.Parse(m.stores.Enrollment, "true")

	if err != nil {
		step.SetError(fmt.Errorf("could not parse query for removing orphaned ottca enrollments: %v", err))
		return
	}

	for cursor := m.stores.Enrollment.IterateIds(step.Ctx.Tx(), filter); cursor.IsValid(); cursor.Next() {
		current := cursor.Current()
		currentEnrollmentId := string(current)

		enrollment, err := m.stores.Enrollment.LoadOneById(step.Ctx.Tx(), currentEnrollmentId)

		if err != nil {
			step.SetError(fmt.Errorf("error interating ids of enrollments, enrollment [%s]: %v", currentEnrollmentId, err))
			return
		}

		if enrollment.CaId != nil && *enrollment.CaId != "" {
			_, err := m.stores.Ca.LoadOneById(step.Ctx.Tx(), *enrollment.CaId)

			if err != nil && boltz.IsErrNotFoundErr(err) {
				enrollmentsToDelete = append(enrollmentsToDelete, currentEnrollmentId)
			}
		}
	}

	//clear caIds that are invalid via CheckIntegrity
	m.stores.Enrollment.CheckIntegrity(step.Ctx.Tx(), true, func(err error, fixed bool) {
		if !fixed {
			pfxlog.Logger().Errorf("unfixable error during orphaned ottca enrollment integrity check: %v", err)
		}
	})

	for _, enrollmentId := range enrollmentsToDelete {
		pfxlog.Logger().Infof("removing invalid ottca enrollment [%s]", enrollmentId)
		if err := m.stores.Enrollment.DeleteById(step.Ctx, enrollmentId); err != nil {

			step.SetError(fmt.Errorf("could not delete enrollment [%s] with invalid CA reference: %v", enrollmentId, err))
		}
	}
}
