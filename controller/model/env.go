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

package model

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/cert"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/config"
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/controller/event"
	"github.com/openziti/ziti/controller/jwtsigner"
)

type Env interface {
	GetCommandDispatcher() command.Dispatcher
	GetManagers() *Managers
	GetEventDispatcher() event.Dispatcher
	GetConfig() *config.Config
	GetDb() boltz.Db
	GetStores() *db.Stores
	GetAuthRegistry() AuthRegistry
	GetEnrollRegistry() EnrollmentRegistry
	GetApiClientCsrSigner() cert.Signer
	GetApiServerCsrSigner() cert.Signer
	GetControlClientCsrSigner() cert.Signer
	IsEdgeRouterOnline(id string) bool
	GetMetricsRegistry() metrics.Registry
	GetFingerprintGenerator() cert.FingerprintGenerator
	HandleServiceUpdatedEventForIdentityId(identityId string)

	GetServerJwtSigner() jwtsigner.Signer
	GetServerCert() (*tls.Certificate, string, jwt.SigningMethod)
	JwtSignerKeyFunc(token *jwt.Token) (interface{}, error)
	GetPeerControllerAddresses() []string

	ValidateAccessToken(token string) (*common.AccessClaims, error)
	ValidateServiceAccessToken(token string, apiSessionId *string) (*common.ServiceAccessClaims, error)

	OidcIssuer() string
	RootIssuer() string

	GetRaftInfo() (string, string, string)
	GetApiAddresses() (map[string][]event.ApiAddress, []byte)
	GetCloseNotifyChannel() <-chan struct{}
	GetPeerSigners() []*x509.Certificate
}
