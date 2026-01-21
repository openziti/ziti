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
	"sync/atomic"

	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
)

type versionedSnapshotDb struct {
	boltz.Db
	snapshotVersion atomic.Int64
}

func (d *versionedSnapshotDb) GetDefaultSnapshotPath() string {
	// Important: do not open a nested read transaction here.
	// This method may be called while a write transaction is already in progress
	// (e.g. snapshot-before-migrate), and attempting to read the DB can deadlock.
	version := d.snapshotVersion.Load()
	return fmt.Sprintf("%s-edge-v%d", d.Db.GetDefaultSnapshotPath(), version)
}

func setSnapshotVersion(db boltz.Db, version int64) {
	if vdb, ok := db.(*versionedSnapshotDb); ok {
		vdb.snapshotVersion.Store(version)
	}
}

type appEnvKey string

const (
	RootBucket     = "ziti"
	MetadataBucket = "metadata"
	FieldRaftIndex = "raftIndex"
	FieldClusterId = "clusterId"

	AppEnvKey = appEnvKey("AppEnvKey")
)

func Open(path string) (boltz.Db, error) {
	inner, err := boltz.Open(path, RootBucket)
	if err != nil {
		return nil, err
	}

	db := &versionedSnapshotDb{Db: inner}
	// Default to 0 (uninitialized) unless explicitly set by callers.
	db.snapshotVersion.Store(0)

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

func InitClusterId(db boltz.Db, ctx boltz.MutateContext, clusterId string) error {
	return db.Update(ctx, func(ctx boltz.MutateContext) error {
		raftBucket := boltz.GetOrCreatePath(ctx.Tx(), RootBucket, MetadataBucket)
		if raftBucket.HasError() {
			return raftBucket.Err
		}
		currentId := raftBucket.GetStringWithDefault(FieldClusterId, "")
		if currentId != "" {
			return fmt.Errorf("cluster id already initialized to %s", currentId)
		}
		raftBucket.SetString(FieldClusterId, clusterId, nil)
		return raftBucket.Err
	})
}
