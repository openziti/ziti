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

package rest_server

import (
	"crypto/tls"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"

	"github.com/openziti/ziti/v2/controller/rest_server/operations"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/circuit"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/cluster"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/database"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/inspect"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/link"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/router"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/service"
	"github.com/openziti/ziti/v2/controller/rest_server/operations/terminator"
)

//go:generate swagger generate server --target ../../controller --name ZitiFabric --spec ../specs/swagger.yml --model-package rest_model --server-package rest_server --principal any --exclude-main

func configureFlags(api *operations.ZitiFabricAPI) {
	// api.CommandLineOptionsGroups = []swag.CommandLineOptionsGroup{ ... }
	_ = api
}

func configureAPI(api *operations.ZitiFabricAPI) http.Handler {
	// configure the api here
	api.ServeError = errors.ServeError

	// Set your custom logger if needed. Default one is log.Printf
	// Expected interface func(string, ...any)
	//
	// Example:
	// api.Logger = log.Printf

	api.UseSwaggerUI()
	// To continue using redoc as your UI, uncomment the following line
	// api.UseRedoc()

	api.JSONConsumer = runtime.JSONConsumer()

	api.JSONProducer = runtime.JSONProducer()

	if api.Oauth2Auth == nil {
		api.Oauth2Auth = func(token string, scopes []string) (any, error) {
			_ = token
			_ = scopes

			return nil, errors.NotImplemented("oauth2 bearer auth (oauth2) has not yet been implemented")
		}
	}
	// Applies when the "zt-session" header is set
	if api.ZtSessionAuth == nil {
		api.ZtSessionAuth = func(token string) (any, error) {
			_ = token

			return nil, errors.NotImplemented("api key auth (ztSession) zt-session from header param [zt-session] has not yet been implemented")
		}
	}

	// Set your custom authorizer if needed. Default one is security.Authorized()
	// Expected interface runtime.Authorizer
	//
	// Example:
	// api.APIAuthorizer = security.Authorized()

	if api.DatabaseCheckDataIntegrityHandler == nil {
		api.DatabaseCheckDataIntegrityHandler = database.CheckDataIntegrityHandlerFunc(func(params database.CheckDataIntegrityParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation database.CheckDataIntegrity has not yet been implemented")
		})
	}
	if api.ClusterClusterListMembersHandler == nil {
		api.ClusterClusterListMembersHandler = cluster.ClusterListMembersHandlerFunc(func(params cluster.ClusterListMembersParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation cluster.ClusterListMembers has not yet been implemented")
		})
	}
	if api.ClusterClusterMemberAddHandler == nil {
		api.ClusterClusterMemberAddHandler = cluster.ClusterMemberAddHandlerFunc(func(params cluster.ClusterMemberAddParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation cluster.ClusterMemberAdd has not yet been implemented")
		})
	}
	if api.ClusterClusterMemberRemoveHandler == nil {
		api.ClusterClusterMemberRemoveHandler = cluster.ClusterMemberRemoveHandlerFunc(func(params cluster.ClusterMemberRemoveParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation cluster.ClusterMemberRemove has not yet been implemented")
		})
	}
	if api.ClusterClusterTransferLeadershipHandler == nil {
		api.ClusterClusterTransferLeadershipHandler = cluster.ClusterTransferLeadershipHandlerFunc(func(params cluster.ClusterTransferLeadershipParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation cluster.ClusterTransferLeadership has not yet been implemented")
		})
	}
	if api.DatabaseCreateDatabaseSnapshotHandler == nil {
		api.DatabaseCreateDatabaseSnapshotHandler = database.CreateDatabaseSnapshotHandlerFunc(func(params database.CreateDatabaseSnapshotParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation database.CreateDatabaseSnapshot has not yet been implemented")
		})
	}
	if api.DatabaseCreateDatabaseSnapshotWithPathHandler == nil {
		api.DatabaseCreateDatabaseSnapshotWithPathHandler = database.CreateDatabaseSnapshotWithPathHandlerFunc(func(params database.CreateDatabaseSnapshotWithPathParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation database.CreateDatabaseSnapshotWithPath has not yet been implemented")
		})
	}
	if api.RouterCreateRouterHandler == nil {
		api.RouterCreateRouterHandler = router.CreateRouterHandlerFunc(func(params router.CreateRouterParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.CreateRouter has not yet been implemented")
		})
	}
	if api.ServiceCreateServiceHandler == nil {
		api.ServiceCreateServiceHandler = service.CreateServiceHandlerFunc(func(params service.CreateServiceParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.CreateService has not yet been implemented")
		})
	}
	if api.TerminatorCreateTerminatorHandler == nil {
		api.TerminatorCreateTerminatorHandler = terminator.CreateTerminatorHandlerFunc(func(params terminator.CreateTerminatorParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation terminator.CreateTerminator has not yet been implemented")
		})
	}
	if api.DatabaseDataIntegrityResultsHandler == nil {
		api.DatabaseDataIntegrityResultsHandler = database.DataIntegrityResultsHandlerFunc(func(params database.DataIntegrityResultsParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation database.DataIntegrityResults has not yet been implemented")
		})
	}
	if api.CircuitDeleteCircuitHandler == nil {
		api.CircuitDeleteCircuitHandler = circuit.DeleteCircuitHandlerFunc(func(params circuit.DeleteCircuitParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation circuit.DeleteCircuit has not yet been implemented")
		})
	}
	if api.LinkDeleteLinkHandler == nil {
		api.LinkDeleteLinkHandler = link.DeleteLinkHandlerFunc(func(params link.DeleteLinkParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation link.DeleteLink has not yet been implemented")
		})
	}
	if api.RouterDeleteRouterHandler == nil {
		api.RouterDeleteRouterHandler = router.DeleteRouterHandlerFunc(func(params router.DeleteRouterParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.DeleteRouter has not yet been implemented")
		})
	}
	if api.ServiceDeleteServiceHandler == nil {
		api.ServiceDeleteServiceHandler = service.DeleteServiceHandlerFunc(func(params service.DeleteServiceParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.DeleteService has not yet been implemented")
		})
	}
	if api.TerminatorDeleteTerminatorHandler == nil {
		api.TerminatorDeleteTerminatorHandler = terminator.DeleteTerminatorHandlerFunc(func(params terminator.DeleteTerminatorParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation terminator.DeleteTerminator has not yet been implemented")
		})
	}
	if api.CircuitDetailCircuitHandler == nil {
		api.CircuitDetailCircuitHandler = circuit.DetailCircuitHandlerFunc(func(params circuit.DetailCircuitParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation circuit.DetailCircuit has not yet been implemented")
		})
	}
	if api.LinkDetailLinkHandler == nil {
		api.LinkDetailLinkHandler = link.DetailLinkHandlerFunc(func(params link.DetailLinkParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation link.DetailLink has not yet been implemented")
		})
	}
	if api.RouterDetailRouterHandler == nil {
		api.RouterDetailRouterHandler = router.DetailRouterHandlerFunc(func(params router.DetailRouterParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.DetailRouter has not yet been implemented")
		})
	}
	if api.ServiceDetailServiceHandler == nil {
		api.ServiceDetailServiceHandler = service.DetailServiceHandlerFunc(func(params service.DetailServiceParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.DetailService has not yet been implemented")
		})
	}
	if api.TerminatorDetailTerminatorHandler == nil {
		api.TerminatorDetailTerminatorHandler = terminator.DetailTerminatorHandlerFunc(func(params terminator.DetailTerminatorParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation terminator.DetailTerminator has not yet been implemented")
		})
	}
	if api.DatabaseFixDataIntegrityHandler == nil {
		api.DatabaseFixDataIntegrityHandler = database.FixDataIntegrityHandlerFunc(func(params database.FixDataIntegrityParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation database.FixDataIntegrity has not yet been implemented")
		})
	}
	if api.InspectInspectHandler == nil {
		api.InspectInspectHandler = inspect.InspectHandlerFunc(func(params inspect.InspectParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation inspect.Inspect has not yet been implemented")
		})
	}
	if api.CircuitListCircuitsHandler == nil {
		api.CircuitListCircuitsHandler = circuit.ListCircuitsHandlerFunc(func(params circuit.ListCircuitsParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation circuit.ListCircuits has not yet been implemented")
		})
	}
	if api.LinkListLinksHandler == nil {
		api.LinkListLinksHandler = link.ListLinksHandlerFunc(func(params link.ListLinksParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation link.ListLinks has not yet been implemented")
		})
	}
	if api.RouterListRouterTerminatorsHandler == nil {
		api.RouterListRouterTerminatorsHandler = router.ListRouterTerminatorsHandlerFunc(func(params router.ListRouterTerminatorsParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.ListRouterTerminators has not yet been implemented")
		})
	}
	if api.RouterListRoutersHandler == nil {
		api.RouterListRoutersHandler = router.ListRoutersHandlerFunc(func(params router.ListRoutersParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.ListRouters has not yet been implemented")
		})
	}
	if api.ServiceListServiceTerminatorsHandler == nil {
		api.ServiceListServiceTerminatorsHandler = service.ListServiceTerminatorsHandlerFunc(func(params service.ListServiceTerminatorsParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.ListServiceTerminators has not yet been implemented")
		})
	}
	if api.ServiceListServicesHandler == nil {
		api.ServiceListServicesHandler = service.ListServicesHandlerFunc(func(params service.ListServicesParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.ListServices has not yet been implemented")
		})
	}
	if api.TerminatorListTerminatorsHandler == nil {
		api.TerminatorListTerminatorsHandler = terminator.ListTerminatorsHandlerFunc(func(params terminator.ListTerminatorsParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation terminator.ListTerminators has not yet been implemented")
		})
	}
	if api.LinkPatchLinkHandler == nil {
		api.LinkPatchLinkHandler = link.PatchLinkHandlerFunc(func(params link.PatchLinkParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation link.PatchLink has not yet been implemented")
		})
	}
	if api.RouterPatchRouterHandler == nil {
		api.RouterPatchRouterHandler = router.PatchRouterHandlerFunc(func(params router.PatchRouterParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.PatchRouter has not yet been implemented")
		})
	}
	if api.ServicePatchServiceHandler == nil {
		api.ServicePatchServiceHandler = service.PatchServiceHandlerFunc(func(params service.PatchServiceParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.PatchService has not yet been implemented")
		})
	}
	if api.TerminatorPatchTerminatorHandler == nil {
		api.TerminatorPatchTerminatorHandler = terminator.PatchTerminatorHandlerFunc(func(params terminator.PatchTerminatorParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation terminator.PatchTerminator has not yet been implemented")
		})
	}
	if api.RouterUpdateRouterHandler == nil {
		api.RouterUpdateRouterHandler = router.UpdateRouterHandlerFunc(func(params router.UpdateRouterParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation router.UpdateRouter has not yet been implemented")
		})
	}
	if api.ServiceUpdateServiceHandler == nil {
		api.ServiceUpdateServiceHandler = service.UpdateServiceHandlerFunc(func(params service.UpdateServiceParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation service.UpdateService has not yet been implemented")
		})
	}
	if api.TerminatorUpdateTerminatorHandler == nil {
		api.TerminatorUpdateTerminatorHandler = terminator.UpdateTerminatorHandlerFunc(func(params terminator.UpdateTerminatorParams, principal any) middleware.Responder {
			_ = params
			_ = principal

			return middleware.NotImplemented("operation terminator.UpdateTerminator has not yet been implemented")
		})
	}

	api.PreServerShutdown = func() {}

	api.ServerShutdown = func() {}

	return setupGlobalMiddleware(api.Serve(setupMiddlewares))
}

// The TLS configuration before HTTPS server starts.
func configureTLS(tlsConfig *tls.Config) {
	// Make all necessary changes to the TLS configuration here.
	_ = tlsConfig
}

// As soon as server is initialized but not run yet, this function will be called.
// If you need to modify a config, store server instance to stop it individually later, this is the place.
// This function can be called multiple times, depending on the number of serving schemes.
// scheme value will be set accordingly: "http", "https" or "unix".
func configureServer(server *http.Server, scheme, addr string) {
	_ = server
	_ = scheme
	_ = addr
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
