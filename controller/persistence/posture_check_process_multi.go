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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/storage/boltz"
)

const (
	FieldPostureCheckProcessMultiOsType             = "osType"
	FieldPostureCheckProcessMultiPath               = "path"
	FieldPostureCheckProcessMultiHashes             = "hashes"
	FieldPostureCheckProcessMultiSignerFingerprints = "signerFingerprints"
	FieldPostureCheckProcessMultiProcesses          = "processes"
)

type PostureCheckProcessMulti struct {
	Semantic  string
	Processes []*ProcessMulti
}

type ProcessMulti struct {
	OsType             string
	Path               string
	Hashes             []string
	SignerFingerprints []string
}

func newPostureCheckProcessMulti() PostureCheckSubType {
	return &PostureCheckProcessMulti{
		Semantic:  "",
		Processes: nil,
	}
}

func (entity *PostureCheckProcessMulti) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.Semantic = bucket.GetStringOrError(FieldSemantic)

	processesBucket := bucket.GetBucket(FieldPostureCheckProcessMultiProcesses)

	processCursor := processesBucket.Cursor()

	for key, _ := processCursor.First(); key != nil; key, _ = processCursor.Next() {
		procBucket := processesBucket.GetBucket(string(key))
		proc := &ProcessMulti{}

		proc.Path = procBucket.GetStringOrError(FieldPostureCheckProcessMultiPath)
		proc.OsType = procBucket.GetStringOrError(FieldPostureCheckProcessMultiOsType)
		proc.SignerFingerprints = procBucket.GetStringList(FieldPostureCheckProcessMultiSignerFingerprints)
		proc.Hashes = procBucket.GetStringList(FieldPostureCheckProcessMultiHashes)

		entity.Processes = append(entity.Processes, proc)
	}
}

func (entity *PostureCheckProcessMulti) SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket) {
	bucket.SetString(FieldSemantic, entity.Semantic, ctx.FieldChecker)

	processesBucket := bucket.GetOrCreateBucket(FieldPostureCheckProcessMultiProcesses)

	seenKeys := map[string]struct{}{}
	for _, proc := range entity.Processes {
		key := proc.OsType + "-" + proc.Path
		seenKeys[key] = struct{}{}

		procBucket := processesBucket.GetOrCreateBucket(key)

		procBucket.SetString(FieldPostureCheckProcessMultiPath, proc.Path, ctx.FieldChecker)
		procBucket.SetString(FieldPostureCheckProcessMultiOsType, proc.OsType, ctx.FieldChecker)
		procBucket.SetStringList(FieldPostureCheckProcessMultiHashes, proc.Hashes, ctx.FieldChecker)
		procBucket.SetStringList(FieldPostureCheckProcessMultiSignerFingerprints, proc.SignerFingerprints, ctx.FieldChecker)
	}

	processCursor := processesBucket.Cursor()

	var removeKeys [][]byte
	for key, _ := processCursor.First(); key != nil; key, _ = processCursor.Next() {
		if _, ok := seenKeys[string(key)]; !ok {
			removeKeys = append(removeKeys, key)
		}
	}

	for _, key := range removeKeys {
		if err := processesBucket.DeleteBucket(key); err != nil {
			pfxlog.Logger().Debugf("error deleting multi process key %s: %v", string(key), err)
		}

	}
}
