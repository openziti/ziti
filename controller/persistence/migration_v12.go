package persistence

import (
	"fmt"
	"github.com/openziti/foundation/storage/boltz"
	"time"
)

func (m *Migrations) addPostureCheckTypes(step *boltz.MigrationStep) {

	_, count, err := m.stores.PostureCheckType.QueryIds(step.Ctx.Tx(), "true limit 500")

	if err != nil {
		step.SetError(fmt.Errorf("could not query posture check types: %v", err))
	}

	if count > 0 {
		return //already added
	}

	windows := OperatingSystem{
		OsType:     "Windows",
		OsVersions: []string{"Vista", "7", "8", "10", "2000"},
	}

	linux := OperatingSystem{
		OsType:     "Linux",
		OsVersions: []string{"4.14", "4.19", "5.4", "5.9"},
	}

	iOS := OperatingSystem{
		OsType:     "iOS",
		OsVersions: []string{"11", "12"},
	}

	macOS := OperatingSystem{
		OsType:     "macOS",
		OsVersions: []string{"10.15", "11.0"},
	}

	android := OperatingSystem{
		OsType:     "Android",
		OsVersions: []string{"9", "10", "11"},
	}

	allOS := []OperatingSystem{
		windows,
		linux,
		android,
		macOS,
		iOS,
	}

	types := []*PostureCheckType{
		{
			BaseExtEntity: boltz.BaseExtEntity{
				Id:        "OS",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tags:      map[string]interface{}{},
				Migrate:   false,
			},
			Name:             "Operating System Check",
			OperatingSystems: allOS,
		},
		{
			BaseExtEntity: boltz.BaseExtEntity{
				Id:        "PROCESS",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tags:      map[string]interface{}{},
				Migrate:   false,
			},
			Name: "Process Check",
			OperatingSystems: []OperatingSystem{
				windows,
				macOS,
				linux,
			},
		},
		{
			BaseExtEntity: boltz.BaseExtEntity{
				Id:        "DOMAIN",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tags:      map[string]interface{}{},
				Migrate:   false,
			},
			Name: "Windows Domain Check",
			OperatingSystems: []OperatingSystem{
				windows,
			},
		},
		{
			BaseExtEntity: boltz.BaseExtEntity{
				Id:        "MAC",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Tags:      map[string]interface{}{},
				Migrate:   false,
			},
			Name: "MAC Address Check",
			OperatingSystems: []OperatingSystem{
				windows,
				linux,
				macOS,
				android,
			},
		},
	}

	for _, postureCheckType := range types {
		if err := m.stores.PostureCheckType.Create(step.Ctx, postureCheckType); err != nil {
			step.SetError(err)
			return
		}
	}

}
