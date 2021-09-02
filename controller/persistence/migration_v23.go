package persistence

import (
	"github.com/openziti/foundation/storage/boltz"
	"time"
)

func (m *Migrations) addProcessMultiPostureCheck(step *boltz.MigrationStep) {
	windows := OperatingSystem{
		OsType:     "Windows",
		OsVersions: []string{},
	}

	linux := OperatingSystem{
		OsType:     "Linux",
		OsVersions: []string{},
	}

	macOS := OperatingSystem{
		OsType:     "macOS",
		OsVersions: []string{},
	}

	processCheckType := &PostureCheckOs{
		BaseExtEntity: boltz.BaseExtEntity{
			Id:        "PROCESS_MULTI",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			Tags:      map[string]interface{}{},
			Migrate:   false,
		},
		Name: "Process Multi Check",
		OperatingSystems: []OperatingSystem{
			windows,
			macOS,
			linux,
		},
	}

	if err := m.stores.PostureCheckType.Create(step.Ctx, processCheckType); err != nil {
		step.SetError(err)
		return
	}
}
