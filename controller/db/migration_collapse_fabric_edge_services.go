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
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

func encodeBool(value bool) []byte {
	buf := make([]byte, 2)
	buf[0] = byte(boltz.TypeBool)
	if value {
		buf[1] = 1
	}
	return buf
}

func (m *Migrations) collapseEdgeServices(step *boltz.MigrationStep) {
	log := pfxlog.Logger()
	tx := step.Ctx.Tx()

	rootBucket := tx.Bucket([]byte(RootBucket))
	if rootBucket == nil {
		return
	}

	servicesBucket := rootBucket.Bucket([]byte(EntityTypeServices))
	if servicesBucket == nil {
		return
	}

	var edgeCount, fabricCount int

	// Iterate all service entity IDs
	cursor := servicesBucket.Cursor()
	for key, val := cursor.First(); key != nil; key, val = cursor.Next() {
		// Skip non-bucket entries (val != nil means it's a key-value pair, not a sub-bucket)
		if val != nil {
			continue
		}

		entityBucket := servicesBucket.Bucket(key)
		if entityBucket == nil {
			continue
		}

		edgeBucket := entityBucket.Bucket([]byte(EdgeBucket))
		if edgeBucket == nil {
			// Fabric-only service: set isFabricOnly = true. Also write encryptionRequired = true,
			// matching both the fabric create path and FillEntity's absent-default. Encryption is
			// the secure default: the field is unreachable for fabric-only services today, but when
			// fabric and edge services fully merge, pre-existing fabric services should require
			// encryption unless explicitly opted out.
			step.SetError(entityBucket.Put([]byte(FieldServiceIsFabricOnly), encodeBool(true)))
			step.SetError(entityBucket.Put([]byte(FieldServiceEncryptionRequired), encodeBool(true)))
			fabricCount++
			continue
		}

		// Edge service: move data from edge sub-bucket to main entity bucket
		step.SetError(migrateEdgeServiceBucket(key, entityBucket, edgeBucket))
		if step.HasError() {
			return
		}

		// Set isFabricOnly = false
		step.SetError(entityBucket.Put([]byte(FieldServiceIsFabricOnly), encodeBool(false)))

		// Delete the edge sub-bucket
		step.SetError(entityBucket.DeleteBucket([]byte(EdgeBucket)))
		if step.HasError() {
			return
		}

		edgeCount++
	}

	log.Infof("service collapse migration: migrated %d edge services and %d fabric-only services", edgeCount, fabricCount)

	if step.HasError() {
		return
	}

	// The indexes need no migration: the old edge service child store inherited its parent's
	// entity type, so its role-attributes index already lives at the unified service store
	// path (ziti/indexes/services/roleAttributes), where the unified store expects it. There
	// was never an index bucket at an edge child path (verified against a real pre-collapse
	// db by the migration gate in zititest/migration-test).
	//
	// Verify (and as a safety net, repair) the service store's indexes and denormalized link
	// collections. The check is expected to find nothing. Anything it fixes indicates a bug
	// in the entity move, the pre-migration index, or the checker itself, so fixes are
	// logged at warning level and reported to ServiceCollapseEventListener for migration
	// verification tooling to flag.
	step.SetError(m.stores.Service.CheckIntegrity(step.Ctx, true, func(err error, fixed bool) {
		reportServiceCollapseEvent(fmt.Sprintf("integrity check: fixed=%v: %v", fixed, err))
		if fixed {
			log.WithError(err).Warn("unexpectedly fixed service store integrity issue during edge service collapse")
		} else {
			log.WithError(err).Error("unfixable service store integrity issue during edge service collapse")
		}
	}))
}

// ServiceCollapseEventListener if set, receives anomaly events from the edge service collapse
// migration: integrity check reports and replaced pre-existing service-level fields. The
// migration is expected to report nothing; migration verification tooling and tests use this
// hook to assert that.
var ServiceCollapseEventListener func(event string)

// reportServiceCollapseEvent delivers an anomaly event to ServiceCollapseEventListener, if set.
func reportServiceCollapseEvent(event string) {
	if listener := ServiceCollapseEventListener; listener != nil {
		listener(event)
	}
}

// migrateEdgeServiceBucket moves all data from the edge sub-bucket into the main entity bucket:
// scalar key-value pairs are copied (skipping the name field, which is duplicated in the parent)
// and nested sub-buckets (FK set buckets like configs, servicePolicies, bindIdentities,
// dialIdentities, etc.) are moved wholesale via bbolt's MoveBucket. Keys are collected first and
// applied after iteration, since mutating a bucket while iterating it is undefined behavior.
//
// Edge-owned fields should not exist at the service level pre-collapse, so a pre-existing
// destination indicates something unusual. Since such data was unreachable through the
// pre-collapse stores, it is replaced with the authoritative edge data rather than failing the
// migration, with a warning logged and reported to ServiceCollapseEventListener.
func migrateEdgeServiceBucket(serviceId []byte, entityBucket, edgeBucket *bbolt.Bucket) error {
	type scalar struct {
		key   []byte
		value []byte
	}
	var scalars []scalar
	var subBuckets [][]byte

	err := edgeBucket.ForEach(func(k, v []byte) error {
		if v != nil {
			// Skip the "name" field since it already exists in the parent bucket.
			if string(k) == FieldName {
				return nil
			}
			scalars = append(scalars, scalar{key: append([]byte(nil), k...), value: append([]byte(nil), v...)})
			return nil
		}
		subBuckets = append(subBuckets, append([]byte(nil), k...))
		return nil
	})
	if err != nil {
		return err
	}

	for _, entry := range scalars {
		if entityBucket.Get(entry.key) != nil {
			warnPreExistingDestination(serviceId, entry.key, "field")
		}
		if err := entityBucket.Put(entry.key, entry.value); err != nil {
			return err
		}
	}

	for _, key := range subBuckets {
		// remove any pre-existing destination bucket; MoveBucket requires the key to be free
		if entityBucket.Bucket(key) != nil {
			warnPreExistingDestination(serviceId, key, "bucket")
			if err := entityBucket.DeleteBucket(key); err != nil {
				return err
			}
		}
		if err := edgeBucket.MoveBucket(key, entityBucket); err != nil {
			return err
		}
	}
	return nil
}

// warnPreExistingDestination logs and reports the replacement of an unexpected pre-existing
// service-level field or bucket during the edge service collapse. Such data was unreachable
// through the pre-collapse stores, but its presence indicates something unusual happened.
func warnPreExistingDestination(serviceId, key []byte, kind string) {
	event := fmt.Sprintf("service collapse migration: replaced unexpected pre-existing %v %q on service %v with edge service data",
		kind, string(key), string(serviceId))
	pfxlog.Logger().Warn(event)
	reportServiceCollapseEvent(event)
}
