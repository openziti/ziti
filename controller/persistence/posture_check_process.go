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
	"github.com/openziti/foundation/storage/boltz"
	"strings"
)

const (
	FieldPostureCheckProcessOs          = "os"
	FieldPostureCheckProcessPath        = "path"
	FieldPostureCheckProcessHashes      = "hashes"
	FieldPostureCheckProcessFingerprint = "fingerprint"
)

type PostureCheckProcess struct {
	OperatingSystem string
	Path            string
	Hashes          []string
	Fingerprint     string
}

func newPostureCheckProcess() PostureCheckSubType {
	return &PostureCheckProcess{
		OperatingSystem: "",
		Path:            "",
		Hashes:          []string{},
		Fingerprint:     "",
	}
}

func (entity *PostureCheckProcess) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {
	entity.OperatingSystem = bucket.GetStringOrError(FieldPostureCheckProcessOs)
	entity.Path = bucket.GetStringOrError(FieldPostureCheckProcessPath)
	entity.Hashes = bucket.GetStringList(FieldPostureCheckProcessHashes)
	entity.Fingerprint = bucket.GetStringOrError(FieldPostureCheckProcessFingerprint)
}

func (entity *PostureCheckProcess) SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket) {

	entity.Fingerprint = strings.ToLower(entity.Fingerprint)

	for i, hash := range entity.Hashes {
		entity.Hashes[i] = strings.ToLower(hash)
	}

	bucket.SetString(FieldPostureCheckProcessOs, entity.OperatingSystem, ctx.FieldChecker)
	bucket.SetString(FieldPostureCheckProcessPath, entity.Path, ctx.FieldChecker)
	bucket.SetStringList(FieldPostureCheckProcessHashes, entity.Hashes, ctx.FieldChecker)
	bucket.SetString(FieldPostureCheckProcessFingerprint, entity.Fingerprint, ctx.FieldChecker)
}
