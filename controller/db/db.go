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
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"go.etcd.io/bbolt"
)

type appEnvKey string

const (
	RootBucket     = "ziti"
	MetadataBucket = "metadata"
	FieldRaftIndex = "raftIndex"
	FieldClusterId = "clusterId"

	AppEnvKey = appEnvKey("AppEnvKey")
)

func Open(path string) (boltz.Db, error) {
	db, err := boltz.Open(path, RootBucket)
	if err != nil {
		return nil, err
	}

	err = db.Update(nil, func(ctx boltz.MutateContext) error {
		_, err := ctx.Tx().CreateBucketIfNotExists([]byte(RootBucket))
		return err
	})

	if err != nil {
		return nil, err
	}

	return db, nil
}

func LoadCurrentRaftIndex(tx *bbolt.Tx) uint64 {
	if raftBucket := boltz.Path(tx, RootBucket, MetadataBucket); raftBucket != nil {
		if val := raftBucket.GetInt64(FieldRaftIndex); val != nil {
			return uint64(*val)
		}
	}
	return 0
}

func LoadClusterId(db boltz.Db) (string, error) {
	var result string
	err := db.View(func(tx *bbolt.Tx) error {
		raftBucket := boltz.Path(tx, RootBucket, MetadataBucket)
		if raftBucket == nil {
			return nil
		}
		result = raftBucket.GetStringWithDefault(FieldClusterId, "")
		return nil
	})
	return result, err
}

// InitClusterId sets the cluster id if unset and returns the effective id (an existing id wins). It
// is set-once: a differing id is kept with a warning rather than an error, so redundant or racing
// writes (e.g. backfill across leadership changes) cannot fail.
func InitClusterId(db boltz.Db, ctx boltz.MutateContext, clusterId string) (string, error) {
	effective := clusterId
	err := db.Update(ctx, func(ctx boltz.MutateContext) error {
		raftBucket := boltz.GetOrCreatePath(ctx.Tx(), RootBucket, MetadataBucket)
		if raftBucket.HasError() {
			return raftBucket.Err
		}
		currentId := raftBucket.GetStringWithDefault(FieldClusterId, "")
		if currentId != "" {
			effective = currentId
			if currentId != clusterId {
				pfxlog.Logger().
					WithField("existingClusterId", currentId).
					WithField("ignoredClusterId", clusterId).
					Warn("cluster id already set; keeping existing value and ignoring the new one")
			}
			return nil
		}
		raftBucket.SetString(FieldClusterId, clusterId, nil)
		return raftBucket.Err
	})
	return effective, err
}
