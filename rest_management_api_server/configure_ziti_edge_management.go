// This file is safe to edit. Once it exists it will not be overwritten

//
// Copyright NetFoundry, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// __          __              _
// \ \        / /             (_)
//  \ \  /\  / /_ _ _ __ _ __  _ _ __   __ _
//   \ \/  \/ / _` | '__| '_ \| | '_ \ / _` |
//    \  /\  / (_| | |  | | | | | | | | (_| | : This file is generated, do not edit it.
//     \/  \/ \__,_|_|  |_| |_|_|_| |_|\__, |
//                                      __/ |
//                                     |___/

package rest_management_api_server

import (
	"crypto/tls"
	"io"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/openziti/edge/rest_management_api_server/operations"
	"github.com/openziti/edge/rest_management_api_server/operations/api_session"
	"github.com/openziti/edge/rest_management_api_server/operations/authentication"
	"github.com/openziti/edge/rest_management_api_server/operations/authenticator"
	"github.com/openziti/edge/rest_management_api_server/operations/certificate_authority"
	"github.com/openziti/edge/rest_management_api_server/operations/config"
	"github.com/openziti/edge/rest_management_api_server/operations/current_api_session"
	"github.com/openziti/edge/rest_management_api_server/operations/current_identity"
	"github.com/openziti/edge/rest_management_api_server/operations/database"
	"github.com/openziti/edge/rest_management_api_server/operations/edge_router"
	"github.com/openziti/edge/rest_management_api_server/operations/edge_router_policy"
	"github.com/openziti/edge/rest_management_api_server/operations/enrollment"
	"github.com/openziti/edge/rest_management_api_server/operations/identity"
	"github.com/openziti/edge/rest_management_api_server/operations/informational"
	"github.com/openziti/edge/rest_management_api_server/operations/posture_checks"
	"github.com/openziti/edge/rest_management_api_server/operations/role_attributes"
	"github.com/openziti/edge/rest_management_api_server/operations/router"
	"github.com/openziti/edge/rest_management_api_server/operations/service"
	"github.com/openziti/edge/rest_management_api_server/operations/service_edge_router_policy"
	"github.com/openziti/edge/rest_management_api_server/operations/service_policy"
	"github.com/openziti/edge/rest_management_api_server/operations/session"
	"github.com/openziti/edge/rest_management_api_server/operations/terminator"
)

//go:generate swagger generate server --target ../../edge --name ZitiEdgeManagement --spec ../specs/management.yml --model-package rest_model --server-package rest_management_api_server --principal interface{} --exclude-main

func configureFlags(api *operations.ZitiEdgeManagementAPI) {
	// api.CommandLineOptionsGroups = []swag.CommandLineOptionsGroup{ ... }
}

func configureAPI(api *operations.ZitiEdgeManagementAPI) http.Handler {
	// configure the api here
	api.ServeError = errors.ServeError

	// Set your custom logger if needed. Default one is log.Printf
	// Expected interface func(string, ...interface{})
	//
	// Example:
	// api.Logger = log.Printf

	api.UseSwaggerUI()
	// To continue using redoc as your UI, uncomment the following line
	// api.UseRedoc()

	api.JSONConsumer = runtime.JSONConsumer()
	api.TxtConsumer = runtime.TextConsumer()

	api.ApplicationJwtProducer = runtime.ProducerFunc(func(w io.Writer, data interface{}) error {
		return errors.NotImplemented("applicationJwt producer has not yet been implemented")
	})
	api.BinProducer = runtime.ByteStreamProducer()
	api.JSONProducer = runtime.JSONProducer()
	api.TextYamlProducer = runtime.ProducerFunc(func(w io.Writer, data interface{}) error {
		return errors.NotImplemented("textYaml producer has not yet been implemented")
	})

	// Applies when the "zt-session" header is set
	if api.ZtSessionAuth == nil {
		api.ZtSessionAuth = func(token string) (interface{}, error) {
			return nil, errors.NotImplemented("api key auth (ztSession) zt-session from header param [zt-session] has not yet been implemented")
		}
	}

	// Set your custom authorizer if needed. Default one is security.Authorized()
	// Expected interface runtime.Authorizer
	//
	// Example:
	// api.APIAuthorizer = security.Authorized()

	if api.CurrentAPISessionDeleteCurrentAPISessionHandler == nil {
		api.CurrentAPISessionDeleteCurrentAPISessionHandler = current_api_session.DeleteCurrentAPISessionHandlerFunc(func(params current_api_session.DeleteCurrentAPISessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.DeleteCurrentAPISession has not yet been implemented")
		})
	}
	if api.IdentityAssociateIdentitysServiceConfigsHandler == nil {
		api.IdentityAssociateIdentitysServiceConfigsHandler = identity.AssociateIdentitysServiceConfigsHandlerFunc(func(params identity.AssociateIdentitysServiceConfigsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.AssociateIdentitysServiceConfigs has not yet been implemented")
		})
	}
	if api.AuthenticationAuthenticateHandler == nil {
		api.AuthenticationAuthenticateHandler = authentication.AuthenticateHandlerFunc(func(params authentication.AuthenticateParams) middleware.Responder {
			return middleware.NotImplemented("operation authentication.Authenticate has not yet been implemented")
		})
	}
	if api.AuthenticationAuthenticateMfaHandler == nil {
		api.AuthenticationAuthenticateMfaHandler = authentication.AuthenticateMfaHandlerFunc(func(params authentication.AuthenticateMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authentication.AuthenticateMfa has not yet been implemented")
		})
	}
	if api.DatabaseCheckDataIntegrityHandler == nil {
		api.DatabaseCheckDataIntegrityHandler = database.CheckDataIntegrityHandlerFunc(func(params database.CheckDataIntegrityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation database.CheckDataIntegrity has not yet been implemented")
		})
	}
	if api.AuthenticatorCreateAuthenticatorHandler == nil {
		api.AuthenticatorCreateAuthenticatorHandler = authenticator.CreateAuthenticatorHandlerFunc(func(params authenticator.CreateAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authenticator.CreateAuthenticator has not yet been implemented")
		})
	}
	if api.CertificateAuthorityCreateCaHandler == nil {
		api.CertificateAuthorityCreateCaHandler = certificate_authority.CreateCaHandlerFunc(func(params certificate_authority.CreateCaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.CreateCa has not yet been implemented")
		})
	}
	if api.ConfigCreateConfigHandler == nil {
		api.ConfigCreateConfigHandler = config.CreateConfigHandlerFunc(func(params config.CreateConfigParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.CreateConfig has not yet been implemented")
		})
	}
	if api.ConfigCreateConfigTypeHandler == nil {
		api.ConfigCreateConfigTypeHandler = config.CreateConfigTypeHandlerFunc(func(params config.CreateConfigTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.CreateConfigType has not yet been implemented")
		})
	}
	if api.DatabaseCreateDatabaseSnapshotHandler == nil {
		api.DatabaseCreateDatabaseSnapshotHandler = database.CreateDatabaseSnapshotHandlerFunc(func(params database.CreateDatabaseSnapshotParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation database.CreateDatabaseSnapshot has not yet been implemented")
		})
	}
	if api.EdgeRouterCreateEdgeRouterHandler == nil {
		api.EdgeRouterCreateEdgeRouterHandler = edge_router.CreateEdgeRouterHandlerFunc(func(params edge_router.CreateEdgeRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.CreateEdgeRouter has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyCreateEdgeRouterPolicyHandler == nil {
		api.EdgeRouterPolicyCreateEdgeRouterPolicyHandler = edge_router_policy.CreateEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.CreateEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.CreateEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.IdentityCreateIdentityHandler == nil {
		api.IdentityCreateIdentityHandler = identity.CreateIdentityHandlerFunc(func(params identity.CreateIdentityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.CreateIdentity has not yet been implemented")
		})
	}
	if api.CurrentIdentityCreateMfaRecoveryCodesHandler == nil {
		api.CurrentIdentityCreateMfaRecoveryCodesHandler = current_identity.CreateMfaRecoveryCodesHandlerFunc(func(params current_identity.CreateMfaRecoveryCodesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.CreateMfaRecoveryCodes has not yet been implemented")
		})
	}
	if api.PostureChecksCreatePostureCheckHandler == nil {
		api.PostureChecksCreatePostureCheckHandler = posture_checks.CreatePostureCheckHandlerFunc(func(params posture_checks.CreatePostureCheckParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.CreatePostureCheck has not yet been implemented")
		})
	}
	if api.RouterCreateRouterHandler == nil {
		api.RouterCreateRouterHandler = router.CreateRouterHandlerFunc(func(params router.CreateRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.CreateRouter has not yet been implemented")
		})
	}
	if api.ServiceCreateServiceHandler == nil {
		api.ServiceCreateServiceHandler = service.CreateServiceHandlerFunc(func(params service.CreateServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.CreateService has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyCreateServiceEdgeRouterPolicyHandler == nil {
		api.ServiceEdgeRouterPolicyCreateServiceEdgeRouterPolicyHandler = service_edge_router_policy.CreateServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.CreateServiceEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.CreateServiceEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.ServicePolicyCreateServicePolicyHandler == nil {
		api.ServicePolicyCreateServicePolicyHandler = service_policy.CreateServicePolicyHandlerFunc(func(params service_policy.CreateServicePolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.CreateServicePolicy has not yet been implemented")
		})
	}
	if api.TerminatorCreateTerminatorHandler == nil {
		api.TerminatorCreateTerminatorHandler = terminator.CreateTerminatorHandlerFunc(func(params terminator.CreateTerminatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation terminator.CreateTerminator has not yet been implemented")
		})
	}
	if api.RouterCreateTransitRouterHandler == nil {
		api.RouterCreateTransitRouterHandler = router.CreateTransitRouterHandlerFunc(func(params router.CreateTransitRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.CreateTransitRouter has not yet been implemented")
		})
	}
	if api.DatabaseDataIntegrityResultsHandler == nil {
		api.DatabaseDataIntegrityResultsHandler = database.DataIntegrityResultsHandlerFunc(func(params database.DataIntegrityResultsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation database.DataIntegrityResults has not yet been implemented")
		})
	}
	if api.APISessionDeleteAPISessionsHandler == nil {
		api.APISessionDeleteAPISessionsHandler = api_session.DeleteAPISessionsHandlerFunc(func(params api_session.DeleteAPISessionsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation api_session.DeleteAPISessions has not yet been implemented")
		})
	}
	if api.AuthenticatorDeleteAuthenticatorHandler == nil {
		api.AuthenticatorDeleteAuthenticatorHandler = authenticator.DeleteAuthenticatorHandlerFunc(func(params authenticator.DeleteAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authenticator.DeleteAuthenticator has not yet been implemented")
		})
	}
	if api.CertificateAuthorityDeleteCaHandler == nil {
		api.CertificateAuthorityDeleteCaHandler = certificate_authority.DeleteCaHandlerFunc(func(params certificate_authority.DeleteCaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.DeleteCa has not yet been implemented")
		})
	}
	if api.ConfigDeleteConfigHandler == nil {
		api.ConfigDeleteConfigHandler = config.DeleteConfigHandlerFunc(func(params config.DeleteConfigParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.DeleteConfig has not yet been implemented")
		})
	}
	if api.ConfigDeleteConfigTypeHandler == nil {
		api.ConfigDeleteConfigTypeHandler = config.DeleteConfigTypeHandlerFunc(func(params config.DeleteConfigTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.DeleteConfigType has not yet been implemented")
		})
	}
	if api.EdgeRouterDeleteEdgeRouterHandler == nil {
		api.EdgeRouterDeleteEdgeRouterHandler = edge_router.DeleteEdgeRouterHandlerFunc(func(params edge_router.DeleteEdgeRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.DeleteEdgeRouter has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyDeleteEdgeRouterPolicyHandler == nil {
		api.EdgeRouterPolicyDeleteEdgeRouterPolicyHandler = edge_router_policy.DeleteEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.DeleteEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.DeleteEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.EnrollmentDeleteEnrollmentHandler == nil {
		api.EnrollmentDeleteEnrollmentHandler = enrollment.DeleteEnrollmentHandlerFunc(func(params enrollment.DeleteEnrollmentParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation enrollment.DeleteEnrollment has not yet been implemented")
		})
	}
	if api.IdentityDeleteIdentityHandler == nil {
		api.IdentityDeleteIdentityHandler = identity.DeleteIdentityHandlerFunc(func(params identity.DeleteIdentityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.DeleteIdentity has not yet been implemented")
		})
	}
	if api.CurrentIdentityDeleteMfaHandler == nil {
		api.CurrentIdentityDeleteMfaHandler = current_identity.DeleteMfaHandlerFunc(func(params current_identity.DeleteMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.DeleteMfa has not yet been implemented")
		})
	}
	if api.PostureChecksDeletePostureCheckHandler == nil {
		api.PostureChecksDeletePostureCheckHandler = posture_checks.DeletePostureCheckHandlerFunc(func(params posture_checks.DeletePostureCheckParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.DeletePostureCheck has not yet been implemented")
		})
	}
	if api.RouterDeleteRouterHandler == nil {
		api.RouterDeleteRouterHandler = router.DeleteRouterHandlerFunc(func(params router.DeleteRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.DeleteRouter has not yet been implemented")
		})
	}
	if api.ServiceDeleteServiceHandler == nil {
		api.ServiceDeleteServiceHandler = service.DeleteServiceHandlerFunc(func(params service.DeleteServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.DeleteService has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyDeleteServiceEdgeRouterPolicyHandler == nil {
		api.ServiceEdgeRouterPolicyDeleteServiceEdgeRouterPolicyHandler = service_edge_router_policy.DeleteServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.DeleteServiceEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.DeleteServiceEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.ServicePolicyDeleteServicePolicyHandler == nil {
		api.ServicePolicyDeleteServicePolicyHandler = service_policy.DeleteServicePolicyHandlerFunc(func(params service_policy.DeleteServicePolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.DeleteServicePolicy has not yet been implemented")
		})
	}
	if api.SessionDeleteSessionHandler == nil {
		api.SessionDeleteSessionHandler = session.DeleteSessionHandlerFunc(func(params session.DeleteSessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.DeleteSession has not yet been implemented")
		})
	}
	if api.TerminatorDeleteTerminatorHandler == nil {
		api.TerminatorDeleteTerminatorHandler = terminator.DeleteTerminatorHandlerFunc(func(params terminator.DeleteTerminatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation terminator.DeleteTerminator has not yet been implemented")
		})
	}
	if api.RouterDeleteTransitRouterHandler == nil {
		api.RouterDeleteTransitRouterHandler = router.DeleteTransitRouterHandlerFunc(func(params router.DeleteTransitRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.DeleteTransitRouter has not yet been implemented")
		})
	}
	if api.APISessionDetailAPISessionsHandler == nil {
		api.APISessionDetailAPISessionsHandler = api_session.DetailAPISessionsHandlerFunc(func(params api_session.DetailAPISessionsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation api_session.DetailAPISessions has not yet been implemented")
		})
	}
	if api.AuthenticatorDetailAuthenticatorHandler == nil {
		api.AuthenticatorDetailAuthenticatorHandler = authenticator.DetailAuthenticatorHandlerFunc(func(params authenticator.DetailAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authenticator.DetailAuthenticator has not yet been implemented")
		})
	}
	if api.CertificateAuthorityDetailCaHandler == nil {
		api.CertificateAuthorityDetailCaHandler = certificate_authority.DetailCaHandlerFunc(func(params certificate_authority.DetailCaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.DetailCa has not yet been implemented")
		})
	}
	if api.ConfigDetailConfigHandler == nil {
		api.ConfigDetailConfigHandler = config.DetailConfigHandlerFunc(func(params config.DetailConfigParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.DetailConfig has not yet been implemented")
		})
	}
	if api.ConfigDetailConfigTypeHandler == nil {
		api.ConfigDetailConfigTypeHandler = config.DetailConfigTypeHandlerFunc(func(params config.DetailConfigTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.DetailConfigType has not yet been implemented")
		})
	}
	if api.CurrentAPISessionDetailCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionDetailCurrentIdentityAuthenticatorHandler = current_api_session.DetailCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.DetailCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.DetailCurrentIdentityAuthenticator has not yet been implemented")
		})
	}
	if api.EdgeRouterDetailEdgeRouterHandler == nil {
		api.EdgeRouterDetailEdgeRouterHandler = edge_router.DetailEdgeRouterHandlerFunc(func(params edge_router.DetailEdgeRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.DetailEdgeRouter has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyDetailEdgeRouterPolicyHandler == nil {
		api.EdgeRouterPolicyDetailEdgeRouterPolicyHandler = edge_router_policy.DetailEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.DetailEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.DetailEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.EnrollmentDetailEnrollmentHandler == nil {
		api.EnrollmentDetailEnrollmentHandler = enrollment.DetailEnrollmentHandlerFunc(func(params enrollment.DetailEnrollmentParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation enrollment.DetailEnrollment has not yet been implemented")
		})
	}
	if api.IdentityDetailIdentityHandler == nil {
		api.IdentityDetailIdentityHandler = identity.DetailIdentityHandlerFunc(func(params identity.DetailIdentityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.DetailIdentity has not yet been implemented")
		})
	}
	if api.IdentityDetailIdentityTypeHandler == nil {
		api.IdentityDetailIdentityTypeHandler = identity.DetailIdentityTypeHandlerFunc(func(params identity.DetailIdentityTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.DetailIdentityType has not yet been implemented")
		})
	}
	if api.CurrentIdentityDetailMfaHandler == nil {
		api.CurrentIdentityDetailMfaHandler = current_identity.DetailMfaHandlerFunc(func(params current_identity.DetailMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.DetailMfa has not yet been implemented")
		})
	}
	if api.CurrentIdentityDetailMfaQrCodeHandler == nil {
		api.CurrentIdentityDetailMfaQrCodeHandler = current_identity.DetailMfaQrCodeHandlerFunc(func(params current_identity.DetailMfaQrCodeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.DetailMfaQrCode has not yet been implemented")
		})
	}
	if api.CurrentIdentityDetailMfaRecoveryCodesHandler == nil {
		api.CurrentIdentityDetailMfaRecoveryCodesHandler = current_identity.DetailMfaRecoveryCodesHandlerFunc(func(params current_identity.DetailMfaRecoveryCodesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.DetailMfaRecoveryCodes has not yet been implemented")
		})
	}
	if api.PostureChecksDetailPostureCheckHandler == nil {
		api.PostureChecksDetailPostureCheckHandler = posture_checks.DetailPostureCheckHandlerFunc(func(params posture_checks.DetailPostureCheckParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.DetailPostureCheck has not yet been implemented")
		})
	}
	if api.PostureChecksDetailPostureCheckTypeHandler == nil {
		api.PostureChecksDetailPostureCheckTypeHandler = posture_checks.DetailPostureCheckTypeHandlerFunc(func(params posture_checks.DetailPostureCheckTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.DetailPostureCheckType has not yet been implemented")
		})
	}
	if api.RouterDetailRouterHandler == nil {
		api.RouterDetailRouterHandler = router.DetailRouterHandlerFunc(func(params router.DetailRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.DetailRouter has not yet been implemented")
		})
	}
	if api.ServiceDetailServiceHandler == nil {
		api.ServiceDetailServiceHandler = service.DetailServiceHandlerFunc(func(params service.DetailServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.DetailService has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyDetailServiceEdgeRouterPolicyHandler == nil {
		api.ServiceEdgeRouterPolicyDetailServiceEdgeRouterPolicyHandler = service_edge_router_policy.DetailServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.DetailServiceEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.DetailServiceEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.ServicePolicyDetailServicePolicyHandler == nil {
		api.ServicePolicyDetailServicePolicyHandler = service_policy.DetailServicePolicyHandlerFunc(func(params service_policy.DetailServicePolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.DetailServicePolicy has not yet been implemented")
		})
	}
	if api.SessionDetailSessionHandler == nil {
		api.SessionDetailSessionHandler = session.DetailSessionHandlerFunc(func(params session.DetailSessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.DetailSession has not yet been implemented")
		})
	}
	if api.SessionDetailSessionRoutePathHandler == nil {
		api.SessionDetailSessionRoutePathHandler = session.DetailSessionRoutePathHandlerFunc(func(params session.DetailSessionRoutePathParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.DetailSessionRoutePath has not yet been implemented")
		})
	}
	if api.InformationalDetailSpecHandler == nil {
		api.InformationalDetailSpecHandler = informational.DetailSpecHandlerFunc(func(params informational.DetailSpecParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.DetailSpec has not yet been implemented")
		})
	}
	if api.InformationalDetailSpecBodyHandler == nil {
		api.InformationalDetailSpecBodyHandler = informational.DetailSpecBodyHandlerFunc(func(params informational.DetailSpecBodyParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.DetailSpecBody has not yet been implemented")
		})
	}
	if api.TerminatorDetailTerminatorHandler == nil {
		api.TerminatorDetailTerminatorHandler = terminator.DetailTerminatorHandlerFunc(func(params terminator.DetailTerminatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation terminator.DetailTerminator has not yet been implemented")
		})
	}
	if api.RouterDetailTransitRouterHandler == nil {
		api.RouterDetailTransitRouterHandler = router.DetailTransitRouterHandlerFunc(func(params router.DetailTransitRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.DetailTransitRouter has not yet been implemented")
		})
	}
	if api.IdentityDisassociateIdentitysServiceConfigsHandler == nil {
		api.IdentityDisassociateIdentitysServiceConfigsHandler = identity.DisassociateIdentitysServiceConfigsHandlerFunc(func(params identity.DisassociateIdentitysServiceConfigsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.DisassociateIdentitysServiceConfigs has not yet been implemented")
		})
	}
	if api.CurrentIdentityEnrollMfaHandler == nil {
		api.CurrentIdentityEnrollMfaHandler = current_identity.EnrollMfaHandlerFunc(func(params current_identity.EnrollMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.EnrollMfa has not yet been implemented")
		})
	}
	if api.DatabaseFixDataIntegrityHandler == nil {
		api.DatabaseFixDataIntegrityHandler = database.FixDataIntegrityHandlerFunc(func(params database.FixDataIntegrityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation database.FixDataIntegrity has not yet been implemented")
		})
	}
	if api.CertificateAuthorityGetCaJwtHandler == nil {
		api.CertificateAuthorityGetCaJwtHandler = certificate_authority.GetCaJwtHandlerFunc(func(params certificate_authority.GetCaJwtParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.GetCaJwt has not yet been implemented")
		})
	}
	if api.CurrentAPISessionGetCurrentAPISessionHandler == nil {
		api.CurrentAPISessionGetCurrentAPISessionHandler = current_api_session.GetCurrentAPISessionHandlerFunc(func(params current_api_session.GetCurrentAPISessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.GetCurrentAPISession has not yet been implemented")
		})
	}
	if api.CurrentIdentityGetCurrentIdentityHandler == nil {
		api.CurrentIdentityGetCurrentIdentityHandler = current_identity.GetCurrentIdentityHandlerFunc(func(params current_identity.GetCurrentIdentityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.GetCurrentIdentity has not yet been implemented")
		})
	}
	if api.IdentityGetIdentityAuthenticatorsHandler == nil {
		api.IdentityGetIdentityAuthenticatorsHandler = identity.GetIdentityAuthenticatorsHandlerFunc(func(params identity.GetIdentityAuthenticatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.GetIdentityAuthenticators has not yet been implemented")
		})
	}
	if api.IdentityGetIdentityFailedServiceRequestsHandler == nil {
		api.IdentityGetIdentityFailedServiceRequestsHandler = identity.GetIdentityFailedServiceRequestsHandlerFunc(func(params identity.GetIdentityFailedServiceRequestsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.GetIdentityFailedServiceRequests has not yet been implemented")
		})
	}
	if api.IdentityGetIdentityPolicyAdviceHandler == nil {
		api.IdentityGetIdentityPolicyAdviceHandler = identity.GetIdentityPolicyAdviceHandlerFunc(func(params identity.GetIdentityPolicyAdviceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.GetIdentityPolicyAdvice has not yet been implemented")
		})
	}
	if api.IdentityGetIdentityPostureDataHandler == nil {
		api.IdentityGetIdentityPostureDataHandler = identity.GetIdentityPostureDataHandlerFunc(func(params identity.GetIdentityPostureDataParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.GetIdentityPostureData has not yet been implemented")
		})
	}
	if api.APISessionListAPISessionsHandler == nil {
		api.APISessionListAPISessionsHandler = api_session.ListAPISessionsHandlerFunc(func(params api_session.ListAPISessionsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation api_session.ListAPISessions has not yet been implemented")
		})
	}
	if api.AuthenticatorListAuthenticatorsHandler == nil {
		api.AuthenticatorListAuthenticatorsHandler = authenticator.ListAuthenticatorsHandlerFunc(func(params authenticator.ListAuthenticatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authenticator.ListAuthenticators has not yet been implemented")
		})
	}
	if api.CertificateAuthorityListCasHandler == nil {
		api.CertificateAuthorityListCasHandler = certificate_authority.ListCasHandlerFunc(func(params certificate_authority.ListCasParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.ListCas has not yet been implemented")
		})
	}
	if api.ConfigListConfigTypesHandler == nil {
		api.ConfigListConfigTypesHandler = config.ListConfigTypesHandlerFunc(func(params config.ListConfigTypesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.ListConfigTypes has not yet been implemented")
		})
	}
	if api.ConfigListConfigsHandler == nil {
		api.ConfigListConfigsHandler = config.ListConfigsHandlerFunc(func(params config.ListConfigsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.ListConfigs has not yet been implemented")
		})
	}
	if api.ConfigListConfigsForConfigTypeHandler == nil {
		api.ConfigListConfigsForConfigTypeHandler = config.ListConfigsForConfigTypeHandlerFunc(func(params config.ListConfigsForConfigTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.ListConfigsForConfigType has not yet been implemented")
		})
	}
	if api.CurrentAPISessionListCurrentIdentityAuthenticatorsHandler == nil {
		api.CurrentAPISessionListCurrentIdentityAuthenticatorsHandler = current_api_session.ListCurrentIdentityAuthenticatorsHandlerFunc(func(params current_api_session.ListCurrentIdentityAuthenticatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.ListCurrentIdentityAuthenticators has not yet been implemented")
		})
	}
	if api.EdgeRouterListEdgeRouterEdgeRouterPoliciesHandler == nil {
		api.EdgeRouterListEdgeRouterEdgeRouterPoliciesHandler = edge_router.ListEdgeRouterEdgeRouterPoliciesHandlerFunc(func(params edge_router.ListEdgeRouterEdgeRouterPoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.ListEdgeRouterEdgeRouterPolicies has not yet been implemented")
		})
	}
	if api.EdgeRouterListEdgeRouterIdentitiesHandler == nil {
		api.EdgeRouterListEdgeRouterIdentitiesHandler = edge_router.ListEdgeRouterIdentitiesHandlerFunc(func(params edge_router.ListEdgeRouterIdentitiesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.ListEdgeRouterIdentities has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyListEdgeRouterPoliciesHandler == nil {
		api.EdgeRouterPolicyListEdgeRouterPoliciesHandler = edge_router_policy.ListEdgeRouterPoliciesHandlerFunc(func(params edge_router_policy.ListEdgeRouterPoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.ListEdgeRouterPolicies has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyListEdgeRouterPolicyEdgeRoutersHandler == nil {
		api.EdgeRouterPolicyListEdgeRouterPolicyEdgeRoutersHandler = edge_router_policy.ListEdgeRouterPolicyEdgeRoutersHandlerFunc(func(params edge_router_policy.ListEdgeRouterPolicyEdgeRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.ListEdgeRouterPolicyEdgeRouters has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyListEdgeRouterPolicyIdentitiesHandler == nil {
		api.EdgeRouterPolicyListEdgeRouterPolicyIdentitiesHandler = edge_router_policy.ListEdgeRouterPolicyIdentitiesHandlerFunc(func(params edge_router_policy.ListEdgeRouterPolicyIdentitiesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.ListEdgeRouterPolicyIdentities has not yet been implemented")
		})
	}
	if api.RoleAttributesListEdgeRouterRoleAttributesHandler == nil {
		api.RoleAttributesListEdgeRouterRoleAttributesHandler = role_attributes.ListEdgeRouterRoleAttributesHandlerFunc(func(params role_attributes.ListEdgeRouterRoleAttributesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation role_attributes.ListEdgeRouterRoleAttributes has not yet been implemented")
		})
	}
	if api.EdgeRouterListEdgeRouterServiceEdgeRouterPoliciesHandler == nil {
		api.EdgeRouterListEdgeRouterServiceEdgeRouterPoliciesHandler = edge_router.ListEdgeRouterServiceEdgeRouterPoliciesHandlerFunc(func(params edge_router.ListEdgeRouterServiceEdgeRouterPoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.ListEdgeRouterServiceEdgeRouterPolicies has not yet been implemented")
		})
	}
	if api.EdgeRouterListEdgeRouterServicesHandler == nil {
		api.EdgeRouterListEdgeRouterServicesHandler = edge_router.ListEdgeRouterServicesHandlerFunc(func(params edge_router.ListEdgeRouterServicesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.ListEdgeRouterServices has not yet been implemented")
		})
	}
	if api.EdgeRouterListEdgeRoutersHandler == nil {
		api.EdgeRouterListEdgeRoutersHandler = edge_router.ListEdgeRoutersHandlerFunc(func(params edge_router.ListEdgeRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.ListEdgeRouters has not yet been implemented")
		})
	}
	if api.EnrollmentListEnrollmentsHandler == nil {
		api.EnrollmentListEnrollmentsHandler = enrollment.ListEnrollmentsHandlerFunc(func(params enrollment.ListEnrollmentsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation enrollment.ListEnrollments has not yet been implemented")
		})
	}
	if api.IdentityListIdentitiesHandler == nil {
		api.IdentityListIdentitiesHandler = identity.ListIdentitiesHandlerFunc(func(params identity.ListIdentitiesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentities has not yet been implemented")
		})
	}
	if api.IdentityListIdentityEdgeRoutersHandler == nil {
		api.IdentityListIdentityEdgeRoutersHandler = identity.ListIdentityEdgeRoutersHandlerFunc(func(params identity.ListIdentityEdgeRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentityEdgeRouters has not yet been implemented")
		})
	}
	if api.RoleAttributesListIdentityRoleAttributesHandler == nil {
		api.RoleAttributesListIdentityRoleAttributesHandler = role_attributes.ListIdentityRoleAttributesHandlerFunc(func(params role_attributes.ListIdentityRoleAttributesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation role_attributes.ListIdentityRoleAttributes has not yet been implemented")
		})
	}
	if api.IdentityListIdentityServicePoliciesHandler == nil {
		api.IdentityListIdentityServicePoliciesHandler = identity.ListIdentityServicePoliciesHandlerFunc(func(params identity.ListIdentityServicePoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentityServicePolicies has not yet been implemented")
		})
	}
	if api.IdentityListIdentityServicesHandler == nil {
		api.IdentityListIdentityServicesHandler = identity.ListIdentityServicesHandlerFunc(func(params identity.ListIdentityServicesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentityServices has not yet been implemented")
		})
	}
	if api.IdentityListIdentityTypesHandler == nil {
		api.IdentityListIdentityTypesHandler = identity.ListIdentityTypesHandlerFunc(func(params identity.ListIdentityTypesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentityTypes has not yet been implemented")
		})
	}
	if api.IdentityListIdentitysEdgeRouterPoliciesHandler == nil {
		api.IdentityListIdentitysEdgeRouterPoliciesHandler = identity.ListIdentitysEdgeRouterPoliciesHandlerFunc(func(params identity.ListIdentitysEdgeRouterPoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentitysEdgeRouterPolicies has not yet been implemented")
		})
	}
	if api.IdentityListIdentitysServiceConfigsHandler == nil {
		api.IdentityListIdentitysServiceConfigsHandler = identity.ListIdentitysServiceConfigsHandlerFunc(func(params identity.ListIdentitysServiceConfigsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.ListIdentitysServiceConfigs has not yet been implemented")
		})
	}
	if api.PostureChecksListPostureCheckTypesHandler == nil {
		api.PostureChecksListPostureCheckTypesHandler = posture_checks.ListPostureCheckTypesHandlerFunc(func(params posture_checks.ListPostureCheckTypesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.ListPostureCheckTypes has not yet been implemented")
		})
	}
	if api.PostureChecksListPostureChecksHandler == nil {
		api.PostureChecksListPostureChecksHandler = posture_checks.ListPostureChecksHandlerFunc(func(params posture_checks.ListPostureChecksParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.ListPostureChecks has not yet been implemented")
		})
	}
	if api.InformationalListRootHandler == nil {
		api.InformationalListRootHandler = informational.ListRootHandlerFunc(func(params informational.ListRootParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListRoot has not yet been implemented")
		})
	}
	if api.RouterListRoutersHandler == nil {
		api.RouterListRoutersHandler = router.ListRoutersHandlerFunc(func(params router.ListRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.ListRouters has not yet been implemented")
		})
	}
	if api.ServiceListServiceConfigHandler == nil {
		api.ServiceListServiceConfigHandler = service.ListServiceConfigHandlerFunc(func(params service.ListServiceConfigParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceConfig has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyListServiceEdgeRouterPoliciesHandler == nil {
		api.ServiceEdgeRouterPolicyListServiceEdgeRouterPoliciesHandler = service_edge_router_policy.ListServiceEdgeRouterPoliciesHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.ListServiceEdgeRouterPolicies has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyEdgeRoutersHandler == nil {
		api.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyEdgeRoutersHandler = service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRoutersHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.ListServiceEdgeRouterPolicyEdgeRouters has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyServicesHandler == nil {
		api.ServiceEdgeRouterPolicyListServiceEdgeRouterPolicyServicesHandler = service_edge_router_policy.ListServiceEdgeRouterPolicyServicesHandlerFunc(func(params service_edge_router_policy.ListServiceEdgeRouterPolicyServicesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.ListServiceEdgeRouterPolicyServices has not yet been implemented")
		})
	}
	if api.ServiceListServiceEdgeRoutersHandler == nil {
		api.ServiceListServiceEdgeRoutersHandler = service.ListServiceEdgeRoutersHandlerFunc(func(params service.ListServiceEdgeRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceEdgeRouters has not yet been implemented")
		})
	}
	if api.ServiceListServiceIdentitiesHandler == nil {
		api.ServiceListServiceIdentitiesHandler = service.ListServiceIdentitiesHandlerFunc(func(params service.ListServiceIdentitiesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceIdentities has not yet been implemented")
		})
	}
	if api.ServicePolicyListServicePoliciesHandler == nil {
		api.ServicePolicyListServicePoliciesHandler = service_policy.ListServicePoliciesHandlerFunc(func(params service_policy.ListServicePoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.ListServicePolicies has not yet been implemented")
		})
	}
	if api.ServicePolicyListServicePolicyIdentitiesHandler == nil {
		api.ServicePolicyListServicePolicyIdentitiesHandler = service_policy.ListServicePolicyIdentitiesHandlerFunc(func(params service_policy.ListServicePolicyIdentitiesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.ListServicePolicyIdentities has not yet been implemented")
		})
	}
	if api.ServicePolicyListServicePolicyPostureChecksHandler == nil {
		api.ServicePolicyListServicePolicyPostureChecksHandler = service_policy.ListServicePolicyPostureChecksHandlerFunc(func(params service_policy.ListServicePolicyPostureChecksParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.ListServicePolicyPostureChecks has not yet been implemented")
		})
	}
	if api.ServicePolicyListServicePolicyServicesHandler == nil {
		api.ServicePolicyListServicePolicyServicesHandler = service_policy.ListServicePolicyServicesHandlerFunc(func(params service_policy.ListServicePolicyServicesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.ListServicePolicyServices has not yet been implemented")
		})
	}
	if api.RoleAttributesListServiceRoleAttributesHandler == nil {
		api.RoleAttributesListServiceRoleAttributesHandler = role_attributes.ListServiceRoleAttributesHandlerFunc(func(params role_attributes.ListServiceRoleAttributesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation role_attributes.ListServiceRoleAttributes has not yet been implemented")
		})
	}
	if api.ServiceListServiceServiceEdgeRouterPoliciesHandler == nil {
		api.ServiceListServiceServiceEdgeRouterPoliciesHandler = service.ListServiceServiceEdgeRouterPoliciesHandlerFunc(func(params service.ListServiceServiceEdgeRouterPoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceServiceEdgeRouterPolicies has not yet been implemented")
		})
	}
	if api.ServiceListServiceServicePoliciesHandler == nil {
		api.ServiceListServiceServicePoliciesHandler = service.ListServiceServicePoliciesHandlerFunc(func(params service.ListServiceServicePoliciesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceServicePolicies has not yet been implemented")
		})
	}
	if api.ServiceListServiceTerminatorsHandler == nil {
		api.ServiceListServiceTerminatorsHandler = service.ListServiceTerminatorsHandlerFunc(func(params service.ListServiceTerminatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceTerminators has not yet been implemented")
		})
	}
	if api.ServiceListServicesHandler == nil {
		api.ServiceListServicesHandler = service.ListServicesHandlerFunc(func(params service.ListServicesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServices has not yet been implemented")
		})
	}
	if api.SessionListSessionsHandler == nil {
		api.SessionListSessionsHandler = session.ListSessionsHandlerFunc(func(params session.ListSessionsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.ListSessions has not yet been implemented")
		})
	}
	if api.InformationalListSpecsHandler == nil {
		api.InformationalListSpecsHandler = informational.ListSpecsHandlerFunc(func(params informational.ListSpecsParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListSpecs has not yet been implemented")
		})
	}
	if api.InformationalListSummaryHandler == nil {
		api.InformationalListSummaryHandler = informational.ListSummaryHandlerFunc(func(params informational.ListSummaryParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListSummary has not yet been implemented")
		})
	}
	if api.TerminatorListTerminatorsHandler == nil {
		api.TerminatorListTerminatorsHandler = terminator.ListTerminatorsHandlerFunc(func(params terminator.ListTerminatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation terminator.ListTerminators has not yet been implemented")
		})
	}
	if api.RouterListTransitRoutersHandler == nil {
		api.RouterListTransitRoutersHandler = router.ListTransitRoutersHandlerFunc(func(params router.ListTransitRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.ListTransitRouters has not yet been implemented")
		})
	}
	if api.InformationalListVersionHandler == nil {
		api.InformationalListVersionHandler = informational.ListVersionHandlerFunc(func(params informational.ListVersionParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListVersion has not yet been implemented")
		})
	}
	if api.AuthenticatorPatchAuthenticatorHandler == nil {
		api.AuthenticatorPatchAuthenticatorHandler = authenticator.PatchAuthenticatorHandlerFunc(func(params authenticator.PatchAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authenticator.PatchAuthenticator has not yet been implemented")
		})
	}
	if api.CertificateAuthorityPatchCaHandler == nil {
		api.CertificateAuthorityPatchCaHandler = certificate_authority.PatchCaHandlerFunc(func(params certificate_authority.PatchCaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.PatchCa has not yet been implemented")
		})
	}
	if api.ConfigPatchConfigHandler == nil {
		api.ConfigPatchConfigHandler = config.PatchConfigHandlerFunc(func(params config.PatchConfigParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.PatchConfig has not yet been implemented")
		})
	}
	if api.ConfigPatchConfigTypeHandler == nil {
		api.ConfigPatchConfigTypeHandler = config.PatchConfigTypeHandlerFunc(func(params config.PatchConfigTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.PatchConfigType has not yet been implemented")
		})
	}
	if api.CurrentAPISessionPatchCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionPatchCurrentIdentityAuthenticatorHandler = current_api_session.PatchCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.PatchCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.PatchCurrentIdentityAuthenticator has not yet been implemented")
		})
	}
	if api.EdgeRouterPatchEdgeRouterHandler == nil {
		api.EdgeRouterPatchEdgeRouterHandler = edge_router.PatchEdgeRouterHandlerFunc(func(params edge_router.PatchEdgeRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.PatchEdgeRouter has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyPatchEdgeRouterPolicyHandler == nil {
		api.EdgeRouterPolicyPatchEdgeRouterPolicyHandler = edge_router_policy.PatchEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.PatchEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.PatchEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.IdentityPatchIdentityHandler == nil {
		api.IdentityPatchIdentityHandler = identity.PatchIdentityHandlerFunc(func(params identity.PatchIdentityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.PatchIdentity has not yet been implemented")
		})
	}
	if api.PostureChecksPatchPostureCheckHandler == nil {
		api.PostureChecksPatchPostureCheckHandler = posture_checks.PatchPostureCheckHandlerFunc(func(params posture_checks.PatchPostureCheckParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.PatchPostureCheck has not yet been implemented")
		})
	}
	if api.RouterPatchRouterHandler == nil {
		api.RouterPatchRouterHandler = router.PatchRouterHandlerFunc(func(params router.PatchRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.PatchRouter has not yet been implemented")
		})
	}
	if api.ServicePatchServiceHandler == nil {
		api.ServicePatchServiceHandler = service.PatchServiceHandlerFunc(func(params service.PatchServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.PatchService has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyPatchServiceEdgeRouterPolicyHandler == nil {
		api.ServiceEdgeRouterPolicyPatchServiceEdgeRouterPolicyHandler = service_edge_router_policy.PatchServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.PatchServiceEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.PatchServiceEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.ServicePolicyPatchServicePolicyHandler == nil {
		api.ServicePolicyPatchServicePolicyHandler = service_policy.PatchServicePolicyHandlerFunc(func(params service_policy.PatchServicePolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.PatchServicePolicy has not yet been implemented")
		})
	}
	if api.TerminatorPatchTerminatorHandler == nil {
		api.TerminatorPatchTerminatorHandler = terminator.PatchTerminatorHandlerFunc(func(params terminator.PatchTerminatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation terminator.PatchTerminator has not yet been implemented")
		})
	}
	if api.RouterPatchTransitRouterHandler == nil {
		api.RouterPatchTransitRouterHandler = router.PatchTransitRouterHandlerFunc(func(params router.PatchTransitRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.PatchTransitRouter has not yet been implemented")
		})
	}
	if api.IdentityRemoveIdentityMfaHandler == nil {
		api.IdentityRemoveIdentityMfaHandler = identity.RemoveIdentityMfaHandlerFunc(func(params identity.RemoveIdentityMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.RemoveIdentityMfa has not yet been implemented")
		})
	}
	if api.AuthenticatorUpdateAuthenticatorHandler == nil {
		api.AuthenticatorUpdateAuthenticatorHandler = authenticator.UpdateAuthenticatorHandlerFunc(func(params authenticator.UpdateAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation authenticator.UpdateAuthenticator has not yet been implemented")
		})
	}
	if api.CertificateAuthorityUpdateCaHandler == nil {
		api.CertificateAuthorityUpdateCaHandler = certificate_authority.UpdateCaHandlerFunc(func(params certificate_authority.UpdateCaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.UpdateCa has not yet been implemented")
		})
	}
	if api.ConfigUpdateConfigHandler == nil {
		api.ConfigUpdateConfigHandler = config.UpdateConfigHandlerFunc(func(params config.UpdateConfigParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.UpdateConfig has not yet been implemented")
		})
	}
	if api.ConfigUpdateConfigTypeHandler == nil {
		api.ConfigUpdateConfigTypeHandler = config.UpdateConfigTypeHandlerFunc(func(params config.UpdateConfigTypeParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation config.UpdateConfigType has not yet been implemented")
		})
	}
	if api.CurrentAPISessionUpdateCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionUpdateCurrentIdentityAuthenticatorHandler = current_api_session.UpdateCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.UpdateCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.UpdateCurrentIdentityAuthenticator has not yet been implemented")
		})
	}
	if api.EdgeRouterUpdateEdgeRouterHandler == nil {
		api.EdgeRouterUpdateEdgeRouterHandler = edge_router.UpdateEdgeRouterHandlerFunc(func(params edge_router.UpdateEdgeRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router.UpdateEdgeRouter has not yet been implemented")
		})
	}
	if api.EdgeRouterPolicyUpdateEdgeRouterPolicyHandler == nil {
		api.EdgeRouterPolicyUpdateEdgeRouterPolicyHandler = edge_router_policy.UpdateEdgeRouterPolicyHandlerFunc(func(params edge_router_policy.UpdateEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation edge_router_policy.UpdateEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.IdentityUpdateIdentityHandler == nil {
		api.IdentityUpdateIdentityHandler = identity.UpdateIdentityHandlerFunc(func(params identity.UpdateIdentityParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.UpdateIdentity has not yet been implemented")
		})
	}
	if api.IdentityUpdateIdentityTracingHandler == nil {
		api.IdentityUpdateIdentityTracingHandler = identity.UpdateIdentityTracingHandlerFunc(func(params identity.UpdateIdentityTracingParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation identity.UpdateIdentityTracing has not yet been implemented")
		})
	}
	if api.PostureChecksUpdatePostureCheckHandler == nil {
		api.PostureChecksUpdatePostureCheckHandler = posture_checks.UpdatePostureCheckHandlerFunc(func(params posture_checks.UpdatePostureCheckParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.UpdatePostureCheck has not yet been implemented")
		})
	}
	if api.RouterUpdateRouterHandler == nil {
		api.RouterUpdateRouterHandler = router.UpdateRouterHandlerFunc(func(params router.UpdateRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.UpdateRouter has not yet been implemented")
		})
	}
	if api.ServiceUpdateServiceHandler == nil {
		api.ServiceUpdateServiceHandler = service.UpdateServiceHandlerFunc(func(params service.UpdateServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.UpdateService has not yet been implemented")
		})
	}
	if api.ServiceEdgeRouterPolicyUpdateServiceEdgeRouterPolicyHandler == nil {
		api.ServiceEdgeRouterPolicyUpdateServiceEdgeRouterPolicyHandler = service_edge_router_policy.UpdateServiceEdgeRouterPolicyHandlerFunc(func(params service_edge_router_policy.UpdateServiceEdgeRouterPolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_edge_router_policy.UpdateServiceEdgeRouterPolicy has not yet been implemented")
		})
	}
	if api.ServicePolicyUpdateServicePolicyHandler == nil {
		api.ServicePolicyUpdateServicePolicyHandler = service_policy.UpdateServicePolicyHandlerFunc(func(params service_policy.UpdateServicePolicyParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service_policy.UpdateServicePolicy has not yet been implemented")
		})
	}
	if api.TerminatorUpdateTerminatorHandler == nil {
		api.TerminatorUpdateTerminatorHandler = terminator.UpdateTerminatorHandlerFunc(func(params terminator.UpdateTerminatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation terminator.UpdateTerminator has not yet been implemented")
		})
	}
	if api.RouterUpdateTransitRouterHandler == nil {
		api.RouterUpdateTransitRouterHandler = router.UpdateTransitRouterHandlerFunc(func(params router.UpdateTransitRouterParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation router.UpdateTransitRouter has not yet been implemented")
		})
	}
	if api.CertificateAuthorityVerifyCaHandler == nil {
		api.CertificateAuthorityVerifyCaHandler = certificate_authority.VerifyCaHandlerFunc(func(params certificate_authority.VerifyCaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation certificate_authority.VerifyCa has not yet been implemented")
		})
	}
	if api.CurrentIdentityVerifyMfaHandler == nil {
		api.CurrentIdentityVerifyMfaHandler = current_identity.VerifyMfaHandlerFunc(func(params current_identity.VerifyMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.VerifyMfa has not yet been implemented")
		})
	}

	api.PreServerShutdown = func() {}

	api.ServerShutdown = func() {}

	return setupGlobalMiddleware(api.Serve(setupMiddlewares))
}

// The TLS configuration before HTTPS server starts.
func configureTLS(tlsConfig *tls.Config) {
	// Make all necessary changes to the TLS configuration here.
}

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// scheme value will be set accordingly: "http", "https" or "unix".
func configureServer(s *http.Server, scheme, addr string) {
}

// The middleware configuration is for the handler executors. These do not apply to the swagger.json document.
// The middleware executes after routing but before authentication, binding and validation.
func setupMiddlewares(handler http.Handler) http.Handler {
	return handler
}

// The middleware configuration happens before anything, this middleware also applies to serving the swagger.json document.
// So this is a good place to plug in a panic handling middleware, logging and metrics.
func setupGlobalMiddleware(handler http.Handler) http.Handler {
	return handler
}
