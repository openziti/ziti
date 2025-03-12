/*
	Copyright NetFoundry Inc.

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

package db

import (
	"fmt"
	"github.com/openziti/storage/boltz"
	"math"
	"time"
)

func (m *Migrations) initialize(step *boltz.MigrationStep) int {
	versionBucket := boltz.GetOrCreatePath(step.Ctx.Tx(), RootBucket)
	if step.SetError(versionBucket.GetError()) {
		return 0
	}
	version := versionBucket.GetInt64(FieldVersion)
	if version != nil && *version > 0 {
		return int(*version)
	}

	m.createIdentityTypesV1(step)
	m.createInitialTunnelerConfigTypes(step)
	m.addPostureCheckTypes(step)
	m.createInterceptV1ConfigType(step)
	m.createHostV1ConfigType(step)
	m.addProcessMultiPostureCheck(step)
	step.SetError(m.stores.ConfigType.Create(step.Ctx, hostV2ConfigType))
	m.addSystemAuthPolicies(step)
	m.addGlobalControllerConfig(step)

	return CurrentDbVersion
}

var IdentityTypesV1 = map[string]string{
	"Default": "Default",
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
	"dialAddress": map[string]interface{}{
		"type":   "string",
		"format": "idn-hostname",
		"not":    map[string]interface{}{"pattern": "^$"},
	},
	"listenAddress": map[string]interface{}{
		"type":        "string",
		"format":      "idn-hostname",
		"not":         map[string]interface{}{"pattern": "^$"},
		"description": "idn-hostname allows ipv4 and ipv6 addresses, as well as hostnames that might happen to contain '*' and/or '/'. so idn-hostname allows every supported intercept address, although ip addresses, wildcards and cidrs are only being validated as hostnames by this format. client applications will need to look for _valid_ ips, cidrs, and wildcards when parsing intercept addresses and treat them accordingly. anything else should be interpreted as a dns label. this means e.g. that '1.2.3.4/56' should be treated as a dns label, since it is not a valid cidr",
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
	"proxyType": map[string]interface{}{
		"type":        "string",
		"enum":        []interface{}{"http"},
		"description": "supported proxy types",
	},
	"proxyConfiguration": map[string]interface{}{
		"type": "object",
		"required": []interface{}{
			"type",
			"address",
		},
		"properties": map[string]interface{}{
			"type": map[string]interface{}{
				"$ref":        "#/definitions/proxyType",
				"description": "The type of the proxy being used",
			},
			"address": map[string]interface{}{
				"type":        "string",
				"description": "The address of the proxy in host:port format",
			},
		},
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
						"description": "Timeout when making outbound connections. Defaults to 5. If both connectTimoutSeconds and connectTimeout are specified, connectTimeout will be used.",
						"deprecated":  true,
					},
					"connectTimeout": map[string]interface{}{
						"$ref":        "#/definitions/duration",
						"description": "Timeout when making outbound connections. Defaults to '5s'. If both connectTimoutSeconds and connectTimeout are specified, connectTimeout will be used.",
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
					"cost": map[string]interface{}{
						"type":        "integer",
						"minimum":     0,
						"maximum":     65535,
						"description": "defaults to 0",
					},
					"precedence": map[string]interface{}{
						"type":        "string",
						"enum":        []interface{}{"default", "required", "failed"},
						"description": "defaults to 'default'",
					},
				},
			},
			"proxy": map[string]interface{}{
				"$ref":        "#/definitions/proxyConfiguration",
				"description": "If defined, outgoing connections will be send through this proxy server",
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
		"required": []string{
			"terminators",
		},
		"additionalProperties": false,
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
