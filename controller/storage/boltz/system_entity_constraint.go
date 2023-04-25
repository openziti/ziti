package boltz

import (
	"github.com/openziti/foundation/v2/errorz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

func NewSystemEntityEnforcementConstraint(store Store) Constraint {
	symbol := store.GetSymbol(FieldIsSystemEntity)
	return &systemEntityConstraint{
		systemFlagSymbol: symbol,
	}
}

type systemEntityConstraint struct {
	systemFlagSymbol EntitySymbol
}

func (self *systemEntityConstraint) checkOperation(operation string, ctx *IndexingContext) error {
	t, val := self.systemFlagSymbol.Eval(ctx.Tx(), ctx.RowId)
	isSystem := FieldToBool(t, val)
	if (isSystem != nil && *isSystem) && !ctx.Ctx.IsSystemContext() {
		err := errors.Errorf("cannot %v system %v in a non-system context (id=%v)",
			operation, self.systemFlagSymbol.GetStore().GetSingularEntityType(), string(ctx.RowId))
		return err
	}
	return nil
}

func (self *systemEntityConstraint) ProcessBeforeUpdate(ctx *IndexingContext) {
	if !ctx.IsCreate {
		if err := self.checkOperation("update", ctx); err != nil {
			ctx.ErrHolder.SetError(errorz.NewEntityCanNotBeUpdatedFrom(err))
		}
	}
}

func (self *systemEntityConstraint) ProcessAfterUpdate(ctx *IndexingContext) {
	if ctx.IsCreate {
		if err := self.checkOperation("create", ctx); err != nil {
			ctx.ErrHolder.SetError(err)
		}
	}
}

func (self *systemEntityConstraint) ProcessBeforeDelete(ctx *IndexingContext) {
	if err := self.checkOperation("delete", ctx); err != nil {
		ctx.ErrHolder.SetError(errorz.NewEntityCanNotBeDeletedFrom(err))
	}
}

func (self *systemEntityConstraint) Initialize(*bbolt.Tx, errorz.ErrorHolder) {}

func (self *systemEntityConstraint) CheckIntegrity(MutateContext, bool, func(err error, fixed bool)) error {
	return nil
}
