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
	"fmt"
	"strings"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/foundation/v2/util"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/controller/model"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/response"
)

const (
	EntityNameIdentity = "identities"
)

type PermissionsApi []string

var IdentityLinkFactory = NewIdentityLinkFactory(NewBasicLinkFactory(EntityNameIdentity))

func NewIdentityLinkFactory(selfFactory *BasicLinkFactory) *IdentityLinkFactoryImpl {
	return &IdentityLinkFactoryImpl{
		BasicLinkFactory: *selfFactory,
	}
}

type IdentityLinkFactoryImpl struct {
	BasicLinkFactory
}

func (factory *IdentityLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	links[EntityNameEdgeRouterPolicy] = factory.NewNestedLink(entity, EntityNameEdgeRouterPolicy)
	links[EntityNameEdgeRouter] = factory.NewNestedLink(entity, EntityNameEdgeRouter)
	links[EntityNameServicePolicy] = factory.NewNestedLink(entity, EntityNameServicePolicy)
	links[EntityNameService] = factory.NewNestedLink(entity, EntityNameService)
	links[EntityNamePostureData] = factory.NewNestedLink(entity, EntityNamePostureData)
	links[EntityNameFailedServiceRequest] = factory.NewNestedLink(entity, EntityNameFailedServiceRequest)
	links[EntityNameAuthenticator] = factory.NewNestedLink(entity, EntityNameAuthenticator)
	links[EntityNameEnrollment] = factory.NewNestedLink(entity, EntityNameEnrollment)
	links["service-configs"] = factory.NewNestedLink(entity, "service-configs")

	if identity, ok := entity.(*model.Identity); ok && identity != nil {
		links[EntityNameAuthPolicy] = AuthPolicyLinkFactory.SelfLinkFromId(identity.AuthPolicyId)
	}

	return links
}

func getDefaultHostingCost(v *rest_model.TerminatorCost) uint16 {
	if v == nil {
		return 0
	}

	return uint16(*v)
}

func getServiceHostingPrecedences(v rest_model.TerminatorPrecedenceMap) map[string]ziti.Precedence {
	result := map[string]ziti.Precedence{}
	for k, v := range v {
		result[k] = ziti.GetPrecedenceForLabel(string(v))
	}
	return result
}

func getRestServiceHostingPrecedences(v map[string]ziti.Precedence) rest_model.TerminatorPrecedenceMap {
	result := rest_model.TerminatorPrecedenceMap{}
	for k, v := range v {
		result[k] = rest_model.TerminatorPrecedence(v.String())
	}
	return result
}

func getServiceHostingCosts(costMap rest_model.TerminatorCostMap) map[string]uint16 {
	result := map[string]uint16{}
	for key, cost := range costMap {
		result[key] = uint16(*cost)
	}
	return result
}

func getRestServiceHostingCosts(costMap map[string]uint16) rest_model.TerminatorCostMap {
	result := rest_model.TerminatorCostMap{}
	for key, cost := range costMap {
		val := rest_model.TerminatorCost(cost)
		result[key] = &val
	}
	return result
}

func MapCreateIdentityToModel(identity *rest_model.IdentityCreate, identityTypeId string) (*model.Identity, []*model.Enrollment) {
	var enrollments []*model.Enrollment

	ret := &model.Identity{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(identity.Tags),
		},
		Name:                      stringz.OrEmpty(identity.Name),
		IdentityTypeId:            db.DefaultIdentityType,
		IsDefaultAdmin:            false,
		IsAdmin:                   *identity.IsAdmin,
		RoleAttributes:            AttributesOrDefault(identity.RoleAttributes),
		DefaultHostingPrecedence:  ziti.GetPrecedenceForLabel(string(identity.DefaultHostingPrecedence)),
		DefaultHostingCost:        getDefaultHostingCost(identity.DefaultHostingCost),
		ServiceHostingPrecedences: getServiceHostingPrecedences(identity.ServiceHostingPrecedences),
		ServiceHostingCosts:       getServiceHostingCosts(identity.ServiceHostingCosts),
		AppData:                   TagsOrDefault(identity.AppData),
		AuthPolicyId:              stringz.OrEmpty(identity.AuthPolicyID),
		ExternalId:                identity.ExternalID,
	}

	if identity.Enrollment != nil {
		if identity.Enrollment.Ott {
			enrollments = append(enrollments, &model.Enrollment{
				BaseEntity: models.BaseEntity{},
				Method:     db.MethodEnrollOtt,
				Token:      uuid.New().String(),
			})
		} else if identity.Enrollment.Ottca != "" {
			caId := identity.Enrollment.Ottca
			enrollments = append(enrollments, &model.Enrollment{
				BaseEntity: models.BaseEntity{},
				Method:     db.MethodEnrollOttCa,
				Token:      uuid.New().String(),
				CaId:       &caId,
			})
		} else if identity.Enrollment.Updb != "" {
			username := identity.Enrollment.Updb
			enrollments = append(enrollments, &model.Enrollment{
				BaseEntity: models.BaseEntity{},
				Method:     db.MethodEnrollUpdb,
				Token:      uuid.New().String(),
				Username:   &username,
			})
		}
	}

	return ret, enrollments
}

func MapUpdateIdentityToModel(id string, identity *rest_model.IdentityUpdate, identityTypeId string) *model.Identity {
	ret := &model.Identity{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(identity.Tags),
			Id:   id,
		},
		Name:                      stringz.OrEmpty(identity.Name),
		IdentityTypeId:            identityTypeId,
		IsAdmin:                   *identity.IsAdmin,
		RoleAttributes:            AttributesOrDefault(identity.RoleAttributes),
		DefaultHostingPrecedence:  ziti.GetPrecedenceForLabel(string(identity.DefaultHostingPrecedence)),
		DefaultHostingCost:        getDefaultHostingCost(identity.DefaultHostingCost),
		ServiceHostingPrecedences: getServiceHostingPrecedences(identity.ServiceHostingPrecedences),
		ServiceHostingCosts:       getServiceHostingCosts(identity.ServiceHostingCosts),
		AppData:                   TagsOrDefault(identity.AppData),
		AuthPolicyId:              stringz.OrEmpty(identity.AuthPolicyID),
		ExternalId:                identity.ExternalID,
	}

	return ret
}

func MapPatchIdentityToModel(id string, identity *rest_model.IdentityPatch, identityTypeId string) *model.Identity {
	ret := &model.Identity{
		BaseEntity: models.BaseEntity{
			Tags: TagsOrDefault(identity.Tags),
			Id:   id,
		},
		Name:                      stringz.OrEmpty(identity.Name),
		IdentityTypeId:            identityTypeId,
		IsAdmin:                   BoolOrDefault(identity.IsAdmin),
		RoleAttributes:            AttributesOrDefault(identity.RoleAttributes),
		DefaultHostingPrecedence:  ziti.GetPrecedenceForLabel(string(identity.DefaultHostingPrecedence)),
		DefaultHostingCost:        getDefaultHostingCost(identity.DefaultHostingCost),
		ServiceHostingPrecedences: getServiceHostingPrecedences(identity.ServiceHostingPrecedences),
		ServiceHostingCosts:       getServiceHostingCosts(identity.ServiceHostingCosts),
		AppData:                   TagsOrDefault(identity.AppData),
		AuthPolicyId:              stringz.OrEmpty(identity.AuthPolicyID),
		ExternalId:                identity.ExternalID,
	}

	return ret
}

func MapIdentityToRestEntity(ae *env.AppEnv, _ *response.RequestContext, entity *model.Identity) (interface{}, error) {
	return MapIdentityToRestModel(ae, entity)
}

func MapIdentityToRestModel(ae *env.AppEnv, identity *model.Identity) (*rest_model.IdentityDetail, error) {
	identityType, err := ae.Managers.IdentityType.ReadByIdOrName(identity.IdentityTypeId)

	if err != nil {
		return nil, err
	}

	mfa, err := ae.Managers.Mfa.ReadOneByIdentityId(identity.Id)
	if err != nil {
		return nil, err
	}

	isMfaEnabled := mfa != nil && mfa.IsVerified

	hasApiSession := false

	err = ae.GetManagers().ApiSession.StreamIds(fmt.Sprintf(`identity = "%s" limit 1`, identity.Id), func(s string, err error) error {
		hasApiSession = true
		return nil
	})

	var authPolicyRef *rest_model.EntityRef

	if identity.AuthPolicyId != "" {
		authPolicy, err := ae.Managers.AuthPolicy.Read(identity.AuthPolicyId)

		if err == nil {
			authPolicyRef = ToEntityRef(authPolicy.Name, authPolicy, AuthPolicyLinkFactory)
		} else {
			pfxlog.Logger().
				WithField("identityId", identity.Id).
				WithField("authPolicyId", identity.AuthPolicyId).
				Errorf("reading identity, detected auth policy id but failed to find it")
		}
	}

	if err != nil {
		pfxlog.Logger().Errorf("error attempting to determine identity id's [%s] API session existence: %v", identity.Id, err)
	}

	cost := rest_model.TerminatorCost(identity.DefaultHostingCost)

	if identity.RoleAttributes == nil {
		identity.RoleAttributes = []string{}
	}
	roleAttributes := rest_model.Attributes(identity.RoleAttributes)

	appData := rest_model.Tags{
		SubTags: identity.AppData,
	}

	if appData.SubTags == nil {
		appData.SubTags = map[string]interface{}{}
	}

	var disabledAt *strfmt.DateTime

	if identity.DisabledAt != nil {
		at := strfmt.DateTime(*identity.DisabledAt)
		disabledAt = &at
	}

	var disabledUntil *strfmt.DateTime

	if identity.DisabledUntil != nil {
		until := strfmt.DateTime(*identity.DisabledUntil)
		disabledUntil = &until
	}

	erConnState := identity.EdgeRouterConnectionStatus.String()

	ret := &rest_model.IdentityDetail{
		BaseEntity:                 BaseEntityToRestModel(identity, IdentityLinkFactory),
		AppData:                    &appData,
		AuthPolicy:                 authPolicyRef,
		AuthPolicyID:               &identity.AuthPolicyId,
		DefaultHostingCost:         &cost,
		DefaultHostingPrecedence:   rest_model.TerminatorPrecedence(identity.DefaultHostingPrecedence.String()),
		Disabled:                   &identity.Disabled,
		DisabledAt:                 disabledAt,
		DisabledUntil:              disabledUntil,
		EdgeRouterConnectionStatus: &erConnState,
		ExternalID:                 identity.ExternalId,
		HasAPISession:              &hasApiSession,
		HasEdgeRouterConnection:    &identity.HasErConnection,
		IsAdmin:                    &identity.IsAdmin,
		IsDefaultAdmin:             &identity.IsDefaultAdmin,
		IsMfaEnabled:               &isMfaEnabled,
		Name:                       &identity.Name,
		RoleAttributes:             &roleAttributes,
		ServiceHostingCosts:        getRestServiceHostingCosts(identity.ServiceHostingCosts),
		ServiceHostingPrecedences:  getRestServiceHostingPrecedences(identity.ServiceHostingPrecedences),
		Type:                       ToEntityRef(identityType.Name, identityType, IdentityTypeLinkFactory),
		TypeID:                     &identityType.Id,
	}

	for _, intf := range identity.Interfaces {
		apiIntf := &rest_model.Interface{
			HardwareAddress: &intf.HardwareAddress,
			Index:           &intf.Index,
			IsBroadcast:     util.Ptr(intf.IsBroadcast()),
			IsLoopback:      util.Ptr(intf.IsLoopback()),
			IsMulticast:     util.Ptr(intf.IsMulticast()),
			IsRunning:       util.Ptr(intf.IsRunning()),
			IsUp:            util.Ptr(intf.IsUp()),
			Mtu:             &intf.MTU,
			Name:            &intf.Name,
			Addresses:       intf.Addresses,
		}
		ret.Interfaces = append(ret.Interfaces, apiIntf)
	}

	fillInfo(ret, identity.EnvInfo, identity.SdkInfo)

	ret.Authenticators = &rest_model.IdentityAuthenticators{}
	if err = ae.GetManagers().Identity.CollectAuthenticators(identity.Id, func(entity *model.Authenticator) error {
		if entity.Method == db.MethodAuthenticatorUpdb {
			ret.Authenticators.Updb = &rest_model.IdentityAuthenticatorsUpdb{
				ID:       entity.Id,
				Username: entity.ToUpdb().Username,
			}
		}

		if entity.Method == db.MethodAuthenticatorCert {
			ret.Authenticators.Cert = &rest_model.IdentityAuthenticatorsCert{
				ID:          entity.Id,
				Fingerprint: entity.ToCert().Fingerprint,
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	ret.Enrollment = &rest_model.IdentityEnrollments{}
	if err := ae.GetManagers().Identity.CollectEnrollments(identity.Id, func(entity *model.Enrollment) error {
		var expiresAt strfmt.DateTime
		if entity.ExpiresAt != nil {
			expiresAt = strfmt.DateTime(*entity.ExpiresAt)
		}

		if entity.Method == db.MethodEnrollUpdb {

			ret.Enrollment.Updb = &rest_model.IdentityEnrollmentsUpdb{
				ID:        entity.Id,
				JWT:       entity.Jwt,
				Token:     entity.Token,
				ExpiresAt: expiresAt,
			}
		}

		if entity.Method == db.MethodEnrollOtt {
			ret.Enrollment.Ott = &rest_model.IdentityEnrollmentsOtt{
				ID:        entity.Id,
				JWT:       entity.Jwt,
				Token:     entity.Token,
				ExpiresAt: expiresAt,
			}
		}

		if entity.Method == db.MethodEnrollOttCa {
			if ca, err := ae.Managers.Ca.Read(*entity.CaId); err == nil {
				ret.Enrollment.Ottca = &rest_model.IdentityEnrollmentsOttca{
					ID:        entity.Id,
					Ca:        ToEntityRef(ca.Name, ca, CaLinkFactory),
					CaID:      ca.Id,
					JWT:       entity.Jwt,
					Token:     entity.Token,
					ExpiresAt: expiresAt,
				}
			} else {
				pfxlog.Logger().Errorf("could not read CA [%s] to render ottca enrollment for identity [%s]", stringz.OrEmpty(entity.CaId), identity.Id)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

func fillInfo(identity *rest_model.IdentityDetail, envInfo *model.EnvInfo, sdkInfo *model.SdkInfo) {
	if envInfo != nil {
		identity.EnvInfo = &rest_model.EnvInfo{
			Arch:      envInfo.Arch,
			Os:        envInfo.Os,
			OsRelease: envInfo.OsRelease,
			OsVersion: envInfo.OsVersion,
			Domain:    envInfo.Domain,
			Hostname:  envInfo.Hostname,
		}
	} else {
		identity.EnvInfo = &rest_model.EnvInfo{}
	}

	if sdkInfo != nil {
		identity.SdkInfo = &rest_model.SdkInfo{
			AppID:      sdkInfo.AppId,
			AppVersion: sdkInfo.AppVersion,
			Branch:     sdkInfo.Branch,
			Revision:   sdkInfo.Revision,
			Type:       sdkInfo.Type,
			Version:    sdkInfo.Version,
		}
	} else {
		identity.SdkInfo = &rest_model.SdkInfo{}
	}
}

func MapServiceConfigToModel(config rest_model.ServiceConfigAssign) model.ServiceConfig {
	return model.ServiceConfig{
		Service: stringz.OrEmpty(config.ServiceID),
		Config:  stringz.OrEmpty(config.ConfigID),
	}
}
func MapAdvisorServiceReachabilityToRestEntity(entity *model.AdvisorServiceReachability) *rest_model.PolicyAdvice {

	var commonRouters []*rest_model.RouterEntityRef

	for _, router := range entity.CommonRouters {
		commonRouters = append(commonRouters, &rest_model.RouterEntityRef{
			EntityRef: *ToEntityRef(router.Router.Name, router.Router, EdgeRouterLinkFactory),
			IsOnline:  &router.IsOnline,
		})
	}

	result := &rest_model.PolicyAdvice{
		IdentityID:          entity.Identity.Id,
		Identity:            ToEntityRef(entity.Identity.Name, entity.Identity, IdentityLinkFactory),
		ServiceID:           entity.Service.Id,
		Service:             ToEntityRef(entity.Service.Name, entity.Service, ServiceLinkFactory),
		IsBindAllowed:       entity.IsBindAllowed,
		IsDialAllowed:       entity.IsDialAllowed,
		IdentityRouterCount: int32(entity.IdentityRouterCount),
		ServiceRouterCount:  int32(entity.ServiceRouterCount),
		CommonRouters:       commonRouters,
	}

	return result
}

func GetNamedIdentityRoles(identityHandler *model.IdentityManager, roles []string) rest_model.NamedRoles {
	result := rest_model.NamedRoles{}
	for _, role := range roles {
		if strings.HasPrefix(role, "@") {

			identity, err := identityHandler.Read(role[1:])
			if err != nil {
				pfxlog.Logger().Errorf("error converting identity role [%s] to a named role: %v", role, err)
				continue
			}

			result = append(result, &rest_model.NamedRole{
				Role: role,
				Name: "@" + identity.Name,
			})
		} else {
			result = append(result, &rest_model.NamedRole{
				Role: role,
				Name: role,
			})
		}

	}
	return result
}
