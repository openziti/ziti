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

package tests

import (
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
)

// ManagerHelpers provides direct access to controller manager operations, bypassing the REST API.
// Accessible via TestContext.Managers after StartServer() has been called.
type ManagerHelpers struct {
	ctx *TestContext
}

// CreateAuthPolicy creates an auth policy via the manager. The policy's Id field is populated on success.
func (m *ManagerHelpers) CreateAuthPolicy(policy *model.AuthPolicy) error {
	return m.ctx.EdgeController.AppEnv.Managers.AuthPolicy.Create(policy, nil)
}

// CreateIdentity creates an identity via the manager. The identity's Id field is populated on success.
func (m *ManagerHelpers) CreateIdentity(identity *model.Identity) error {
	return m.ctx.EdgeController.AppEnv.Managers.Identity.Create(identity, nil)
}

// CreateUpdbAuthenticator creates a username/password authenticator for the given identity.
func (m *ManagerHelpers) CreateUpdbAuthenticator(identityId, username, password string) error {
	authenticator := &model.Authenticator{
		BaseEntity: models.BaseEntity{},
		Method:     db.MethodAuthenticatorUpdb,
		IdentityId: identityId,
		SubType: &model.AuthenticatorUpdb{
			Username: username,
			Password: password,
		},
	}
	return m.ctx.EdgeController.AppEnv.Managers.Authenticator.Create(authenticator, nil)
}

// NewIdentityWithUpdb creates a non-admin identity with random credentials attached to the given
// auth policy, and registers a UPDB authenticator for it. Returns the identity, credentials, and
// any error.
func (m *ManagerHelpers) NewIdentityWithUpdb(authPolicyId string) (*model.Identity, *updbAuthenticator, error) {
	creds := &updbAuthenticator{
		Username: eid.New(),
		Password: eid.New(),
	}

	identity := &model.Identity{
		Name:           creds.Username,
		IdentityTypeId: db.DefaultIdentityType,
		IsAdmin:        false,
		AuthPolicyId:   authPolicyId,
	}

	if err := m.CreateIdentity(identity); err != nil {
		return nil, nil, err
	}

	if err := m.CreateUpdbAuthenticator(identity.Id, creds.Username, creds.Password); err != nil {
		return nil, nil, err
	}

	return identity, creds, nil
}
