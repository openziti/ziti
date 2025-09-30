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

package routes

import (
	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/response"
)

const EntityNameSession = "sessions"
const EntityNameRoutePath = "route-path"

var SessionLinkFactory = NewSessionLinkFactory()

type SessionLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewSessionLinkFactory() *SessionLinkFactoryImpl {
	return &SessionLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameSession),
	}
}

func (factory *SessionLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameRoutePath] = factory.NewNestedLink(entity, EntityNameRoutePath)
	return links
}

func MapCreateSessionToModel(identityId, apiSessionId string, session *rest_model.SessionCreate) *model.Session {
	id := cuid.New()

	ret := &model.Session{
		BaseEntity: models.BaseEntity{
			Id:   id,
			Tags: TagsOrDefault(session.Tags),
		},
		//token is redundant now as these are encoded as JWTs, filled for existing subsystem compatibility
		Token:           id,
		ApiSessionId:    apiSessionId,
		ServiceId:       session.ServiceID,
		IdentityId:      identityId,
		Type:            string(session.Type),
		ServicePolicies: nil,
	}

	return ret
}

func MapSessionToRestEntity(ae *env.AppEnv, _ *response.RequestContext, session *model.Session) (interface{}, error) {
	return MapSessionToRestModel(ae, session)
}

func MapSessionToRestModel(ae *env.AppEnv, sessionModel *model.Session) (*rest_model.SessionManagementDetail, error) {
	service, err := ae.Managers.EdgeService.Read(sessionModel.ServiceId)
	if err != nil {
		pfxlog.Logger().Errorf("could not render service [%s] for Session [%s] - should not be possible", sessionModel.ServiceId, sessionModel.Id)
	}

	var serviceRef *rest_model.EntityRef
	if service != nil {
		serviceRef = ToEntityRef(service.Name, service, ServiceLinkFactory)
	}

	edgeRouters, err := getSessionEdgeRouters(ae, sessionModel)
	if err != nil {
		pfxlog.Logger().Errorf("could not render edge routers for Session [%s]: %v", sessionModel.Id, err)
	}

	apiSession, err := ae.Managers.ApiSession.Read(sessionModel.ApiSessionId)
	if err != nil {
		pfxlog.Logger().Errorf("could not render API Session [%s] for Session [%s], orphaned session - should not be possible", sessionModel.ApiSessionId, sessionModel.Id)
	}

	var apiSessionRef *rest_model.EntityRef
	if apiSession != nil {
		apiSessionRef = ToEntityRef("", apiSession, ApiSessionLinkFactory)
	}

	dialBindType := rest_model.DialBind(sessionModel.Type)

	servicePolicyRefs := []*rest_model.EntityRef{} //send `[]` not `null`

	for _, servicePolicyId := range sessionModel.ServicePolicies {
		if policy, _ := ae.GetManagers().ServicePolicy.Read(servicePolicyId); policy != nil {
			ref := &rest_model.EntityRef{
				Links:  ServicePolicyLinkFactory.Links(policy),
				Entity: EntityNameServicePolicy,
				ID:     servicePolicyId,
				Name:   policy.Name,
			}

			servicePolicyRefs = append(servicePolicyRefs, ref)
		}

	}

	ret := &rest_model.SessionManagementDetail{
		SessionDetail: rest_model.SessionDetail{
			BaseEntity:   BaseEntityToRestModel(sessionModel, SessionLinkFactory),
			APISession:   apiSessionRef,
			APISessionID: &sessionModel.ApiSessionId,
			Service:      serviceRef,
			ServiceID:    &sessionModel.ServiceId,
			IdentityID:   &sessionModel.IdentityId,
			EdgeRouters:  edgeRouters,
			Type:         &dialBindType,
			Token:        &sessionModel.Token,
		},
		ServicePolicies: servicePolicyRefs,
	}

	return ret, nil
}

func MapSessionsToRestEntities(ae *env.AppEnv, rc *response.RequestContext, sessions []*model.Session) ([]interface{}, error) {
	var ret []interface{}
	for _, session := range sessions {
		restEntity, err := MapSessionToRestEntity(ae, rc, session)

		if err != nil {
			return nil, err
		}

		ret = append(ret, restEntity)
	}

	return ret, nil
}

func getSessionEdgeRouters(ae *env.AppEnv, ns *model.Session) ([]*rest_model.SessionEdgeRouter, error) {
	var edgeRouters []*rest_model.SessionEdgeRouter

	edgeRoutersForSession, err := ae.Managers.EdgeRouter.ListForIdentityAndService(ns.IdentityId, ns.ServiceId)
	if err != nil {
		return nil, err
	}

	for _, edgeRouter := range edgeRoutersForSession.EdgeRouters {
		state := ae.Broker.GetEdgeRouterState(edgeRouter.Id)

		syncStatus := string(state.SyncStatus)
		cost := int64(edgeRouter.Cost)
		restModel := &rest_model.SessionEdgeRouter{
			CommonEdgeRouterProperties: rest_model.CommonEdgeRouterProperties{
				Hostname:           &state.Hostname,
				IsOnline:           &state.IsOnline,
				Name:               &edgeRouter.Name,
				SupportedProtocols: state.Protocols,
				SyncStatus:         &syncStatus,
				Cost:               &cost,
				NoTraversal:        &edgeRouter.NoTraversal,
				Disabled:           &edgeRouter.Disabled,
			},
			// `urls` is deprecated and should be removed once older SDKs that rely on it are not longer in use
			Urls: state.Protocols,
		}

		pfxlog.Logger().Debugf("Returning %+v to %+v, with urls: %+v", edgeRouter, restModel, restModel.SupportedProtocols)
		edgeRouters = append(edgeRouters, restModel)
	}

	return edgeRouters, nil
}
