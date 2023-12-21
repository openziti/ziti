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
	"github.com/openziti/storage/boltz"
)

const (
	FieldPostureCheckOsType     = "osType"
	FieldPostureCheckOsVersions = "osVersions"
)

type PostureCheckOperatingSystem struct {
	OperatingSystems []OperatingSystem `json:"operatingSystems"`
}

type OperatingSystem struct {
	OsType     string   `json:"osType"`
	OsVersions []string `json:"osVersions"`
}

func newPostureCheckOperatingSystem() PostureCheckSubType {
	return &PostureCheckOperatingSystem{
		OperatingSystems: []OperatingSystem{},
	}
}

func (entity *PostureCheckOperatingSystem) LoadValues(bucket *boltz.TypedBucket) {

	cursor := bucket.Cursor()

	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		osBucket := bucket.GetBucket(string(key))
		newOsMatch := OperatingSystem{
			OsType: osBucket.GetStringOrError(FieldPostureCheckOsType),
		}

		newOsMatch.OsVersions = append(newOsMatch.OsVersions, osBucket.GetStringList(FieldPostureCheckOsVersions)...)
		entity.OperatingSystems = append(entity.OperatingSystems, newOsMatch)
	}

}

func (entity *PostureCheckOperatingSystem) SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket) {
	if bucket.ProceedWithSet(FieldPostureCheckOsType, ctx.FieldChecker) && bucket.ProceedWithSet(FieldPostureCheckOsVersions, ctx.FieldChecker) {

		osMap := map[string]OperatingSystem{}

		for _, os := range entity.OperatingSystems {
			osMap[os.OsType] = os
		}

		cursor := bucket.Cursor()
		for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
			if _, found := osMap[string(key)]; !found {
				err := bucket.DeleteBucket(key)
				if err != nil {
					pfxlog.Logger().Errorf(err.Error())
				}
			}
		}

		seenOs := map[string]struct{}{}
		for _, os := range entity.OperatingSystems {
			seenOs[os.OsType] = struct{}{}
			existing := bucket.GetOrCreateBucket(os.OsType)
			existing.SetString(FieldPostureCheckOsType, os.OsType, ctx.FieldChecker)
			existing.SetStringList(FieldPostureCheckOsVersions, os.OsVersions, ctx.FieldChecker)
		}
	}
}
