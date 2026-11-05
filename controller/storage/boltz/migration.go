package boltz

import (
	"github.com/openziti/foundation/v2/errorz"
)

type MigrationStep struct {
	errorz.ErrorHolderImpl
	Component      string
	Ctx            MutateContext
	CurrentVersion int
}
