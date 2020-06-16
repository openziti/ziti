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
	"github.com/openziti/edge/eid"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
)

const (
	FieldServiceDnsHostname = "dnsHostname"
	FieldServiceDnsPort     = "dnsPort"
)

func (m *Migrations) createInitialTunnelerConfigs(step *boltz.MigrationStep) {
	ids, _, err := m.stores.EdgeService.QueryIds(step.Ctx.Tx(), "true")
	step.SetError(err)

	hostnameSymbol := m.stores.EdgeService.NewEntitySymbol(FieldServiceDnsHostname, ast.NodeTypeString)
	portSymbol := m.stores.EdgeService.NewEntitySymbol(FieldServiceDnsPort, ast.NodeTypeInt64)

	for _, id := range ids {
		service, err := m.stores.EdgeService.LoadOneById(step.Ctx.Tx(), id)
		if step.SetError(err) {
			return
		}

		fieldType, val := hostnameSymbol.Eval(step.Ctx.Tx(), []byte(id))
		hostname := boltz.FieldToString(fieldType, val)

		fieldType, val = portSymbol.Eval(step.Ctx.Tx(), []byte(id))
		port := boltz.FieldToInt64(fieldType, val)
		finalPort := 0
		if port != nil {
			finalPort = int(*port)
		}
		step.SetError(m.createServiceConfigs(step, service, hostname, finalPort))
	}
}

type migrationConfigUpdateFieldChecker struct{}

func (m migrationConfigUpdateFieldChecker) IsUpdated(field string) bool {
	return field == EntityTypeConfigs
}

func (m *Migrations) createServiceConfigs(step *boltz.MigrationStep, service *EdgeService, hostname *string, port int) error {
	if hostname == nil {
		return nil
	}
	clientConfigData := map[string]interface{}{
		"hostname": *hostname,
		"port":     port,
	}
	config := &Config{
		BaseExtEntity: boltz.BaseExtEntity{Id: eid.New()},
		Name:          service.Name + "-tunneler-client-config",
		Type:          clientConfigV1TypeId,
		Data:          clientConfigData,
	}
	if err := m.stores.Config.Create(step.Ctx, config); err != nil {
		return err
	}
	service.Configs = append(service.Configs, config.Id)
	return m.stores.EdgeService.Update(step.Ctx, service, &migrationConfigUpdateFieldChecker{})
}
