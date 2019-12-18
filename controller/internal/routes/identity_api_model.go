/*
	Copyright 2019 Netfoundry, Inc.

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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-edge/migration"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const (
	EntityNameIdentity = "identities"
)

type PermissionsApi []string

type IdentityTypeApi struct {
	Id   *string `json:"id"`
	Name *string `json:"name"`
}

type IdentityApiList struct {
	*env.BaseApi
	Name           *string                `json:"name"`
	Type           *EntityApiRef          `json:"type"`
	IsDefaultAdmin *bool                  `json:"isDefaultAdmin"`
	IsAdmin        *bool                  `json:"isAdmin"`
	Authenticators map[string]interface{} `json:"authenticators"`
	Enrollments    map[string]interface{} `json:"enrollment"` //per original API "enrollment" is correct
	RoleAttributes []string               `json:"roleAttributes"`
}

type IdentityApiUpdate struct {
	Tags           *migration.PropertyMap `json:"tags"`
	Name           *string                `json:"name"`
	Type           *string                `json:"type"`
	IsAdmin        *bool                  `json:"isAdmin"`
	RoleAttributes []string               `json:"roleAttributes"`
}

func (i IdentityApiUpdate) ToModel(id string) *model.Identity {
	result := &model.Identity{}
	result.Id = id
	result.Name = stringz.OrEmpty(i.Name)
	result.IsAdmin = boolOrFalse(i.IsAdmin)
	result.IsDefaultAdmin = false
	result.IdentityTypeId = stringz.OrEmpty(i.Type)
	result.RoleAttributes = i.RoleAttributes
	if i.Tags != nil {
		result.Tags = *i.Tags
	}
	return result
}

type IdentityApiCreate struct {
	Tags           map[string]interface{} `json:"tags"`
	Enrollment     map[string]interface{} `json:"enrollment"`
	Name           *string                `json:"name"`
	Type           *string                `json:"type"`
	IsAdmin        *bool                  `json:"isAdmin"`
	RoleAttributes []string               `json:"roleAttributes"`
}

func NewIdentityApiCreate() *IdentityApiCreate {
	f := false
	return &IdentityApiCreate{
		IsAdmin: &f,
	}
}

func (i *IdentityApiCreate) ToModel() (*model.Identity, []*model.Enrollment) {
	identity := &model.Identity{}
	identity.Name = stringz.OrEmpty(i.Name)
	identity.IdentityTypeId = stringz.OrEmpty(i.Type)
	identity.IsDefaultAdmin = false
	identity.IsAdmin = boolOrFalse(i.IsAdmin)
	identity.RoleAttributes = i.RoleAttributes
	identity.Tags = i.Tags

	var enrollments []*model.Enrollment
	if caId, ok := i.Enrollment[persistence.MethodEnrollOttCa]; ok {
		caId := caId.(string)
		enrollment := &model.Enrollment{
			BaseModelEntityImpl: model.BaseModelEntityImpl{},
			Method:              persistence.MethodEnrollOttCa,
			CaId:                &caId,
		}
		enrollments = append(enrollments, enrollment)
	}

	if _, ok := i.Enrollment[persistence.MethodEnrollOtt]; ok {
		enrollments = append(enrollments, &model.Enrollment{
			BaseModelEntityImpl: model.BaseModelEntityImpl{},
			Method:              persistence.MethodEnrollOtt,
		})
	}

	if val, ok := i.Enrollment[persistence.MethodEnrollUpdb]; ok {
		username := val.(string)
		enrollments = append(enrollments, &model.Enrollment{
			BaseModelEntityImpl: model.BaseModelEntityImpl{},
			Method:              persistence.MethodEnrollUpdb,
			Username:            &username,
		})
	}

	return identity, enrollments
}

func boolOrFalse(b *bool) bool {
	if b == nil {
		return false
	}

	return *b
}

type AuthenticatorUpdbApi struct {
	Username string `json:"username"`
}

type AuthenticatorCertApi struct {
	Fingerprint string `json:"fingerprint"`
}

func (e *IdentityApiList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (IdentityApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameIdentity, id))
}

func (e *IdentityApiList) PopulateLinks() {
	if e.Links == nil {
		e.Links = &response.Links{
			EntityNameSelf: e.GetSelfLink(),
		}
	}
}

func (e *IdentityApiList) ToEntityApiRef() *EntityApiRef {
	e.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameIdentity,
		Name:   e.Name,
		Id:     e.Id,
		Links:  e.Links,
	}
}

func NewIdentityEntityRef(s *model.Identity) *EntityApiRef {
	links := &response.Links{
		"self": NewIdentityLink(s.Id),
	}

	return &EntityApiRef{
		Id:     s.Id,
		Name:   &s.Name,
		Links:  links,
		Entity: EntityNameIdentity,
	}
}

func NewIdentityLink(sessionId string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameIdentity, sessionId))
}

func MapIdentityToApiEntity(ae *env.AppEnv, _ *response.RequestContext, e model.BaseModelEntity) (BaseApiEntity, error) {
	i, ok := e.(*model.Identity)

	if !ok {
		err := fmt.Errorf("entity is not an identity \"%s\"", e.GetId())
		pfxlog.Logger().Error(err)
		return nil, err
	}

	al, err := MapToIdentityApiList(ae, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapToIdentityApiList(ae *env.AppEnv, i *model.Identity) (*IdentityApiList, error) {
	identityType, err := ae.Handlers.IdentityType.HandleRead(i.IdentityTypeId)
	if err != nil {
		return nil, err
	}
	it, err := MapIdentityTypeToApiEntity(nil, nil, identityType)

	if err != nil {
		return nil, err
	}
	ret := &IdentityApiList{
		BaseApi:        env.FromBaseModelEntity(i),
		Name:           &i.Name,
		Type:           it.ToEntityApiRef(),
		IsDefaultAdmin: &i.IsDefaultAdmin,
		IsAdmin:        &i.IsAdmin,
		Authenticators: map[string]interface{}{},
		Enrollments:    map[string]interface{}{},
		RoleAttributes: i.RoleAttributes,
	}

	err = ae.GetHandlers().Identity.HandleCollectEnrollments(ret.Id, func(entity model.BaseModelEntity) error {
		enrollmentModel, ok := entity.(*model.Enrollment)

		if !ok {
			return fmt.Errorf("entity is not an enrollment \"%s\"", entity.GetId())
		}

		var err error
		ret.Enrollments[enrollmentModel.Method], err = MapToIdentityEnrollmentApiList(ae, enrollmentModel)

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	err = ae.GetHandlers().Identity.HandleCollectAuthenticators(ret.Id, func(entity model.BaseModelEntity) error {
		authenticatorModel, ok := entity.(*model.Authenticator)

		if !ok {
			return fmt.Errorf("entity is not an enrollment \"%s\"", entity.GetId())
		}

		var err error
		ret.Authenticators[authenticatorModel.Method], err = MapToIdentityAuthenticatorApiList(ae, authenticatorModel)

		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	ret.PopulateLinks()

	return ret, nil
}

func MapToIdentityAuthenticatorApiList(_ *env.AppEnv, authenticator *model.Authenticator) (interface{}, error) {
	var ret map[string]interface{}
	switch authenticator.Method {
	case persistence.MethodAuthenticatorCert:
		cert := authenticator.ToCert()
		ret = map[string]interface{}{
			"fingerprint": cert.Fingerprint,
		}
	case persistence.MethodAuthenticatorUpdb:
		updb := authenticator.ToUpdb()
		ret = map[string]interface{}{
			"username": updb.Username,
		}
	default:
		return nil, fmt.Errorf("unknown authenticator method %s", authenticator.Method)
	}

	return ret, nil
}

func MapToIdentityEnrollmentApiList(_ *env.AppEnv, enrollment *model.Enrollment) (map[string]interface{}, error) {
	var ret map[string]interface{}
	switch enrollment.Method {
	case persistence.MethodEnrollOtt:
		ret = map[string]interface{}{
			"token":     enrollment.Token,
			"jwt":       enrollment.Jwt,
			"issuedAt":  enrollment.IssuedAt,
			"expiresAt": enrollment.ExpiresAt,
		}
	case persistence.MethodEnrollUpdb:
		ret = map[string]interface{}{
			"token":     enrollment.Token,
			"jwt":       enrollment.Jwt,
			"issuedAt":  enrollment.IssuedAt,
			"expiresAt": enrollment.ExpiresAt,
		}
	case persistence.MethodEnrollOttCa:
		ret = map[string]interface{}{
			"token":     enrollment.Token,
			"jwt":       enrollment.Jwt,
			"issuedAt":  enrollment.IssuedAt,
			"expiresAt": enrollment.ExpiresAt,
			"caId":      enrollment.CaId,
		}
	default:
		return nil, fmt.Errorf("unknown enollment method %s", enrollment.Method)
	}

	return ret, nil
}
