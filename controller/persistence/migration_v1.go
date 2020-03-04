/*
	Copyright 2020 NetFoundry, Inc.

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

import "github.com/netfoundry/ziti-foundation/storage/boltz"

func (m *Migrations) initialize(step *boltz.MigrationStep) int {
	versionBucket := boltz.GetOrCreatePath(step.Ctx.Tx(), boltz.RootBucket)
	if step.SetError(versionBucket.GetError()) {
		return 0
	}
	version := versionBucket.GetInt64(FieldVersion)
	if version != nil && *version > 0 {
		return int(*version)
	}

	m.createGeoRegionsV1(step)
	m.createIdentityTypesV1(step)

	if m.dbStores != nil {
		m.upgradeToV1FromPG(step)
	}
	return 1
}

var geoRegionsV1 = map[string]string{
	"a0e2c29f-9922-4435-a8a7-5dbf7bd92377": "Canada Central",
	"ac469973-105c-4de1-9f31-fffc077487fb": "US West",
	"2360479d-cc08-4224-bd56-43baa672af30": "Japan",
	"edd0680f-3ab4-49e6-9db5-c68258ba480d": "Australia",
	"521a3db9-8140-4854-a782-61cb5d3fe043": "South America",
	"8342acad-4e49-4098-84de-829feb55d350": "Korea",
	"b5562b15-ffeb-4910-bf14-a03067f9ca2e": "US Midwest",
	"6efe28a5-744e-464d-b147-4072efb769f0": "Canada West",
	"10c6f648-92b7-49e2-be96-f62357ea572f": "Europe West",
	"72946251-1fc7-4b3b-8568-c59d4723e704": "Africa",
	"70438d63-97b3-48b2-aeb5-b066a9526456": "Europe East",
	"e339d699-7f51-4e9c-a2ca-81720e07f196": "US South",
	"00586703-6748-4c78-890d-efa216f21ef3": "Canada East",
	"63f7200b-7794-4a68-92aa-a36ed338ecba": "Asia",
	"f91ecca3-6f82-4c7b-8191-ea0036ce7b5a": "US East",
	"41929e78-6674-4708-89d2-9b934ea96822": "Middle East",
	"5d0042bb-6fd5-4959-90a7-6bca70e23f76": "US Central",
}

func (m *Migrations) createGeoRegionsV1(step *boltz.MigrationStep) {
	for id, name := range geoRegionsV1 {
		step.SetError(m.stores.GeoRegion.Create(step.Ctx, &GeoRegion{
			BaseExtEntity: *boltz.NewExtEntity(id, nil),
			Name:          name,
		}))
	}
}

var IdentityTypesV1 = map[string]string{
	"577104f2-1e3a-4947-a927-7383baefbc9a": "User",
	"5b53fb49-51b1-4a87-a4e4-edda9716a970": "Device",
	"c4d66f9d-fe18-4143-85d3-74329c54282b": "Service",
}

func (m *Migrations) createIdentityTypesV1(step *boltz.MigrationStep) {
	for id, name := range IdentityTypesV1 {
		step.SetError(m.stores.IdentityType.Create(step.Ctx, &IdentityType{
			BaseExtEntity: *boltz.NewExtEntity(id, nil),
			Name:          name,
		}))
	}
}
