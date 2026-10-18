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
	"crypto/x509"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/jwtsigner"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
)

// Env defines the core environment interface for Ziti Edge controller operations.
// It provides access to all essential services including data stores, authentication,
// certificate management, JWT operations, network information, and controller coordination.
// This interface abstracts the controller's runtime environment and is implemented by AppEnv.
type Env interface {

	// GetCommandDispatcher provides access to the command processing system for executing
	// control plane operations like configuration changes and administrative tasks.
	GetCommandDispatcher() command.Dispatcher

	// GetManagers provides access to business logic managers that handle CRUD operations
	// for entities like identities, services, policies, and certificates.
	GetManagers() *Managers

	// GetEventDispatcher enables publishing system events for auditing, monitoring,
	// and integration with external systems.
	GetEventDispatcher() event.Dispatcher

	// GetConfig provides access to controller configuration for runtime behavior customization.
	GetConfig() *config.Config

	// GetDb provides direct access to the underlying database for low-level operations.
	GetDb() boltz.Db

	// GetStores provides access to the data access layer for entity persistence and querying.
	GetStores() *db.Stores

	// GetAuthRegistry provides access to pluggable authentication modules for different
	// authentication methods like certificates, UPDB, and external JWT.
	GetAuthRegistry() AuthRegistry

	// GetEnrollRegistry provides access to enrollment handlers that process different
	// types of enrollment requests (OTTCA, UPDB, etc.).
	GetEnrollRegistry() EnrollmentRegistry

	// GetApiClientCsrSigner provides certificate signing capability for API clients
	// during enrollment and certificate renewal processes.
	GetApiClientCsrSigner() cert.Signer

	// GetApiServerCsrSigner provides certificate signing capability for API servers
	// in multi-controller deployments.
	GetApiServerCsrSigner() cert.Signer

	// GetControlClientCsrSigner provides certificate signing capability for control
	// plane clients like routers connecting to the controller.
	GetControlClientCsrSigner() cert.Signer

	// IsEdgeRouterOnline enables checking router connectivity status for service
	// availability and load balancing decisions.
	IsEdgeRouterOnline(id string) bool

	// GetMetricsRegistry provides access to performance metrics collections for
	// monitoring, alerting, and system health assessment.
	GetMetricsRegistry() metrics.Registry

	// GetFingerprintGenerator creates certificate fingerprints for identity verification
	// and certificate matching during authentication.
	GetFingerprintGenerator() cert.FingerprintGenerator

	// HandleServiceUpdatedEventForIdentityId triggers identity refresh when services
	// change, ensuring clients get updated service lists promptly.
	HandleServiceUpdatedEventForIdentityId(identityId string)

	// GetEnrollmentJwtSigner creates JWT tokens for enrollment processes, matching
	// the hostname in edge.api.address for proper certificate validation.
	GetEnrollmentJwtSigner() (jwtsigner.Signer, error)

	// GetRootTlsJwtSigner provides JWT signing using the controller's root certificate
	// for administrative operations and inter-controller communication.
	GetRootTlsJwtSigner() *jwtsigner.TlsJwtSigner

	// GetClientApiDefaultTlsJwtSigner provides the standard JWT signer for client API
	// operations like authentication.
	GetClientApiDefaultTlsJwtSigner() *jwtsigner.TlsJwtSigner

	// JwtSignerKeyFunc enables JWT token verification from multiple controllers in
	// clustered deployments by providing appropriate public keys.
	JwtSignerKeyFunc(token *jwt.Token) (interface{}, error)

	// GetPeerControllerAddresses provides network addresses of other controllers
	// for cluster coordination and failover scenarios.
	GetPeerControllerAddresses() []string

	// ValidateAccessToken verifies and extracts claims from access tokens, ensuring
	// proper authentication and authorization for API requests.
	ValidateAccessToken(token string) (*common.AccessClaims, error)

	// ValidateServiceAccessToken validates tokens used for service-specific access,
	// enabling fine-grained authorization for individual services.
	ValidateServiceAccessToken(token string, apiSessionId *string) (*common.ServiceAccessClaims, error)

	// OidcIssuer provides the OIDC-compliant issuer URL for integration with
	// external identity providers and OAuth2/OIDC flows.
	OidcIssuer() string

	// RootIssuer provides the base issuer URL for JWT tokens and OIDC discovery,
	// derived from the controller's API address.
	RootIssuer() string

	// GetRaftInfo exposes Raft consensus information for cluster health monitoring
	// and debugging distributed consensus issues.
	GetRaftInfo() (string, string, string)

	// GetRaftIndex returns the raft index this controller has applied, which versions the answers its stores
	// give. Statements this controller makes about what exists can be ordered against another controller's,
	// which nothing else sequences.
	//
	// Zero where there is no such index, as on a controller running without raft. It is not the router data
	// model's index, which counts a different sequence of changes.
	GetRaftIndex() uint64

	// RequestReadBarrier asks the raft leader for a read barrier and returns without waiting; GetReadBarrier
	// reports the answer once it arrives. A request is not guaranteed an answer, so a caller that still needs
	// one asks again later. Does nothing without raft.
	RequestReadBarrier()

	// GetReadBarrier returns the latest answered read barrier: a store index, in the measure GetRaftIndex
	// reports, and the local time it was requested. Once GetRaftIndex reaches index, this controller has
	// applied everything the cluster had committed at requestedAt. requestedAt is zero before any has been
	// answered. Without raft it returns zero and the current time, since a lone controller's store is always
	// current.
	GetReadBarrier() (index uint64, requestedAt time.Time)

	// HasRaftLeader reports whether this controller currently knows of a leader, which is true when it is the
	// leader itself. False when isolated or mid-election.
	//
	// Callers that draw conclusions from an entity's absence need this: a controller with no leader is not
	// receiving commits, so its view is frozen rather than merely behind, and waiting for an index it will
	// never be told about would let it mistake isolation for deletion. Always true without raft.
	HasRaftLeader() bool

	// GetApiAddresses provides current API endpoints and their signatures for
	// service discovery and client configuration updates.
	GetApiAddresses() (map[string][]event.ApiAddress, []byte)

	// GetCloseNotifyChannel enables graceful shutdown coordination by signaling
	// when the controller is terminating.
	GetCloseNotifyChannel() <-chan struct{}

	// GetPeerSigners provides peer controller certificates for validating signed
	// messages and ensuring secure inter-controller communication.
	GetPeerSigners() []*x509.Certificate

	// AddRouterPresenceHandler enables monitoring router connectivity changes
	// for network topology updates and service availability tracking.
	AddRouterPresenceHandler(h RouterPresenceHandler)

	// GetId provides the unique controller instance identifier for cluster
	// coordination and distributed system operations.
	GetId() string

	// GetTokenIssuerCache provides access to the cache of external JWT token issuers.
	// Used for token-based enrollment and JWT authentication to verify tokens from external identity providers.
	GetTokenIssuerCache() *TokenIssuerCache

	// CreateTotpTokenFromAccessClaims creates a new TOTP JWT for the given access claims
	CreateTotpTokenFromAccessClaims(issuer string, claims *common.AccessClaims) (string, *common.TotpClaims, error)
}
