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

package routes

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/model"
	"github.com/netfoundry/ziti-edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/controller/response"
	"github.com/netfoundry/ziti-fabric/controller/models"
	"github.com/netfoundry/ziti-foundation/util/stringz"
)

const EntityNameAuthenticator = "authenticators"

type AuthenticatorProperties struct {
	Username    *string `json:"username, omitempty"`
	Password    *string `json:"password, omitempty"`
	CertPem     *string `json:"certPem, omitempty"`
	Fingerprint *string `json:"fingerprint, omitempty"`
}

type AuthenticatorCreateApi struct {
	Method     *string `json:"method"`
	IdentityId *string `json:"identityId"`
	AuthenticatorProperties
	Tags map[string]interface{} `json:"tags"`
}

func (i *AuthenticatorCreateApi) ToModel(id string) *model.Authenticator {
	result := &model.Authenticator{}
	result.Id = id
	result.Method = stringz.OrEmpty(i.Method)
	result.IdentityId = stringz.OrEmpty(i.IdentityId)

	if i.Username != nil || i.Password != nil {
		result.Method = persistence.MethodAuthenticatorUpdb
	} else {
		result.Method = persistence.MethodAuthenticatorCert
	}

	switch result.Method {
	case persistence.MethodAuthenticatorUpdb:
		result.SubType = &model.AuthenticatorUpdb{
			Authenticator: result,
			Username:      stringz.OrEmpty(i.Username),
			Password:      stringz.OrEmpty(i.Password),
			Salt:          "",
		}
	case persistence.MethodAuthenticatorCert:
		result.SubType = &model.AuthenticatorCert{
			Authenticator: result,
			Pem:           stringz.OrEmpty(i.CertPem),
		}
	}

	result.Tags = i.Tags
	return result
}

func (i *AuthenticatorCreateApi) FillFromMap(in map[string]interface{}) {
	if val, ok := in["username"]; ok && val != nil {
		username := val.(string)
		i.Username = &username
	}

	if val, ok := in["password"]; ok && val != nil {
		password := val.(string)
		i.Password = &password
	}

	if val, ok := in["certPem"]; ok && val != nil {
		certPem := val.(string)
		i.CertPem = &certPem
	}

	if val, ok := in["tags"]; ok && val != nil {
		tags := val.(map[string]interface{})
		i.Tags = tags
	}

	if val, ok := in["identityId"]; ok && val != nil {
		identityId := val.(string)
		i.IdentityId = &identityId
	}
}

type AuthenticatorUpdateApi struct {
	AuthenticatorProperties
	Tags map[string]interface{} `json:"tags"`
}

func (i *AuthenticatorUpdateApi) FillFromMap(in map[string]interface{}) {
	if val, ok := in["username"]; ok && val != nil {
		username := val.(string)
		i.Username = &username
	}

	if val, ok := in["password"]; ok && val != nil {
		password := val.(string)
		i.Password = &password
	}

	if val, ok := in["certPem"]; ok && val != nil {
		certPem := val.(string)
		i.CertPem = &certPem
	}

	if val, ok := in["tags"]; ok && val != nil {
		tags := val.(map[string]interface{})
		i.Tags = tags
	}
}

func (i *AuthenticatorUpdateApi) ToModel(id string) *model.Authenticator {
	result := &model.Authenticator{}
	result.Id = id

	if i.Username != nil || i.Password != nil {
		result.Method = persistence.MethodAuthenticatorUpdb
	} else {
		result.Method = persistence.MethodAuthenticatorCert
	}

	switch result.Method {
	case persistence.MethodAuthenticatorUpdb:
		result.SubType = &model.AuthenticatorUpdb{
			Authenticator: result,
			Username:      stringz.OrEmpty(i.Username),
			Password:      stringz.OrEmpty(i.Password),
			Salt:          "",
		}
	case persistence.MethodAuthenticatorCert:
		result.SubType = &model.AuthenticatorCert{
			Authenticator: result,
			Pem:           stringz.OrEmpty(i.CertPem),
		}
	}

	result.Tags = i.Tags
	return result
}

type AuthenticatorApiList struct {
	*env.BaseApi
	Method     *string `json:"method"`
	IdentityId *string `json:"identityId"`
	*AuthenticatorProperties
	Tags map[string]interface{} `json:"tags"`
}

func (c *AuthenticatorApiList) GetSelfLink() *response.Link {
	return c.BuildSelfLink(c.Id)
}

func (AuthenticatorApiList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameAuthenticator, id))
}

func (c *AuthenticatorApiList) PopulateLinks() {
	if c.Links == nil {
		self := c.GetSelfLink()
		c.Links = &response.Links{
			EntityNameSelf:     self,
			EntityNameIdentity: NewIdentityLink(*c.IdentityId),
		}
	}
}

func (c *AuthenticatorApiList) ToEntityApiRef() *EntityApiRef {
	c.PopulateLinks()
	return &EntityApiRef{
		Entity: EntityNameAuthenticator,
		Id:     c.Id,
		Links:  c.Links,
	}
}

func MapAuthenticatorToApiEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (BaseApiEntity, error) {
	i, ok := e.(*model.Authenticator)

	if !ok {
		err := fmt.Errorf("entity is not an authenticator \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapAuthenticatorToApiList(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapAuthenticatorToApiList(i *model.Authenticator) (*AuthenticatorApiList, error) {
	ret := &AuthenticatorApiList{
		BaseApi:    env.FromBaseModelEntity(i),
		Method:     &i.Method,
		IdentityId: &i.IdentityId,
		Tags:       i.Tags,
	}

	switch i.Method {
	case persistence.MethodAuthenticatorUpdb:
		subType := i.SubType.(*model.AuthenticatorUpdb)
		ret.AuthenticatorProperties = &AuthenticatorProperties{
			Username: &subType.Username,
		}
	case persistence.MethodAuthenticatorCert:
		subType := i.SubType.(*model.AuthenticatorCert)
		ret.AuthenticatorProperties = &AuthenticatorProperties{
			CertPem:     &subType.Pem,
			Fingerprint: &subType.Fingerprint,
		}
	}

	ret.PopulateLinks()

	return ret, nil
}

func MapAuthenticatorsToApiEntities(ae *env.AppEnv, rc *response.RequestContext, es []*model.Authenticator) ([]BaseApiEntity, error) {
	apiEntities := make([]BaseApiEntity, 0)

	for _, e := range es {
		al, err := MapAuthenticatorToApiEntity(ae, rc, e)

		if err != nil {
			return nil, err
		}

		apiEntities = append(apiEntities, al)
	}

	return apiEntities, nil
}
