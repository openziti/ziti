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

func (m *Migrations) addGlobalControllerConfig(step *boltz.MigrationStep) {

	redirects := []string{
		"openziti://auth/callback",
		"https://127.0.0.1:*/auth/callback",
		"http://127.0.0.1:*/auth/callback",
		"https://localhost:*/auth/callback",
		"http://localhost:*/auth/callback",
		"http://[::1]:*/auth/callback",
		"https://[::1]:*/auth/callback",
	}

	postLogouts := []string{
		"openziti://auth/logout",
		"https://127.0.0.1:*/auth/logout",
		"http://127.0.0.1:*/auth/logout",
		"https://localhost:*/auth/logout",
		"http://localhost:*/auth/logout",
		"http://[::1]:*/auth/logout",
		"https://[::1]:*/auth/logout",
	}

	settings := &ControllerSetting{
		BaseExtEntity: boltz.BaseExtEntity{
			Id:       ControllerSettingGlobalId,
			IsSystem: true,
		},
		Oidc: &OidcSettingDef{
			RedirectUris:   redirects,
			PostLogoutUris: postLogouts,
		},
		IsMigration: true,
	}

	step.SetError(m.stores.ControllerSetting.CreateGlobalDefault(step.Ctx, settings))
}
