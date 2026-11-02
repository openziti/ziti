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

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/change"
	"go.etcd.io/bbolt"
)

// systemConfigTypes are the config types the controller provides, kept current by
// syncSystemConfigTypes. Changing a definition's schema requires bumping its SystemVersion.
var systemConfigTypes = []*ConfigType{
	clientConfigTypeV1,
	serverConfigTypeV1,
	interceptV1ConfigType,
	hostV1ConfigType,
	hostV2ConfigType,
	interfacesConfigTypeV1,
}

// configTypeWrite is a pending change to one system config type. current is nil when the type is to
// be created.
type configTypeWrite struct {
	definition *ConfigType
	current    *ConfigType
}

// syncSystemConfigTypes creates each system config type that is missing, and updates the schema of
// each whose stored SystemVersion is lower than its definition's. Stored types are matched by name, or
// failing that by the definition's id, in which case the type is renamed back; either way it keeps its
// id. A stored schema with a higher SystemVersion is left alone. Nothing is written when every type is
// current. When snapshotBeforeWrite is true and there is something to write, a copy of the datastore
// is saved first.
func (m *Migrations) syncSystemConfigTypes(db boltz.Db, snapshotBeforeWrite bool) error {
	mutateCtx := change.New().
		SetSourceType("system.config-type-sync").
		SetChangeAuthorType(change.AuthorTypeController).
		NewMutateContext()

	return db.Update(mutateCtx, func(ctx boltz.MutateContext) error {
		var writes []*configTypeWrite
		for _, configType := range systemConfigTypes {
			write, err := m.planSystemConfigTypeWrite(ctx.Tx(), configType)
			if err != nil {
				return fmt.Errorf("unable to load config type '%s' (%w)", configType.Name, err)
			}
			if write != nil {
				writes = append(writes, write)
			}
		}

		if len(writes) > 0 && snapshotBeforeWrite {
			if _, _, err := db.SnapshotInTx(ctx.Tx(), db.GetDefaultSnapshotPath()); err != nil {
				return fmt.Errorf("failed to create bolt db snapshot before syncing config types (%w)", err)
			}
		}

		for _, write := range writes {
			if err := m.applySystemConfigTypeWrite(ctx, write); err != nil {
				return fmt.Errorf("unable to sync config type '%s' (%w)", write.definition.Name, err)
			}
		}
		return nil
	})
}

// planSystemConfigTypeWrite returns the write that brings the stored type up to configType, or nil when
// none is needed.
func (m *Migrations) planSystemConfigTypeWrite(tx *bbolt.Tx, configType *ConfigType) (*configTypeWrite, error) {
	current, err := m.stores.ConfigType.LoadOneByName(tx, configType.Name)
	if err != nil {
		return nil, err
	}

	if current == nil {
		if !m.stores.ConfigType.IsEntityPresent(tx, configType.Id) {
			return &configTypeWrite{definition: configType}, nil
		}
		if current, err = m.stores.ConfigType.LoadById(tx, configType.Id); err != nil {
			return nil, err
		}
	}

	if current.Name == configType.Name && current.SystemVersion >= configType.SystemVersion {
		return nil, nil
	}

	return &configTypeWrite{definition: configType, current: current}, nil
}

func (m *Migrations) applySystemConfigTypeWrite(ctx boltz.MutateContext, write *configTypeWrite) error {
	log := pfxlog.Logger().
		WithField("configType", write.definition.Name).
		WithField("systemVersion", write.definition.SystemVersion)

	if write.current == nil {
		log.Info("creating system config type")
		// create from a copy, so the package-level definition isn't stamped with timestamps
		toCreate := *write.definition
		return m.stores.ConfigType.Create(ctx, &toCreate)
	}

	log = log.WithField("id", write.current.Id)
	fields := boltz.MapFieldChecker{}

	if write.current.Name != write.definition.Name {
		log.WithField("storedName", write.current.Name).Warn("renaming system config type back to its system name")
		write.current.Name = write.definition.Name
		fields[FieldName] = struct{}{}
	}

	if write.current.SystemVersion < write.definition.SystemVersion {
		log.WithField("storedVersion", write.current.SystemVersion).Info("updating system config type")
		write.current.Schema = write.definition.Schema
		write.current.SystemVersion = write.definition.SystemVersion
		fields[FieldConfigTypeSchema] = struct{}{}
		fields[FieldConfigTypeSystemVersion] = struct{}{}
	}

	return m.stores.ConfigType.Update(ctx, write.current, fields)
}
