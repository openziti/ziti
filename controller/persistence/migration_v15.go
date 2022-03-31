package persistence

import (
	"github.com/openziti/storage/boltz"
)

func (m *Migrations) updateServerV1Config(step *boltz.MigrationStep) {
	step.SetError(m.stores.ConfigType.Update(step.Ctx, serverConfigTypeV1, nil))
}
