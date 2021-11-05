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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/stringz"
)

const EntityNameAuthenticator = "authenticators"

var AuthenticatorLinkFactory = NewAuthenticatorLinkFactory()

type AuthenticatorLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewAuthenticatorLinkFactory() *AuthenticatorLinkFactoryImpl {
	return &AuthenticatorLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameAuthenticator),
	}
}

func (factory *AuthenticatorLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	authenticator := entity.(*model.Authenticator)

	links := factory.BasicLinkFactory.Links(entity)
	if authenticator != nil {
		links[EntityNameIdentity] = IdentityLinkFactory.SelfLinkFromId(authenticator.IdentityId)
	}

	return links
}

func MapCreateToAuthenticatorModel(in *rest_model.AuthenticatorCreate) (*model.Authenticator, error) {
	result := &model.Authenticator{
		BaseEntity: models.BaseEntity{},
		Method:     stringz.OrEmpty(in.Method),
		IdentityId: stringz.OrEmpty(in.IdentityID),
		SubType:    nil,
	}
	var subType interface{}

	switch result.Method {
	case persistence.MethodAuthenticatorCert:
		if in.CertPem == "" {
			return nil, errorz.NewFieldError("certPem is required", "certPem", in.CertPem)
		}

		subType = &model.AuthenticatorCert{
			Pem: in.CertPem,
		}
	case persistence.MethodAuthenticatorUpdb:
		if in.Username == "" {
			return nil, errorz.NewFieldError("username is required", "username", in.Username)
		}

		if in.Password == "" {
			return nil, errorz.NewFieldError("password is required", "password", in.Password)
		}

		subType = &model.AuthenticatorUpdb{
			Authenticator: result,
			Username:      in.Username,
			Password:      in.Password,
			Salt:          "",
		}
	default:
		return nil, errorz.NewFieldError("method must be updb or cert", "method", in.Method)
	}

	result.SubType = subType

	return result, nil
}

func MapUpdateAuthenticatorToModel(id string, in *rest_model.AuthenticatorUpdate) *model.Authenticator {
	result := &model.Authenticator{
		BaseEntity: models.BaseEntity{
			Id:   id,
			Tags: TagsOrDefault(in.Tags),
		},
		Method: persistence.MethodAuthenticatorUpdb,
	}

	result.SubType = &model.AuthenticatorUpdb{
		Authenticator: result,
		Username:      string(*in.Username),
		Password:      string(*in.Password),
		Salt:          "",
	}

	return result
}

func MapPatchAuthenticatorToModel(id string, in *rest_model.AuthenticatorPatch) *model.Authenticator {
	result := &model.Authenticator{
		BaseEntity: models.BaseEntity{
			Id:   id,
			Tags: TagsOrDefault(in.Tags),
		},
		Method: persistence.MethodAuthenticatorUpdb,
	}

	subType := &model.AuthenticatorUpdb{
		Authenticator: result,
		Salt:          "",
	}

	if in.Username != nil {
		subType.Username = string(*in.Username)
	}

	if in.Password != nil {
		subType.Password = string(*in.Password)
	}

	result.SubType = subType

	return result
}

func MapAuthenticatorToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	i, ok := e.(*model.Authenticator)

	if !ok {
		err := fmt.Errorf("entity is not an authenticator \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapAuthenticatorToRestModel(ae, i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapAuthenticatorToRestModel(ae *env.AppEnv, i *model.Authenticator) (*rest_model.AuthenticatorDetail, error) {

	identity, err := ae.GetHandlers().Identity.Read(i.IdentityId)

	if err != nil {
		return nil, err
	}

	result := &rest_model.AuthenticatorDetail{
		BaseEntity: BaseEntityToRestModel(i, AuthenticatorLinkFactory),
		Method:     &i.Method,
		IdentityID: &i.IdentityId,
		Identity:   ToEntityRef(identity.Name, identity, IdentityLinkFactory),
	}

	switch i.Method {
	case persistence.MethodAuthenticatorUpdb:
		subType := i.SubType.(*model.AuthenticatorUpdb)
		result.Username = subType.Username
	case persistence.MethodAuthenticatorCert:
		subType := i.SubType.(*model.AuthenticatorCert)
		result.CertPem = subType.Pem
		result.Fingerprint = subType.Fingerprint
	}

	return result, nil
}

func MapAuthenticatorsToRestEntities(ae *env.AppEnv, rc *response.RequestContext, es []*model.Authenticator) ([]*rest_model.AuthenticatorDetail, error) {
	apiEntities := make([]*rest_model.AuthenticatorDetail, 0)

	for _, e := range es {
		al, err := MapAuthenticatorToRestModel(ae, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}
