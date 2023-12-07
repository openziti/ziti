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
	"github.com/openziti/storage/boltz"
)

const (
	FieldPostureCheckDomains = "domains"
)

type PostureCheckWindowsDomains struct {
	Domains []string `json:"domains"`
}

func newPostureCheckWindowsDomain() PostureCheckSubType {
	return &PostureCheckWindowsDomains{
		Domains: []string{},
	}
}

func (entity *PostureCheckWindowsDomains) LoadValues(bucket *boltz.TypedBucket) {
	entity.Domains = bucket.GetStringList(FieldPostureCheckDomains)
}

func (entity *PostureCheckWindowsDomains) SetValues(ctx *boltz.PersistContext, bucket *boltz.TypedBucket) {
	bucket.SetStringList(FieldPostureCheckDomains, entity.Domains, ctx.FieldChecker)
}
