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
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"path"
)

const EntityNameCurrentIdentity = "current-identity"
const EntityNameMfa = "mfa"

var CurrentIdentityLinkFactory FullLinkFactory = NewCurrentIdentityLinkFactory()
var CurrentIdentityMfaLinkFactory FullLinkFactory = NewCurrentIdentityMfaLinkFactory()

type CurrentIdentityLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewCurrentIdentityLinkFactory() *CurrentIdentityLinkFactoryImpl {
	return &CurrentIdentityLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCurrentIdentity),
	}
}

func (factory *CurrentIdentityLinkFactoryImpl) SelfUrlString(_ string) string {
	return "./" + factory.entityName
}

func (factory CurrentIdentityLinkFactoryImpl) NewNestedLink(_ models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{factory.SelfUrlString("")}, elem...)
	return NewLink("./" + path.Join(elem...))
}

func (factory *CurrentIdentityLinkFactoryImpl) SelfLink(_ models.Entity) rest_model.Link {
	return NewLink("./" + factory.entityName)
}

func (factory *CurrentIdentityLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf: factory.SelfLink(entity),
		EntityNameMfa:  factory.NewNestedLink(nil, "mfa"),
	}
}

type CurrentIdentityMfaLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewCurrentIdentityMfaLinkFactory() *CurrentIdentityMfaLinkFactoryImpl {
	return &CurrentIdentityMfaLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCurrentIdentity + "/" + EntityNameMfa),
	}
}

func (factory *CurrentIdentityMfaLinkFactoryImpl) SelfUrlString(_ string) string {
	return "./" + factory.entityName
}

func (factory CurrentIdentityMfaLinkFactoryImpl) NewNestedLink(_ models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{factory.SelfUrlString("")}, elem...)
	return NewLink("./" + path.Join(elem...))
}

func (factory *CurrentIdentityMfaLinkFactoryImpl) SelfLink(_ models.Entity) rest_model.Link {
	return NewLink("./" + factory.entityName)
}

func (factory *CurrentIdentityMfaLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := rest_model.Links{
		EntityNameSelf:            factory.SelfLink(entity),
		EntityNameCurrentIdentity: CurrentIdentityLinkFactory.SelfLink(entity),
	}

	if mfa := entity.(*model.Mfa); mfa != nil {
		if !mfa.IsVerified {
			links["verify"] = factory.NewNestedLink(nil, "verify")
			links["qr"] = factory.NewNestedLink(nil, "qr-code")
		} else {
			links["recovery-codes"] = factory.NewNestedLink(nil, "recovery-codes")
		}
	}

	return links
}

func MapMfaToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	mfa, ok := e.(*model.Mfa)

	if !ok {
		err := fmt.Errorf("entity is not a Mfa \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapMfaToRestModel(ae, mfa)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapMfaToRestModel(ae *env.AppEnv, mfa *model.Mfa) (*rest_model.DetailMfa, error) {
	result := &rest_model.DetailMfa{
		BaseEntity: BaseEntityToRestModel(mfa, CurrentIdentityMfaLinkFactory),
		IsVerified: &mfa.IsVerified,
	}

	if !mfa.IsVerified {
		result.RecoveryCodes = mfa.RecoveryCodes
		result.ProvisioningURL = ae.Managers.Mfa.GetProvisioningUrl(mfa)
	}

	return result, nil
}

func MapCurrentIdentityEdgeRouterToRestEntity(ae *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	router, ok := e.(*model.EdgeRouter)

	if !ok {
		err := fmt.Errorf("entity is not a EdgeRouter \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	restModel, err := MapCurrentIdentityEdgeRouterToRestModel(ae, router)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return restModel, nil
}

func MapCurrentIdentityEdgeRouterToRestModel(ae *env.AppEnv, router *model.EdgeRouter) (*rest_model.CurrentIdentityEdgeRouterDetail, error) {
	hostname := ""

	routerState := ae.Broker.GetEdgeRouterState(router.Id)

	syncStatus := string(routerState.SyncStatus)

	ret := &rest_model.CurrentIdentityEdgeRouterDetail{
		BaseEntity: BaseEntityToRestModel(router, EdgeRouterLinkFactory),
		CommonEdgeRouterProperties: rest_model.CommonEdgeRouterProperties{
			Hostname:           &hostname,
			IsOnline:           &routerState.IsOnline,
			Name:               &router.Name,
			SupportedProtocols: routerState.Protocols,
			SyncStatus:         &syncStatus,
		},
	}

	return ret, nil
}
