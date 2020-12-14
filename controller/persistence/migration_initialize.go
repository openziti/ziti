/*
	Copyright NetFoundry, Inc.

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

package persistence

import (
	"fmt"
	"github.com/openziti/foundation/storage/boltz"
	"math"
	"time"
)

func (m *Migrations) initialize(step *boltz.MigrationStep) int {
	versionBucket := boltz.GetOrCreatePath(step.Ctx.Tx(), boltz.RootBucket)
	if step.SetError(versionBucket.GetError()) {
		return 0
	}
	version := versionBucket.GetInt64(FieldVersion)
	if version != nil && *version > 0 {
		return int(*version)
	}

	m.createGeoRegionsV1(step)
	m.createIdentityTypesV1(step)
	m.createInitialTunnelerConfigTypes(step)
	m.addPostureCheckTypes(step)
	m.createInterceptV1ConfigType(step)
	m.createHostV1ConfigType(step)

	return CurrentDbVersion
}

var geoRegionsV1 = map[string]string{
	"a0e2c29f-9922-4435-a8a7-5dbf7bd92377": "Canada Central",
	"ac469973-105c-4de1-9f31-fffc077487fb": "US West",
	"2360479d-cc08-4224-bd56-43baa672af30": "Japan",
	"edd0680f-3ab4-49e6-9db5-c68258ba480d": "Australia",
	"521a3db9-8140-4854-a782-61cb5d3fe043": "South America",
	"8342acad-4e49-4098-84de-829feb55d350": "Korea",
	"b5562b15-ffeb-4910-bf14-a03067f9ca2e": "US Midwest",
	"6efe28a5-744e-464d-b147-4072efb769f0": "Canada West",
	"10c6f648-92b7-49e2-be96-f62357ea572f": "Europe West",
	"72946251-1fc7-4b3b-8568-c59d4723e704": "Africa",
	"70438d63-97b3-48b2-aeb5-b066a9526456": "Europe East",
	"e339d699-7f51-4e9c-a2ca-81720e07f196": "US South",
	"00586703-6748-4c78-890d-efa216f21ef3": "Canada East",
	"63f7200b-7794-4a68-92aa-a36ed338ecba": "Asia",
	"f91ecca3-6f82-4c7b-8191-ea0036ce7b5a": "US East",
	"41929e78-6674-4708-89d2-9b934ea96822": "Middle East",
	"5d0042bb-6fd5-4959-90a7-6bca70e23f76": "US Central",
}

func (m *Migrations) createGeoRegionsV1(step *boltz.MigrationStep) {
	for id, name := range geoRegionsV1 {
		step.SetError(m.stores.GeoRegion.Create(step.Ctx, &GeoRegion{
			BaseExtEntity: *boltz.NewExtEntity(id, nil),
			Name:          name,
		}))
	}
}

var IdentityTypesV1 = map[string]string{
	"User":    "User",
	"Device":  "Device",
	"Service": "Service",
	"Router":  "Router",
}

func (m *Migrations) createIdentityTypesV1(step *boltz.MigrationStep) {
	for id, name := range IdentityTypesV1 {
		step.SetError(m.stores.IdentityType.Create(step.Ctx, &IdentityType{
			BaseExtEntity: *boltz.NewExtEntity(id, nil),
			Name:          name,
		}))
	}
}

var clientConfigV1TypeId = "f2dd2df0-9c04-4b84-a91e-71437ac229f1"
var serverConfigV1TypeId = "cea49285-6c07-42cf-9f52-09a9b115c783"

var serverConfigTypeV1 = &ConfigType{
	BaseExtEntity: boltz.BaseExtEntity{Id: serverConfigV1TypeId},
	Name:          "ziti-tunneler-server.v1",
	Schema: map[string]interface{}{
		"$id":                  "http://edge.openziti.org/schemas/ziti-tunneler-server.v1.config.json",
		"type":                 "object",
		"additionalProperties": false,
		"definitions": map[string]interface{}{
			"duration": map[string]interface{}{
				"type":    "string",
				"pattern": "[0-9]+(h|m|s|ms)",
			},
			"method": map[string]interface{}{
				"type": "string",
				"enum": []interface{}{
					"GET",
					"POST",
					"PUT",
					"PATCH",
				},
			},
			"action": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required": []interface{}{
					"trigger",
					"action",
				},
				"properties": map[string]interface{}{
					"trigger": map[string]interface{}{
						"type": "string",
						"enum": []interface{}{
							"fail",
							"pass",
						},
					},
					"consecutiveEvents": map[string]interface{}{
						"type":    "integer",
						"minimum": float64(0),
						"maximum": float64(math.MaxUint16),
					},
					"duration": map[string]interface{}{
						"$ref": "#/definitions/duration",
					},
					"action": map[string]interface{}{
						"type":    "string",
						"pattern": "(mark (un)?healthy|increase cost [0-9]+|decrease cost [0-9]+)",
					},
				},
			},
			"actionList": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"$ref": "#/definitions/action",
				},
				"minItems": 1,
				"maxItems": 20,
			},
			"portCheck": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required": []interface{}{
					"interval",
					"timeout",
					"address",
				},
				"properties": map[string]interface{}{
					"interval": map[string]interface{}{"$ref": "#/definitions/duration"},
					"timeout":  map[string]interface{}{"$ref": "#/definitions/duration"},
					"address":  map[string]interface{}{"type": "string"},
					"actions":  map[string]interface{}{"$ref": "#/definitions/actionList"},
				},
			},
			"httpCheck": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"required": []interface{}{
					"interval",
					"timeout",
					"url",
				},
				"properties": map[string]interface{}{
					"url":      map[string]interface{}{"type": "string"},
					"method":   map[string]interface{}{"$ref": "#/definitions/method"},
					"body":     map[string]interface{}{"type": "string"},
					"interval": map[string]interface{}{"$ref": "#/definitions/duration"},
					"timeout":  map[string]interface{}{"$ref": "#/definitions/duration"},
					"actions":  map[string]interface{}{"$ref": "#/definitions/actionList"},
					"expectStatus": map[string]interface{}{
						"type":    "integer",
						"minimum": float64(100),
						"maximum": float64(599),
					},
					"expectInBody": map[string]interface{}{"type": "string"},
				},
			},
			"portCheckList": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"$ref": "#/definitions/portCheck",
				},
			},
			"httpCheckList": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"$ref": "#/definitions/httpCheck",
				},
			},
		},
		"required": []interface{}{
			"hostname",
			"port",
		},
		"properties": map[string]interface{}{
			"protocol": map[string]interface{}{
				"type": []interface{}{
					"string",
					"null",
				},
				"enum": []interface{}{
					"tcp",
					"udp",
				},
			},
			"hostname": map[string]interface{}{
				"type": "string",
			},
			"port": map[string]interface{}{
				"type":    "integer",
				"minimum": float64(0),
				"maximum": float64(math.MaxUint16),
			},
			"portChecks": map[string]interface{}{
				"$ref": "#/definitions/portCheckList",
			},
			"httpChecks": map[string]interface{}{
				"$ref": "#/definitions/httpCheckList",
			},
		},
	},
}

func (m *Migrations) createInitialTunnelerConfigTypes(step *boltz.MigrationStep) {
	clientConfigTypeV1 := &ConfigType{
		BaseExtEntity: boltz.BaseExtEntity{Id: clientConfigV1TypeId},
		Name:          "ziti-tunneler-client.v1",
		Schema: map[string]interface{}{
			"$id":                  "http://edge.openziti.org/schemas/ziti-tunneler-client.v1.config.json",
			"type":                 "object",
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				"hostname": map[string]interface{}{
					"type": "string",
				},
				"port": map[string]interface{}{
					"type":    "integer",
					"minimum": float64(0),
					"maximum": float64(math.MaxUint16),
				},
			},
		},
	}
	step.SetError(m.stores.ConfigType.Create(step.Ctx, clientConfigTypeV1))
	step.SetError(m.stores.ConfigType.Create(step.Ctx, serverConfigTypeV1))
}

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
