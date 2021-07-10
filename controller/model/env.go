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
	"github.com/openziti/edge/controller/config"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/edge/internal/cert"
	"github.com/openziti/edge/internal/jwtsigner"
	"github.com/openziti/fabric/controller/network"
	"github.com/openziti/foundation/metrics"
	"github.com/xeipuuv/gojsonschema"
)

type Env interface {
	GetHandlers() *Handlers
	GetConfig() *config.Config
	GetJwtSigner() jwtsigner.Signer
	GetDbProvider() persistence.DbProvider
	GetStores() *persistence.Stores
	GetAuthRegistry() AuthRegistry
	GetEnrollRegistry() EnrollmentRegistry
	GetApiClientCsrSigner() cert.Signer
	GetApiServerCsrSigner() cert.Signer
	GetControlClientCsrSigner() cert.Signer
	GetHostController() HostController
	IsEdgeRouterOnline(id string) bool
	GetMetricsRegistry() metrics.Registry
	GetFingerprintGenerator() cert.FingerprintGenerator
	HandleServiceUpdatedEventForIdentityId(identityId string)
}

type HostController interface {
	GetNetwork() *network.Network
	Shutdown()
	GetCloseNotifyChannel() <-chan struct{}
}

type Schemas interface {
	GetEnrollErPost() *gojsonschema.Schema
	GetEnrollUpdbPost() *gojsonschema.Schema
}
