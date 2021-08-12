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

package model

import (
	"fmt"
	"github.com/lucsky/cuid"
	"github.com/openziti/edge/controller/apierror"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/storage/ast"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/errorz"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
	"time"
)

func NewSessionHandler(env Env) *SessionHandler {
	handler := &SessionHandler{
		baseHandler: newBaseHandler(env, env.GetStores().Session),
	}
	handler.impl = handler
	return handler
}

type SessionHandler struct {
	baseHandler
}

func (handler *SessionHandler) newModelEntity() boltEntitySink {
	return &Session{}
}

type SessionPostureResult struct {
	Passed           bool
	Failure          *PostureSessionRequestFailure
	PassingPolicyIds []string
	Cause            *apierror.GenericCauseError
}

func (handler *SessionHandler) EvaluatePostureForService(identityId, apiSessionId, sessionType, serviceId, serviceName string) *SessionPostureResult {

	failureByPostureCheckId := map[string]*PostureCheckFailure{} //cache individual check status
	validPosture := false
	hasMatchingPolicies := false

	policyPostureCheckMap := handler.GetEnv().GetHandlers().EdgeService.GetPolicyPostureChecks(identityId, serviceId)

	failedPolicies := map[string][]*PostureCheckFailure{}
	failedPoliciesIdToName := map[string]string{}

	var failedPolicyIds []string
	var successPolicyIds []string

	for policyId, policyPostureCheck := range policyPostureCheckMap {

		if policyPostureCheck.PolicyType.String() != sessionType {
			continue
		}
		hasMatchingPolicies = true
		var failedChecks []*PostureCheckFailure

		for _, postureCheck := range policyPostureCheck.PostureChecks {

			found := false

			if _, found = failureByPostureCheckId[postureCheck.Id]; !found {
				_, failureByPostureCheckId[postureCheck.Id] = handler.GetEnv().GetHandlers().PostureResponse.Evaluate(identityId, apiSessionId, postureCheck)
			}

			if failureByPostureCheckId[postureCheck.Id] != nil {
				failedChecks = append(failedChecks, failureByPostureCheckId[postureCheck.Id])
			}
		}

		if len(failedChecks) == 0 {
			validPosture = true
			successPolicyIds = append(successPolicyIds, policyId)
		} else {
			//save for error output
			failedPolicies[policyId] = failedChecks
			failedPoliciesIdToName[policyId] = policyPostureCheck.PolicyName
			failedPolicyIds = append(failedPolicyIds, policyId)
		}
	}

	if hasMatchingPolicies && !validPosture {
		failureMap := map[string]interface{}{}

		sessionFailure := &PostureSessionRequestFailure{
			When:           time.Now(),
			ServiceId:      serviceId,
			ServiceName:    serviceName,
			ApiSessionId:   apiSessionId,
			SessionType:    sessionType,
			PolicyFailures: []*PosturePolicyFailure{},
		}

		for policyId, failures := range failedPolicies {
			policyFailure := &PosturePolicyFailure{
				PolicyId:   policyId,
				PolicyName: failedPoliciesIdToName[policyId],
				Checks:     failures,
			}

			var outFailures []interface{}

			for _, failure := range failures {
				outFailures = append(outFailures, failure.ToClientErrorData())
			}
			failureMap[policyId] = outFailures

			sessionFailure.PolicyFailures = append(sessionFailure.PolicyFailures, policyFailure)
		}

		cause := &apierror.GenericCauseError{
			Message: fmt.Sprintf("Failed to pass posture checks for service policies: %v", failedPolicyIds),
			DataMap: failureMap,
		}

		return &SessionPostureResult{
			Passed:           false,
			Cause:            cause,
			PassingPolicyIds: nil,
			Failure:          sessionFailure,
		}
	}

	return &SessionPostureResult{
		Passed:           true,
		Cause:            nil,
		PassingPolicyIds: successPolicyIds,
		Failure:          nil,
	}
}

func (handler *SessionHandler) Create(entity *Session) (string, error) {
	entity.Id = cuid.New() //use cuids which are longer than shortids but are monotonic

	apiSession, err := handler.GetEnv().GetHandlers().ApiSession.Read(entity.ApiSessionId)
	if err != nil {
		return "", err
	}
	if apiSession == nil {
		return "", errorz.NewFieldError("api session not found", "ApiSessionId", entity.ApiSessionId)
	}

	service, err := handler.GetEnv().GetHandlers().EdgeService.ReadForIdentity(entity.ServiceId, apiSession.IdentityId, nil)
	if err != nil {
		return "", err
	}

	if entity.Type == "" {
		entity.Type = persistence.SessionTypeDial
	}

	if persistence.SessionTypeDial == entity.Type && !stringz.Contains(service.Permissions, persistence.PolicyTypeDialName) {
		return "", errorz.NewFieldError("service not found", "ServiceId", entity.ServiceId)
	}

	if persistence.SessionTypeBind == entity.Type && !stringz.Contains(service.Permissions, persistence.PolicyTypeBindName) {
		return "", errorz.NewFieldError("service not found", "ServiceId", entity.ServiceId)
	}

	policyResult := handler.EvaluatePostureForService(apiSession.IdentityId, apiSession.Id, entity.Type, service.Id, service.Name)

	if !policyResult.Passed {
		handler.env.GetHandlers().PostureResponse.postureCache.AddSessionRequestFailure(apiSession.IdentityId, policyResult.Failure)
		return "", apierror.NewInvalidPosture(policyResult.Cause)
	}

	maxRows := 1
	result, err := handler.GetEnv().GetHandlers().EdgeRouter.ListForIdentityAndService(apiSession.IdentityId, entity.ServiceId, &maxRows)
	if err != nil {
		return "", err
	}
	if result.Count < 1 {
		return "", apierror.NewNoEdgeRoutersAvailable()
	}

	entity.ServicePolicies = policyResult.PassingPolicyIds

	return handler.createEntity(entity)
}

func (handler *SessionHandler) ReadByToken(token string) (*Session, error) {
	modelSession := &Session{}
	tokenIndex := handler.env.GetStores().Session.GetTokenIndex()
	if err := handler.readEntityWithIndex("token", []byte(token), tokenIndex, modelSession); err != nil {
		return nil, err
	}
	return modelSession, nil
}

func (handler *SessionHandler) ReadForIdentity(id string, identityId string) (*Session, error) {
	identity, err := handler.GetEnv().GetHandlers().Identity.Read(identityId)

	if err != nil {
		return nil, err
	}
	if identity.IsAdmin {
		return handler.Read(id)
	}

	query := fmt.Sprintf(`id = "%v" and apiSession.identity = "%v"`, id, identityId)
	result, err := handler.Query(query)
	if err != nil {
		return nil, err
	}
	if len(result.Sessions) == 0 {
		return nil, boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}
	return result.Sessions[0], nil
}

func (handler *SessionHandler) Read(id string) (*Session, error) {
	entity := &Session{}
	if err := handler.readEntity(id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *SessionHandler) readInTx(tx *bbolt.Tx, id string) (*Session, error) {
	entity := &Session{}
	if err := handler.readEntityInTx(tx, id, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

func (handler *SessionHandler) DeleteForIdentity(id, identityId string) error {
	session, err := handler.ReadForIdentity(id, identityId)
	if err != nil {
		return err
	}
	if session == nil {
		return boltz.NewNotFoundError(handler.GetStore().GetSingularEntityType(), "id", id)
	}
	return handler.deleteEntity(id)
}

func (handler *SessionHandler) Delete(id string) error {
	return handler.deleteEntity(id)
}

func (handler *SessionHandler) PublicQueryForIdentity(sessionIdentity *Identity, query ast.Query) (*SessionListResult, error) {
	if sessionIdentity.IsAdmin {
		return handler.querySessions(query)
	}
	identityFilterString := fmt.Sprintf(`apiSession.identity = "%v"`, sessionIdentity.Id)
	identityFilter, err := ast.Parse(handler.Store, identityFilterString)
	if err != nil {
		return nil, err
	}
	query.SetPredicate(ast.NewAndExprNode(query.GetPredicate(), identityFilter))
	return handler.querySessions(query)
}

func (handler *SessionHandler) ReadSessionCerts(sessionId string) ([]*SessionCert, error) {
	var result []*SessionCert
	err := handler.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		certs, err := handler.GetEnv().GetStores().Session.LoadCerts(tx, sessionId)
		if err != nil {
			return err
		}
		for _, cert := range certs {
			modelSessionCert := &SessionCert{}
			if err = modelSessionCert.FillFrom(handler, tx, cert); err != nil {
				return err
			}
			result = append(result, modelSessionCert)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) Query(query string) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) querySessions(query ast.Query) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	err := handler.preparedList(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (handler *SessionHandler) ListSessionsForEdgeRouter(edgeRouterId string) (*SessionListResult, error) {
	result := &SessionListResult{handler: handler}
	query := fmt.Sprintf(`anyOf(apiSession.identity.edgeRouterPolicies.routers) = "%v" and `+
		`anyOf(service.serviceEdgeRouterPolicies.routers) = "%v"`, edgeRouterId, edgeRouterId)
	err := handler.list(query, result.collect)
	if err != nil {
		return nil, err
	}
	return result, nil
}

type SessionListResult struct {
	handler  *SessionHandler
	Sessions []*Session
	models.QueryMetaData
}

func (result *SessionListResult) collect(tx *bbolt.Tx, ids []string, queryMetaData *models.QueryMetaData) error {
	result.QueryMetaData = *queryMetaData
	for _, key := range ids {
		entity, err := result.handler.readInTx(tx, key)
		if err != nil {
			return err
		}
		result.Sessions = append(result.Sessions, entity)
	}
	return nil
}
