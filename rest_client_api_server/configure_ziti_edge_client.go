// This file is safe to edit. Once it exists it will not be overwritten

//
// Copyright NetFoundry Inc.
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

package rest_client_api_server

import (
	"crypto/tls"
	"io"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/openziti/edge/rest_client_api_server/operations"
	"github.com/openziti/edge/rest_client_api_server/operations/authentication"
	"github.com/openziti/edge/rest_client_api_server/operations/current_api_session"
	"github.com/openziti/edge/rest_client_api_server/operations/current_identity"
	"github.com/openziti/edge/rest_client_api_server/operations/enroll"
	"github.com/openziti/edge/rest_client_api_server/operations/external_jwt_signer"
	"github.com/openziti/edge/rest_client_api_server/operations/informational"
	"github.com/openziti/edge/rest_client_api_server/operations/posture_checks"
	"github.com/openziti/edge/rest_client_api_server/operations/service"
	"github.com/openziti/edge/rest_client_api_server/operations/session"
	"github.com/openziti/edge/rest_client_api_server/operations/well_known"
)

//go:generate swagger generate server --target ../../edge --name ZitiEdgeClient --spec ../specs/client.yml --model-package rest_model --server-package rest_client_api_server --principal interface{} --exclude-main

func configureFlags(api *operations.ZitiEdgeClientAPI) {
	// api.CommandLineOptionsGroups = []swag.CommandLineOptionsGroup{ ... }
}

func configureAPI(api *operations.ZitiEdgeClientAPI) http.Handler {
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

	api.ApplicationPkcs10Consumer = runtime.ConsumerFunc(func(r io.Reader, target interface{}) error {
		return errors.NotImplemented("applicationPkcs10 consumer has not yet been implemented")
	})
	api.ApplicationXPemFileConsumer = runtime.ConsumerFunc(func(r io.Reader, target interface{}) error {
		return errors.NotImplemented("applicationXPemFile consumer has not yet been implemented")
	})
	api.JSONConsumer = runtime.JSONConsumer()
	api.TxtConsumer = runtime.TextConsumer()

	api.ApplicationPkcs7MimeProducer = runtime.ProducerFunc(func(w io.Writer, data interface{}) error {
		return errors.NotImplemented("applicationPkcs7Mime producer has not yet been implemented")
	})
	api.ApplicationXPemFileProducer = runtime.ProducerFunc(func(w io.Writer, data interface{}) error {
		return errors.NotImplemented("applicationXPemFile producer has not yet been implemented")
	})
	api.ApplicationXX509UserCertProducer = runtime.ProducerFunc(func(w io.Writer, data interface{}) error {
		return errors.NotImplemented("applicationXX509UserCert producer has not yet been implemented")
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
	if api.CurrentAPISessionCreateCurrentAPISessionCertificateHandler == nil {
		api.CurrentAPISessionCreateCurrentAPISessionCertificateHandler = current_api_session.CreateCurrentAPISessionCertificateHandlerFunc(func(params current_api_session.CreateCurrentAPISessionCertificateParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.CreateCurrentAPISessionCertificate has not yet been implemented")
		})
	}
	if api.CurrentIdentityCreateMfaRecoveryCodesHandler == nil {
		api.CurrentIdentityCreateMfaRecoveryCodesHandler = current_identity.CreateMfaRecoveryCodesHandlerFunc(func(params current_identity.CreateMfaRecoveryCodesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.CreateMfaRecoveryCodes has not yet been implemented")
		})
	}
	if api.PostureChecksCreatePostureResponseHandler == nil {
		api.PostureChecksCreatePostureResponseHandler = posture_checks.CreatePostureResponseHandlerFunc(func(params posture_checks.CreatePostureResponseParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.CreatePostureResponse has not yet been implemented")
		})
	}
	if api.PostureChecksCreatePostureResponseBulkHandler == nil {
		api.PostureChecksCreatePostureResponseBulkHandler = posture_checks.CreatePostureResponseBulkHandlerFunc(func(params posture_checks.CreatePostureResponseBulkParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation posture_checks.CreatePostureResponseBulk has not yet been implemented")
		})
	}
	if api.SessionCreateSessionHandler == nil {
		api.SessionCreateSessionHandler = session.CreateSessionHandlerFunc(func(params session.CreateSessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.CreateSession has not yet been implemented")
		})
	}
	if api.CurrentAPISessionDeleteCurrentAPISessionCertificateHandler == nil {
		api.CurrentAPISessionDeleteCurrentAPISessionCertificateHandler = current_api_session.DeleteCurrentAPISessionCertificateHandlerFunc(func(params current_api_session.DeleteCurrentAPISessionCertificateParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.DeleteCurrentAPISessionCertificate has not yet been implemented")
		})
	}
	if api.CurrentIdentityDeleteMfaHandler == nil {
		api.CurrentIdentityDeleteMfaHandler = current_identity.DeleteMfaHandlerFunc(func(params current_identity.DeleteMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.DeleteMfa has not yet been implemented")
		})
	}
	if api.ServiceDeleteServiceHandler == nil {
		api.ServiceDeleteServiceHandler = service.DeleteServiceHandlerFunc(func(params service.DeleteServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.DeleteService has not yet been implemented")
		})
	}
	if api.SessionDeleteSessionHandler == nil {
		api.SessionDeleteSessionHandler = session.DeleteSessionHandlerFunc(func(params session.DeleteSessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.DeleteSession has not yet been implemented")
		})
	}
	if api.CurrentAPISessionDetailCurrentAPISessionCertificateHandler == nil {
		api.CurrentAPISessionDetailCurrentAPISessionCertificateHandler = current_api_session.DetailCurrentAPISessionCertificateHandlerFunc(func(params current_api_session.DetailCurrentAPISessionCertificateParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.DetailCurrentAPISessionCertificate has not yet been implemented")
		})
	}
	if api.CurrentAPISessionDetailCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionDetailCurrentIdentityAuthenticatorHandler = current_api_session.DetailCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.DetailCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.DetailCurrentIdentityAuthenticator has not yet been implemented")
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
	if api.ServiceDetailServiceHandler == nil {
		api.ServiceDetailServiceHandler = service.DetailServiceHandlerFunc(func(params service.DetailServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.DetailService has not yet been implemented")
		})
	}
	if api.SessionDetailSessionHandler == nil {
		api.SessionDetailSessionHandler = session.DetailSessionHandlerFunc(func(params session.DetailSessionParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation session.DetailSession has not yet been implemented")
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
	if api.EnrollEnrollHandler == nil {
		api.EnrollEnrollHandler = enroll.EnrollHandlerFunc(func(params enroll.EnrollParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.Enroll has not yet been implemented")
		})
	}
	if api.EnrollEnrollCaHandler == nil {
		api.EnrollEnrollCaHandler = enroll.EnrollCaHandlerFunc(func(params enroll.EnrollCaParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.EnrollCa has not yet been implemented")
		})
	}
	if api.EnrollEnrollErOttHandler == nil {
		api.EnrollEnrollErOttHandler = enroll.EnrollErOttHandlerFunc(func(params enroll.EnrollErOttParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.EnrollErOtt has not yet been implemented")
		})
	}
	if api.CurrentIdentityEnrollMfaHandler == nil {
		api.CurrentIdentityEnrollMfaHandler = current_identity.EnrollMfaHandlerFunc(func(params current_identity.EnrollMfaParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.EnrollMfa has not yet been implemented")
		})
	}
	if api.EnrollEnrollOttHandler == nil {
		api.EnrollEnrollOttHandler = enroll.EnrollOttHandlerFunc(func(params enroll.EnrollOttParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.EnrollOtt has not yet been implemented")
		})
	}
	if api.EnrollEnrollOttCaHandler == nil {
		api.EnrollEnrollOttCaHandler = enroll.EnrollOttCaHandlerFunc(func(params enroll.EnrollOttCaParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.EnrollOttCa has not yet been implemented")
		})
	}
	if api.EnrollErnollUpdbHandler == nil {
		api.EnrollErnollUpdbHandler = enroll.ErnollUpdbHandlerFunc(func(params enroll.ErnollUpdbParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.ErnollUpdb has not yet been implemented")
		})
	}
	if api.CurrentAPISessionExtendCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionExtendCurrentIdentityAuthenticatorHandler = current_api_session.ExtendCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.ExtendCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.ExtendCurrentIdentityAuthenticator has not yet been implemented")
		})
	}
	if api.EnrollExtendRouterEnrollmentHandler == nil {
		api.EnrollExtendRouterEnrollmentHandler = enroll.ExtendRouterEnrollmentHandlerFunc(func(params enroll.ExtendRouterEnrollmentParams) middleware.Responder {
			return middleware.NotImplemented("operation enroll.ExtendRouterEnrollment has not yet been implemented")
		})
	}
	if api.CurrentAPISessionExtendVerifyCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionExtendVerifyCurrentIdentityAuthenticatorHandler = current_api_session.ExtendVerifyCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.ExtendVerifyCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.ExtendVerifyCurrentIdentityAuthenticator has not yet been implemented")
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
	if api.CurrentIdentityGetCurrentIdentityEdgeRoutersHandler == nil {
		api.CurrentIdentityGetCurrentIdentityEdgeRoutersHandler = current_identity.GetCurrentIdentityEdgeRoutersHandlerFunc(func(params current_identity.GetCurrentIdentityEdgeRoutersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_identity.GetCurrentIdentityEdgeRouters has not yet been implemented")
		})
	}
	if api.CurrentAPISessionListCurrentAPISessionCertificatesHandler == nil {
		api.CurrentAPISessionListCurrentAPISessionCertificatesHandler = current_api_session.ListCurrentAPISessionCertificatesHandlerFunc(func(params current_api_session.ListCurrentAPISessionCertificatesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.ListCurrentAPISessionCertificates has not yet been implemented")
		})
	}
	if api.CurrentAPISessionListCurrentIdentityAuthenticatorsHandler == nil {
		api.CurrentAPISessionListCurrentIdentityAuthenticatorsHandler = current_api_session.ListCurrentIdentityAuthenticatorsHandlerFunc(func(params current_api_session.ListCurrentIdentityAuthenticatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.ListCurrentIdentityAuthenticators has not yet been implemented")
		})
	}
	if api.ExternalJWTSignerListExternalJWTSignersHandler == nil {
		api.ExternalJWTSignerListExternalJWTSignersHandler = external_jwt_signer.ListExternalJWTSignersHandlerFunc(func(params external_jwt_signer.ListExternalJWTSignersParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation external_jwt_signer.ListExternalJWTSigners has not yet been implemented")
		})
	}
	if api.InformationalListProtocolsHandler == nil {
		api.InformationalListProtocolsHandler = informational.ListProtocolsHandlerFunc(func(params informational.ListProtocolsParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListProtocols has not yet been implemented")
		})
	}
	if api.InformationalListRootHandler == nil {
		api.InformationalListRootHandler = informational.ListRootHandlerFunc(func(params informational.ListRootParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListRoot has not yet been implemented")
		})
	}
	if api.ServiceListServiceTerminatorsHandler == nil {
		api.ServiceListServiceTerminatorsHandler = service.ListServiceTerminatorsHandlerFunc(func(params service.ListServiceTerminatorsParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.ListServiceTerminators has not yet been implemented")
		})
	}
	if api.CurrentAPISessionListServiceUpdatesHandler == nil {
		api.CurrentAPISessionListServiceUpdatesHandler = current_api_session.ListServiceUpdatesHandlerFunc(func(params current_api_session.ListServiceUpdatesParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.ListServiceUpdates has not yet been implemented")
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
	if api.InformationalListVersionHandler == nil {
		api.InformationalListVersionHandler = informational.ListVersionHandlerFunc(func(params informational.ListVersionParams) middleware.Responder {
			return middleware.NotImplemented("operation informational.ListVersion has not yet been implemented")
		})
	}
	if api.WellKnownListWellKnownCasHandler == nil {
		api.WellKnownListWellKnownCasHandler = well_known.ListWellKnownCasHandlerFunc(func(params well_known.ListWellKnownCasParams) middleware.Responder {
			return middleware.NotImplemented("operation well_known.ListWellKnownCas has not yet been implemented")
		})
	}
	if api.CurrentAPISessionPatchCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionPatchCurrentIdentityAuthenticatorHandler = current_api_session.PatchCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.PatchCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.PatchCurrentIdentityAuthenticator has not yet been implemented")
		})
	}
	if api.ServicePatchServiceHandler == nil {
		api.ServicePatchServiceHandler = service.PatchServiceHandlerFunc(func(params service.PatchServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.PatchService has not yet been implemented")
		})
	}
	if api.CurrentAPISessionUpdateCurrentIdentityAuthenticatorHandler == nil {
		api.CurrentAPISessionUpdateCurrentIdentityAuthenticatorHandler = current_api_session.UpdateCurrentIdentityAuthenticatorHandlerFunc(func(params current_api_session.UpdateCurrentIdentityAuthenticatorParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation current_api_session.UpdateCurrentIdentityAuthenticator has not yet been implemented")
		})
	}
	if api.ServiceUpdateServiceHandler == nil {
		api.ServiceUpdateServiceHandler = service.UpdateServiceHandlerFunc(func(params service.UpdateServiceParams, principal interface{}) middleware.Responder {
			return middleware.NotImplemented("operation service.UpdateService has not yet been implemented")
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
