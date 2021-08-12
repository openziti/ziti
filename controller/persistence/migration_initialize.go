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
	m.addProcessMultiPostureCheck(step)
	step.SetError(m.stores.ConfigType.Create(step.Ctx, hostV2ConfigType))

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
		"definitions":          healthCheckSchema["definitions"],
		"required": []interface{}{
			"hostname",
			"port",
		},
		"properties": combine(
			map[string]interface{}{
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
			},
			healthCheckSchema["properties"].(map[string]interface{}),
		),
	},
}

func combine(maps ...map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			_, alreadyPresent := merged[k]
			if alreadyPresent {
				panic(fmt.Sprintf("duplicate key '%s' in merged maps", k))
			}
			merged[k] = v
		}
	}
	return merged
}

var healthCheckSchema = map[string]interface{}{
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
						"change",
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
					"pattern": "(mark (un)?healthy|increase cost [0-9]+|decrease cost [0-9]+|send event)",
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
	"properties": map[string]interface{}{
		"portChecks": map[string]interface{}{
			"$ref": "#/definitions/portCheckList",
		},
		"httpChecks": map[string]interface{}{
			"$ref": "#/definitions/httpCheckList",
		},
	},
}

var hostV1Definitions = combine(healthCheckSchema["definitions"].(map[string]interface{}), tunnelDefinitions)
var tunnelDefinitions = map[string]interface{}{
	"ipAddressFormat": map[string]interface{}{
		"oneOf": []interface{}{
			map[string]interface{}{"format": "ipv4"},
			map[string]interface{}{"format": "ipv6"},
		},
	},
	"ipAddress": map[string]interface{}{
		"type": "string",
		"$ref": "#/definitions/ipAddressFormat",
	},
	"hostname": map[string]interface{}{
		"type":   "string",
		"format": "hostname",
		"not":    map[string]interface{}{"$ref": "#/definitions/ipAddressFormat"},
	},
	"cidr": map[string]interface{}{
		"type": "string",
		"oneOf": []interface{}{
			// JSON ipv4/ipv6 "format"s should work for cidrs also (see
			// https://json-schema.org/understanding-json-schema/reference/string.html),
			// but https://www.jsonschemavalidator.net disagreed, so using `pattern` instead.
			// Patterns taken from https://blog.markhatton.co.uk/2011/03/15/regular-expressions-for-ip-addresses-cidr-ranges-and-hostnames/
			map[string]interface{}{"pattern": "^(([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])\\.){3}([0-9]|[1-9][0-9]|1[0-9]{2}|2[0-4][0-9]|25[0-5])(\\/(3[0-2]|[1-2][0-9]|[0-9]))$"},
			map[string]interface{}{"pattern": "^s*((([0-9A-Fa-f]{1,4}:){7}([0-9A-Fa-f]{1,4}|:))|(([0-9A-Fa-f]{1,4}:){6}(:[0-9A-Fa-f]{1,4}|((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3})|:))|(([0-9A-Fa-f]{1,4}:){5}(((:[0-9A-Fa-f]{1,4}){1,2})|:((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3})|:))|(([0-9A-Fa-f]{1,4}:){4}(((:[0-9A-Fa-f]{1,4}){1,3})|((:[0-9A-Fa-f]{1,4})?:((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){3}(((:[0-9A-Fa-f]{1,4}){1,4})|((:[0-9A-Fa-f]{1,4}){0,2}:((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){2}(((:[0-9A-Fa-f]{1,4}){1,5})|((:[0-9A-Fa-f]{1,4}){0,3}:((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3}))|:))|(([0-9A-Fa-f]{1,4}:){1}(((:[0-9A-Fa-f]{1,4}){1,6})|((:[0-9A-Fa-f]{1,4}){0,4}:((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3}))|:))|(:(((:[0-9A-Fa-f]{1,4}){1,7})|((:[0-9A-Fa-f]{1,4}){0,5}:((25[0-5]|2[0-4]d|1dd|[1-9]?d)(.(25[0-5]|2[0-4]d|1dd|[1-9]?d)){3}))|:)))(%.+)?s*(\\/(12[0-8]|1[0-1][0-9]|[1-9][0-9]|[0-9]))$"},
		},
	},
	"dialAddress": map[string]interface{}{
		"oneOf": []interface{}{
			map[string]interface{}{"$ref": "#/definitions/ipAddress"},
			map[string]interface{}{"$ref": "#/definitions/hostname"},
		},
	},
	"listenAddress": map[string]interface{}{
		"oneOf": []interface{}{
			map[string]interface{}{"$ref": "#/definitions/ipAddress"},
			map[string]interface{}{"$ref": "#/definitions/hostname"},
			map[string]interface{}{"$ref": "#/definitions/cidr"},
		},
	},
	"inhabitedSet": map[string]interface{}{
		"type":        "array",
		"minItems":    1,
		"uniqueItems": true,
	},
	"portNumber": map[string]interface{}{
		"type":    "integer",
		"minimum": float64(0),
		"maximum": float64(math.MaxUint16),
	},
	"portRange": map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"low":  map[string]interface{}{"$ref": "#/definitions/portNumber"},
			"high": map[string]interface{}{"$ref": "#/definitions/portNumber"},
		},
		"required": []interface{}{"low", "high"},
	},
	"protocolName": map[string]interface{}{
		"type": "string",
		"enum": []interface{}{"tcp", "udp"},
	},
	"timeoutSeconds": map[string]interface{}{
		"type":    "integer",
		"minimum": float64(0),
		"maximum": float64(math.MaxInt32),
	},
}

// hostV1 schema with ["$id"] and ["definitions"] excluded
var hostV1SchemaSansDefs = map[string]interface{}{
	"type": "object",
	"properties": combine(healthCheckSchema["properties"].(map[string]interface{}),
		map[string]interface{}{
			"protocol": map[string]interface{}{
				"$ref":        "#/definitions/protocolName",
				"description": "Dial the specified protocol when a ziti client connects to the service.",
			},
			"forwardProtocol": map[string]interface{}{
				"type":        "boolean",
				"enum":        []interface{}{true},
				"description": "Dial the same protocol that was intercepted at the client tunneler. 'protocol' and 'forwardProtocol' are mutually exclusive.",
			},
			"allowedProtocols": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/protocolName"}},
				},
				"description": "Only allow protocols from this set to be dialed",
			},
			"address": map[string]interface{}{
				"$ref":        "#/definitions/dialAddress",
				"description": "Dial the specified ip address or hostname when a ziti client connects to the service.",
			},
			"forwardAddress": map[string]interface{}{
				"type":        "boolean",
				"enum":        []interface{}{true},
				"description": "Dial the same ip address that was intercepted at the client tunneler. 'address' and 'forwardAddress' are mutually exclusive.",
			},
			"allowedAddresses": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/listenAddress"}},
				},
				"description": "Only allow addresses from this set to be dialed",
			},
			"port": map[string]interface{}{
				"$ref":        "#/definitions/portNumber",
				"description": "Dial the specified port when a ziti client connects to the service.",
			},
			"forwardPort": map[string]interface{}{
				"type":        "boolean",
				"enum":        []interface{}{true},
				"description": "Dial the same port that was intercepted at the client tunneler. 'port' and 'forwardPort' are mutually exclusive.",
			},
			"allowedPortRanges": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/portRange"}},
				},
				"description": "Only allow ports from this set to be dialed",
			},
			"allowedSourceAddresses": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/listenAddress"}},
				},
				"description": "hosting tunnelers establish local routes for the specified source addresses so binding will succeed",
			},
			"listenOptions": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"connectTimeoutSeconds": map[string]interface{}{
						"$ref":        "#/definitions/timeoutSeconds",
						"description": "defaults to 5",
					},
					"maxConnections": map[string]interface{}{
						"type":        "integer",
						"minimum":     1,
						"description": "defaults to 3",
					},
					"identity": map[string]interface{}{
						"type":        "string",
						"description": "Associate the hosting terminator with the specified identity. '$tunneler_id.name' resolves to the name of the hosting tunneler's identity. '$tunneler_id.tag[tagName]' resolves to the value of the 'tagName' tag on the hosting tunneler's identity.",
					},
					"bindUsingEdgeIdentity": map[string]interface{}{
						"type":        "boolean",
						"description": "Associate the hosting terminator with the name of the hosting tunneler's identity. Setting this to 'true' is equivalent to setting 'identiy=$tunneler_id.name'",
					},
				},
			},
		},
	),
	"additionalProperties": false,
	"allOf": []interface{}{
		map[string]interface{}{
			"if": map[string]interface{}{
				"properties": map[string]interface{}{
					"forwardProtocol": map[string]interface{}{"const": true},
				},
				"required": []interface{}{"forwardProtocol"},
			},
			"then": map[string]interface{}{
				"required": []interface{}{"allowedProtocols"},
			},
			"else": map[string]interface{}{
				"required": []interface{}{"protocol"},
			},
		},
		map[string]interface{}{
			"if": map[string]interface{}{
				"properties": map[string]interface{}{
					"forwardAddress": map[string]interface{}{"const": true},
				},
				"required": []interface{}{"forwardAddress"},
			},
			"then": map[string]interface{}{
				"required": []interface{}{"allowedAddresses"},
			},
			"else": map[string]interface{}{
				"required": []interface{}{"address"},
			},
		},
		map[string]interface{}{
			"if": map[string]interface{}{
				"properties": map[string]interface{}{
					"forwardPort": map[string]interface{}{"const": true},
				},
				"required": []interface{}{"forwardPort"},
			},
			"then": map[string]interface{}{
				"required": []interface{}{"allowedPortRanges"},
			},
			"else": map[string]interface{}{
				"required": []interface{}{"port"},
			},
		},
	},
}

var hostV1ConfigTypeId = "NH5p4FpGR"
var hostV1ConfigType = &ConfigType{
	BaseExtEntity: boltz.BaseExtEntity{Id: hostV1ConfigTypeId},
	Name:          "host.v1",
	Schema: combine(
		map[string]interface{}{
			"$id":         "http://ziti-edge.netfoundry.io/schemas/host.v1.schema.json",
			"definitions": hostV1Definitions,
		},
		hostV1SchemaSansDefs),
}

var hostV2ConfigTypeId = "host.v2"
var hostV2ConfigType = &ConfigType{
	BaseExtEntity: boltz.BaseExtEntity{Id: hostV2ConfigTypeId},
	Name:          "host.v2",
	Schema: map[string]interface{}{
		"$id": "http://ziti-edge.netfoundry.io/schemas/host.v2.schema.json",
		"definitions": combine(
			hostV1Definitions,
			map[string]interface{}{
				"terminator": hostV1SchemaSansDefs,
				"terminatorList": map[string]interface{}{
					"type":     "array",
					"minItems": 1,
					"items": map[string]interface{}{
						"$ref": "#/definitions/terminator",
					},
				},
			},
		),
		"type": "object",
		"properties": map[string]interface{}{
			"terminators": map[string]interface{}{
				"$ref": "#/definitions/terminatorList",
			},
		},
	},
}

var interceptV1ConfigType = &ConfigType{
	BaseExtEntity: boltz.BaseExtEntity{Id: "g7cIWbcGg"},
	Name:          "intercept.v1",
	Schema: map[string]interface{}{
		"$id":                  "http://edge.openziti.org/schemas/intercept.v1.config.json",
		"type":                 "object",
		"additionalProperties": false,
		"definitions":          tunnelDefinitions,
		"properties": map[string]interface{}{
			"protocols": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/protocolName"}},
				},
			},
			"addresses": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/listenAddress"}},
				},
			},
			"portRanges": map[string]interface{}{
				"allOf": []interface{}{
					map[string]interface{}{"$ref": "#/definitions/inhabitedSet"},
					map[string]interface{}{"items": map[string]interface{}{"$ref": "#/definitions/portRange"}},
				},
			},
			"dialOptions": map[string]interface{}{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]interface{}{
					"identity": map[string]interface{}{
						"type":        "string",
						"description": "Dial a terminator with the specified identity. '$dst_protocol', '$dst_ip', '$dst_port are resolved to the corresponding value of the destination address.",
					},
					"connectTimeoutSeconds": map[string]interface{}{
						"$ref":        "#/definitions/timeoutSeconds",
						"description": "defaults to 5 seconds if no dialOptions are defined. defaults to 15 if dialOptions are defined but connectTimeoutSeconds is not specified.",
					},
				},
			},
			"sourceIp": map[string]interface{}{
				"type":        "string",
				"description": "The source IP (and optional :port) to spoof when the connection is egressed from the hosting tunneler. '$tunneler_id.name' resolves to the name of the client tunneler's identity. '$tunneler_id.tag[tagName]' resolves to the value of the 'tagName' tag on the client tunneler's identity. '$src_ip' and '$src_port' resolve to the source IP / port of the originating client. '$dst_port' resolves to the port that the client is trying to connect.",
			},
		},
		"required": []interface{}{
			"protocols",
			"addresses",
			"portRanges",
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

	types := []*PostureCheckOs{
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
