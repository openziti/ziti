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
)

const (
	FieldPostureCheckOsType     = "osType"
	FieldPostureCheckOsVersions = "osVersions"
)

type PostureCheckOperatingSystem struct {
	OperatingSystems []OperatingSystem
}

type OperatingSystem struct {
	OsType     string
	OsVersions []string
}

func newPostureCheckOperatingSystem() PostureCheckSubType {
	return &PostureCheckOperatingSystem{
		OperatingSystems: []OperatingSystem{},
	}
}

func (entity *PostureCheckOperatingSystem) LoadValues(_ boltz.CrudStore, bucket *boltz.TypedBucket) {

	cursor := bucket.Cursor()

	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		osBucket := bucket.GetBucket(string(key))
		newOsMatch := OperatingSystem{
			OsType: osBucket.GetStringOrError(FieldPostureCheckOsType),
		}

		for _, osVersion := range osBucket.GetStringList(FieldPostureCheckOsVersions) {
			newOsMatch.OsVersions = append(newOsMatch.OsVersions, osVersion)
		}
		entity.OperatingSystems = append(entity.OperatingSystems, newOsMatch)
	}

}

func (entity *PostureCheckOperatingSystem) SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket) {
	osMap := map[string]OperatingSystem{}

	for _, os := range entity.OperatingSystems {
		osMap[os.OsType] = os
	}

	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		if _, found := osMap[string(key)]; !found {
			_ = bucket.Delete(key)
		}
	}

	for _, os := range entity.OperatingSystems {
		existing := bucket.GetOrCreateBucket(os.OsType)
		existing.SetString(FieldPostureCheckOsType, os.OsType, ctx.FieldChecker)
		existing.SetStringList(FieldPostureCheckOsVersions, os.OsVersions, ctx.FieldChecker)
	}
}
