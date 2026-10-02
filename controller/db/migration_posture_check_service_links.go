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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/controller/storage/ast"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

// rebuildPostureCheckServiceLinks rebuilds the reference-counted tables between posture checks and
// services from the service policies. It clears the tables on both sides, removes any entry in a
// service's identity tables that is not an identity, then counts one reference per policy for each
// posture check and service pair the policy selects.
func (m *Migrations) rebuildPostureCheckServiceLinks(step *boltz.MigrationStep) {
	tx := step.Ctx.Tx()
	stores := m.stores.internal

	for cursor := stores.postureCheck.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		bucket := stores.postureCheck.GetEntityBucket(tx, cursor.Current())
		deleteChildBuckets(step, bucket, FieldPostureCheckDialServices, FieldPostureCheckBindServices)
	}

	var foreign int
	for cursor := stores.service.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		bucket := stores.service.GetEntityBucket(tx, cursor.Current())
		deleteChildBuckets(step, bucket, FieldEdgeServiceDialPostureChecks, FieldEdgeServiceBindPostureChecks)
		for _, field := range []string{FieldEdgeServiceDialIdentities, FieldEdgeServiceBindIdentities} {
			foreign += removeForeignLinks(step, bucket.GetPath(field), stores.identity)
		}
	}

	var pairs int
	policies := stores.servicePolicy
	for cursor := policies.IterateIds(tx, ast.BoolNodeTrue); cursor.IsValid(); cursor.Next() {
		policyId := cursor.Current()
		counts := stores.postureCheck.bindServicesCollection
		if policies.getPolicyType(tx, policyId) == PolicyTypeDial {
			counts = stores.postureCheck.dialServicesCollection
		}
		serviceIds := policies.serviceCollection.GetLinks(tx, string(policyId))
		for _, postureCheckId := range policies.postureCheckCollection.GetLinks(tx, string(policyId)) {
			for _, serviceId := range serviceIds {
				if _, err := counts.IncrementLinkCount(tx, []byte(postureCheckId), []byte(serviceId)); step.SetError(err) {
					return
				}
				pairs++
			}
		}
	}

	pfxlog.Logger().
		WithField("references", pairs).
		WithField("removedForeignIdentityLinks", foreign).
		Info("rebuilt posture check to service links")
}

// deleteChildBuckets deletes the named child buckets of bucket that exist.
func deleteChildBuckets(step *boltz.MigrationStep, bucket *boltz.TypedBucket, names ...string) {
	if bucket == nil {
		return
	}
	for _, name := range names {
		if bucket.GetBucket(name) != nil {
			step.SetError(bucket.DeleteBucket([]byte(name)))
		}
	}
}

// removeForeignLinks deletes every entry of the link list bucket whose id is not an entity of
// store, and returns how many it deleted. A nil bucket is an empty list.
func removeForeignLinks(step *boltz.MigrationStep, bucket *boltz.TypedBucket, store boltz.Store) int {
	if bucket == nil {
		return 0
	}
	var stale [][]byte
	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		_, id := boltz.GetTypeAndValue(key)
		if !store.IsEntityPresent(bucket.Tx(), string(id)) {
			stale = append(stale, append([]byte(nil), key...))
		}
	}
	for _, key := range stale {
		step.SetError(bucket.Delete(key))
	}
	return len(stale)
}
