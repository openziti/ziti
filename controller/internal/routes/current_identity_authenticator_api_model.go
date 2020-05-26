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

package routes

import (
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/foundation/util/stringz"
)

type AuthenticatorSelfUpdateApi struct {
	Username        *string                `json:"username, omitempty"`
	NewPassword     *string                `json:"newPassword, omitempty"`
	CurrentPassword *string                `json:"currentPassword, omitempty"`
	Tags            map[string]interface{} `json:"tags"`
}

func (i *AuthenticatorSelfUpdateApi) ToModel(id, identityId string) *model.AuthenticatorSelf {
	ret := &model.AuthenticatorSelf{
		CurrentPassword: stringz.OrEmpty(i.CurrentPassword),
		NewPassword:     stringz.OrEmpty(i.NewPassword),
		IdentityId:      identityId,
		Username:        stringz.OrEmpty(i.Username),
	}

	ret.Id = id

	return ret
}
