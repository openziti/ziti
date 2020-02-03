/*
	Copyright 2020 Netfoundry, Inc.

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
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-foundation/storage/ast"
	"github.com/netfoundry/ziti-foundation/storage/boltz"
	"math"
)

const (
	FieldServiceDnsHostname = "dnsHostname"
	FieldServiceDnsPort     = "dnsPort"
)

var clientConfigV1TypeId = "f2dd2df0-9c04-4b84-a91e-71437ac229f1"
var serverConfigV1TypeId = "cea49285-6c07-42cf-9f52-09a9b115c783"

func createInitialTunnelerConfigTypes(mtx *MigrationContext) error {
	clientConfigTypeV1 := &ConfigType{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: clientConfigV1TypeId},
		Name:               "ziti-tunneler-client.v1",
		Schema: map[string]interface{}{
			"$id":                  "http://ziti-edge.netfoundry.io/schemas/ziti-tunneler-client.v1.config.json",
			"type":                 "object",
			"additionalProperties": false,
			"required": []interface{}{
				"hostname",
				"port",
			},
			"properties": map[string]interface{}{
				// TODO: Add protocol list here, so we know which protocols to listen on
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
	if err := mtx.Stores.ConfigType.Create(mtx.Ctx, clientConfigTypeV1); err != nil {
		return err
	}

	serverConfigTypeV1 := &ConfigType{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: serverConfigV1TypeId},
		Name:               "ziti-tunneler-server.v1",
		Schema: map[string]interface{}{
			"$id":                  "http://ziti-edge.netfoundry.io/schemas/ziti-tunneler-server.v1.config.json",
			"type":                 "object",
			"additionalProperties": false,
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
			},
		},
	}
	return mtx.Stores.ConfigType.Create(mtx.Ctx, serverConfigTypeV1)
}

func createInitialTunnelerConfigs(mtx *MigrationContext) error {
	ids, _, err := mtx.Stores.EdgeService.QueryIds(mtx.Ctx.Tx(), "true")
	if err != nil {
		return err
	}

	hostnameSymbol := mtx.Stores.EdgeService.NewEntitySymbol(FieldServiceDnsHostname, ast.NodeTypeString)
	portSymbol := mtx.Stores.EdgeService.NewEntitySymbol(FieldServiceDnsHostname, ast.NodeTypeInt64)

	for _, id := range ids {
		service, err := mtx.Stores.EdgeService.LoadOneById(mtx.Ctx.Tx(), id)
		if err != nil {
			return err
		}

		fieldType, val := hostnameSymbol.Eval(mtx.Ctx.Tx(), []byte(id))
		hostname := boltz.FieldToString(fieldType, val)

		fieldType, val = portSymbol.Eval(mtx.Ctx.Tx(), []byte(id))
		port := boltz.FieldToInt64(fieldType, val)
		finalPort := 0
		if port != nil {
			finalPort = int(*port)
		}
		if err := createServiceConfigs(mtx, service, hostname, finalPort); err != nil {
			return err
		}
	}
	return nil
}

type migrationConfigUpdateFieldChecker struct{}

func (m migrationConfigUpdateFieldChecker) IsUpdated(field string) bool {
	return field == EntityTypeConfigs
}

func createServiceConfigs(mtx *MigrationContext, service *EdgeService, hostname *string, port int) error {
	if hostname == nil {
		return nil
	}
	clientConfigData := map[string]interface{}{
		"hostname": *hostname,
		"port":     port,
	}
	config := &Config{
		BaseEdgeEntityImpl: BaseEdgeEntityImpl{Id: uuid.New().String()},
		Name:               service.Name + "-tunneler-client-config",
		Type:               clientConfigV1TypeId,
		Data:               clientConfigData,
	}
	if err := mtx.Stores.Config.Create(mtx.Ctx, config); err != nil {
		return err
	}
	service.Configs = append(service.Configs, config.Id)
	return mtx.Stores.EdgeService.Update(mtx.Ctx, service, &migrationConfigUpdateFieldChecker{})
}
