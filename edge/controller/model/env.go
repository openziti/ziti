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

package model

import (
	edgeconfig "github.com/netfoundry/ziti-edge/edge/controller/config"
	"github.com/netfoundry/ziti-edge/edge/controller/persistence"
	"github.com/netfoundry/ziti-edge/edge/internal/cert"
	"github.com/netfoundry/ziti-edge/edge/internal/jwt"
	"github.com/netfoundry/ziti-fabric/controller/network"
	"github.com/xeipuuv/gojsonschema"
)

type Env interface {
	GetHandlers() *Handlers
	GetConfig() *edgeconfig.Config
	GetEnrollmentJwtGenerator() jwt.EnrollmentGenerator
	GetDbProvider() persistence.DbProvider
	GetStores() *persistence.Stores
	GetAuthRegistry() AuthRegistry
	GetEnrollRegistry() EnrollmentRegistry
	GetApiClientCsrSigner() cert.Signer
	GetApiServerCsrSigner() cert.Signer
	GetControlClientCsrSigner() cert.Signer
	GetHostController() HostController
	GetSchemas() Schemas
	IsEdgeRouterOnline(id string) bool
}

type HostController interface {
	GetNetwork() *network.Network
}

type Schemas interface {
	GetEnrollErPost() *gojsonschema.Schema
	GetEnrollUpdbPost() *gojsonschema.Schema
}
