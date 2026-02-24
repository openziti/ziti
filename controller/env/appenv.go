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

package env

import (
	"bytes"
	"crypto"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	openApiMiddleware "github.com/go-openapi/runtime/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	clientServer "github.com/openziti/edge-api/rest_client_api_server"
	clientOperations "github.com/openziti/edge-api/rest_client_api_server/operations"
	managementServer "github.com/openziti/edge-api/rest_management_api_server"
	managementOperations "github.com/openziti/edge-api/rest_management_api_server/operations"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/identity"
	"github.com/openziti/metrics"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/cert"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/api"
	"github.com/openziti/ziti/v2/controller/command"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/openziti/ziti/v2/controller/events"
	"github.com/openziti/ziti/v2/controller/jwtsigner"
	"github.com/openziti/ziti/v2/controller/model"

	"github.com/openziti/ziti/v2/controller/network"
	"github.com/openziti/ziti/v2/controller/permissions"
	"github.com/openziti/ziti/v2/controller/response"
	fabricServer "github.com/openziti/ziti/v2/controller/rest_server"
	fabricOperations "github.com/openziti/ziti/v2/controller/rest_server/operations"
	"github.com/openziti/ziti/v2/controller/xctrl"
	"github.com/openziti/ziti/v2/controller/xmgmt"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/teris-io/shortid"
	"github.com/xeipuuv/gojsonschema"
)

var _ model.Env = &AppEnv{}

const (
	ZitiSession      = "zt-session"
	ClientApiBinding = "edge-client"

	JwtAudEnrollment = "openziti-enroller"
)

const (
	metricAuthLimiterCurrentQueuedCount = "auth.limiter.queued_count"
	metricAuthLimiterCurrentWindowSize  = "auth.limiter.window_size"
	metricAuthLimiterWorkTimer          = "auth.limiter.work_timer"
)

type AppEnv struct {
	Stores   *db.Stores
	Managers *model.Managers

	Versions *ziti.Versions

	ApiServerCsrSigner     cert.Signer
	ApiClientCsrSigner     cert.Signer
	ControlClientCsrSigner cert.Signer

	FingerprintGenerator cert.FingerprintGenerator
	AuthRegistry         model.AuthRegistry
	EnrollRegistry       model.EnrollmentRegistry
	Broker               *Broker
	HostController       HostController
	FabricApi            *fabricOperations.ZitiFabricAPI
	ManagementApi        *managementOperations.ZitiEdgeManagementAPI
	ClientApi            *clientOperations.ZitiEdgeClientAPI
	IdentityRefreshMap   cmap.ConcurrentMap[string, time.Time]
	identityRefreshMeter metrics.Meter
	StartupTime          time.Time
	InstanceId           string
	AuthRateLimiter      rate.AdaptiveRateLimiter

	clientApiDefaultSigner *jwtsigner.TlsJwtSigner

	TraceManager *TraceManager
	timelineId   concurrenz.AtomicValue[string]

	TokenIssuerCache *model.TokenIssuerCache
}

// GetTokenIssuerCache returns the TokenIssuerCache instance for verifying external JWT tokens.
func (ae *AppEnv) GetTokenIssuerCache() *model.TokenIssuerCache {
	return ae.TokenIssuerCache
}

func (ae *AppEnv) CreateTotpTokenFromAccessClaims(issuer string, claims *common.AccessClaims) (string, *common.TotpClaims, error) {
	if claims == nil {
		return "", nil, errors.New("claims cannot be nil")
	}

	if issuer == "" {
		return "", nil, errors.New("issuer cannot be empty")
	}

	now := time.Now()
	nowTime := jwt.NumericDate{Time: now}
	totpClaims := &common.TotpClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   issuer,
			Subject:  claims.Subject,
			IssuedAt: &nowTime,
			ID:       uuid.NewString(),
		},
		ApiSessionId: claims.ApiSessionId,
		Type:         common.TokenTypeTotp,
	}

	for _, aud := range claims.Audience {
		totpClaims.Audience = append(totpClaims.Audience, aud)
	}

	jwtSigner := ae.GetClientApiDefaultTlsJwtSigner()
	tokenStr, err := jwtSigner.Generate(totpClaims)
	if err != nil {
		return "", nil, err
	}

	return tokenStr, totpClaims, nil
}

// GetPeerControllerAddresses returns the network addresses of peer controllers.
func (ae *AppEnv) GetPeerControllerAddresses() []string {
	return ae.HostController.GetPeerAddresses()
}

// JwtSignerKeyFunc is used in combination with jwt.Parse or jwt.ParseWithClaims to
// facilitate verifying JWTs from the current controller or any peer controllers.
func (ae *AppEnv) JwtSignerKeyFunc(token *jwt.Token) (interface{}, error) {
	kidToPubKey := ae.Broker.GetPublicKeys()

	val := token.Header["kid"]
	targetKid := val.(string)

	if targetKid == "" {
		return nil, errors.New("missing kid in token")
	}

	pubKey, ok := kidToPubKey[targetKid]

	if !ok {
		return nil, errors.New("invalid kid: " + targetKid)
	}

	return pubKey, nil
}

// ValidateAccessToken verifies an access token and returns its claims if valid.
// Checks token signature, audience, type, and revocation status.
func (ae *AppEnv) ValidateAccessToken(token string) (*common.AccessClaims, error) {
	accessClaims := &common.AccessClaims{}

	parsedToken, err := jwt.ParseWithClaims(token, accessClaims, ae.JwtSignerKeyFunc)

	if err != nil {
		return nil, err
	}

	if !parsedToken.Valid {
		return nil, errors.New("access token is invalid")
	}

	if !accessClaims.HasAudience(common.ClaimAudienceOpenZiti) && !accessClaims.HasAudience(common.ClaimLegacyNative) {
		return nil, fmt.Errorf("invalid audience, expected an instance of %s or %s, got %v", common.ClaimAudienceOpenZiti, common.ClaimLegacyNative, accessClaims.Audience)
	}

	if accessClaims.Type != common.TokenTypeAccess {
		return nil, fmt.Errorf("invalid token type, expected %s, got %s", common.TokenTypeAccess, accessClaims.Type)
	}

	tokenRevocation, err := ae.GetManagers().Revocation.Read(accessClaims.JWTID)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return nil, err
	}

	if tokenRevocation != nil {
		return nil, errors.New("access token has been revoked by id")
	}

	revocation, err := ae.GetManagers().Revocation.Read(accessClaims.Subject)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return nil, err
	}

	if revocation != nil && revocation.CreatedAt.After(accessClaims.IssuedAt.AsTime()) {
		return nil, errors.New("access token has been revoked by identity")
	}

	return accessClaims, nil
}

// ValidateServiceAccessToken verifies a service access token and returns its claims.
// Optionally validates against a specific API session ID.
func (ae *AppEnv) ValidateServiceAccessToken(token string, apiSessionId *string) (*common.ServiceAccessClaims, error) {
	serviceAccessClaims := &common.ServiceAccessClaims{}

	parsedToken, err := jwt.ParseWithClaims(token, serviceAccessClaims, ae.JwtSignerKeyFunc)

	if err != nil {
		return nil, err
	}

	if !parsedToken.Valid {
		return nil, errors.New("service access token is invalid")
	}

	if !serviceAccessClaims.HasAudience(common.ClaimAudienceOpenZiti) && !serviceAccessClaims.HasAudience(common.ClaimLegacyNative) {
		return nil, fmt.Errorf("invalid audience, expected an instance of %s or %s, got %v", common.ClaimAudienceOpenZiti, common.ClaimLegacyNative, serviceAccessClaims.Audience)
	}

	if serviceAccessClaims.TokenType != common.TokenTypeServiceAccess {
		return nil, fmt.Errorf("invalid token type, expected %s, got %s", common.TokenTypeServiceAccess, serviceAccessClaims.Type)
	}

	if apiSessionId != nil {
		if *apiSessionId == "" {
			return nil, errors.New("invalid target api session id, must not be empty string")
		}

		if serviceAccessClaims.ApiSessionId != *apiSessionId {
			return nil, fmt.Errorf("invalid api session id, expected %s, got %s", *apiSessionId, serviceAccessClaims.ApiSessionId)
		}
	}

	tokenRevocation, err := ae.GetManagers().Revocation.Read(serviceAccessClaims.ID)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return nil, err
	}

	if tokenRevocation != nil {
		return nil, errors.New("service access token has been revoked by id")
	}

	revocation, err := ae.GetManagers().Revocation.Read(serviceAccessClaims.IdentityId)

	if err != nil && !boltz.IsErrNotFoundErr(err) {
		return nil, err
	}

	if revocation != nil && revocation.CreatedAt.After(serviceAccessClaims.IssuedAt.Time) {
		return nil, errors.New("service access token has been revoked by identity")
	}

	return serviceAccessClaims, nil
}

// GetClientApiDefaultTlsJwtSigner returns the default JWT signer for client API operations.
func (ae *AppEnv) GetClientApiDefaultTlsJwtSigner() *jwtsigner.TlsJwtSigner {
	return ae.clientApiDefaultSigner
}

// GetRootTlsJwtSigner creates and returns a JWT signer using the root server certificate.
func (ae *AppEnv) GetRootTlsJwtSigner() *jwtsigner.TlsJwtSigner {
	rootCerts := ae.GetConfig().Id.ServerCert()
	var rootCert *tls.Certificate

	if len(rootCerts) != 0 {
		rootCert = rootCerts[0]
	} else {
		rootCert = ae.GetConfig().Id.Cert()
	}

	if rootCert == nil {
		panic(fmt.Errorf("root identity doesn't have a server cert or cert"))
	}

	rootSigner := &jwtsigner.TlsJwtSigner{}
	err := rootSigner.Set(rootCerts[0])

	if err != nil {
		pfxlog.Logger().WithError(err).Panic("failed to set root controller identity signer")
	}

	return rootSigner
}

// GetApiServerCsrSigner returns the certificate signer for API server CSRs.
func (ae *AppEnv) GetApiServerCsrSigner() cert.Signer {
	return ae.ApiServerCsrSigner
}

// GetControlClientCsrSigner returns the certificate signer for control client CSRs.
func (ae *AppEnv) GetControlClientCsrSigner() cert.Signer {
	return ae.ControlClientCsrSigner
}

// GetApiClientCsrSigner returns the certificate signer for API client CSRs.
func (ae *AppEnv) GetApiClientCsrSigner() cert.Signer {
	return ae.ApiClientCsrSigner
}

// GetHostController returns the host controller instance.
func (ae *AppEnv) GetHostController() HostController {
	return ae.HostController
}

func (ae *AppEnv) GetNetwork() *network.Network {
	return ae.HostController.GetNetwork()
}

// GetManagers returns the business logic managers.
func (ae *AppEnv) GetManagers() *model.Managers {
	return ae.Managers
}

// GetEventDispatcher returns the event dispatcher for publishing system events.
func (ae *AppEnv) GetEventDispatcher() event.Dispatcher {
	return ae.HostController.GetEventDispatcher()
}

// GetConfig returns the controller configuration.
func (ae *AppEnv) GetConfig() *config.Config {
	return ae.HostController.GetConfig()
}

// GetEnrollmentJwtSigner returns as Signer to use for enrollments based on the edge.api.address hostname
// or an error if one cannot be located that matches. Hostname matching is done across all identity server
// certificates, including alternate server certificates.
func (ae *AppEnv) GetEnrollmentJwtSigner() (jwtsigner.Signer, error) {
	enrollmentCert, err := ae.getEnrollmentTlsCert()

	if err != nil {
		return nil, fmt.Errorf("could not determine enrollment signer: %w", err)
	}

	var signMethod jwt.SigningMethod
	signMethod, err = jwtsigner.GetJwtSigningMethod(enrollmentCert)

	if err != nil {
		return nil, fmt.Errorf("could not determine enrollment signer: %w", err)
	}

	kid := fmt.Sprintf("%x", sha1.Sum(enrollmentCert.Certificate[0]))
	return jwtsigner.New(signMethod, enrollmentCert.PrivateKey, kid), nil
}

// getEnrollmentTlsCert finds the TLS certificate that matches the edge API address hostname.
func (ae *AppEnv) getEnrollmentTlsCert() (*tls.Certificate, error) {
	host, _, err := net.SplitHostPort(ae.GetConfig().Edge.Api.Address)

	var hostnameErrors []error

	if err != nil {
		return nil, fmt.Errorf("could not parse edge.api.address for host and port during enrollment signer selection [%s]", ae.GetConfig().Edge.Api.Address)
	}

	var tlsCert *tls.Certificate

	//look at xweb instances and search
	for _, serverConfig := range ae.GetHostController().GetXWebInstance().GetConfig().ServerConfigs {
		clientApiFound := false
		for _, curApi := range serverConfig.APIs {
			if curApi.Binding() == ClientApiBinding {
				clientApiFound = true
			}
		}

		if clientApiFound {
			tlsCert, err = ae.getCertForHostname(serverConfig.Identity.ServerCert(), host)

			if err != nil {
				hostnameErrors = append(hostnameErrors, err)
				continue
			}

			if tlsCert != nil {
				return tlsCert, nil
			}
		}
	}

	//default to root
	tlsCert, err = ae.getCertForHostname(ae.GetConfig().Id.ServerCert(), host)

	if err == nil {
		return tlsCert, nil
	}

	hostnameErrors = append(hostnameErrors, err)

	pfxlog.Logger().WithField("hostnameErrors", hostnameErrors).Errorf("could not find a server certificate for the edge.api.address host [%s]", host)

	return nil, fmt.Errorf("could not find a configured server certificate that matches hostname [%s] in root controller identity nor in xweb identities", host)
}

// getCertForHostname searches for a certificate that can verify the given hostname.
func (ae *AppEnv) getCertForHostname(tlsCerts []*tls.Certificate, hostname string) (*tls.Certificate, error) {
	for i, tlsCert := range tlsCerts {
		if tlsCert.Leaf == nil {
			if len(tlsCert.Certificate) > 0 {
				var err error
				tlsCert.Leaf, err = x509.ParseCertificate(tlsCert.Certificate[0])

				if err != nil {
					pfxlog.Logger().Warnf("failed to parse leading certificate in a tls configuration while determining enrollment certificate, entry at index %d is skipped, processing other certificates: %s", i, err)
					continue
				}
			}
		}

		if tlsCert.Leaf.VerifyHostname(hostname) == nil {
			return tlsCert, nil
		}
	}

	return nil, fmt.Errorf("could not find a configured server certificate that matches hostname [%s]", hostname)
}

// GetDb returns the database instance.
func (ae *AppEnv) GetDb() boltz.Db {
	return ae.HostController.GetDb()
}

// GetStores returns the database stores.
func (ae *AppEnv) GetStores() *db.Stores {
	return ae.Stores
}

// GetAuthRegistry returns the authentication module registry.
func (ae *AppEnv) GetAuthRegistry() model.AuthRegistry {
	return ae.AuthRegistry
}

// GetEnrollRegistry returns the enrollment handler registry.
func (ae *AppEnv) GetEnrollRegistry() model.EnrollmentRegistry {
	return ae.EnrollRegistry
}

// IsEdgeRouterOnline checks if an edge router is currently connected.
func (ae *AppEnv) IsEdgeRouterOnline(id string) bool {
	return ae.Broker.IsEdgeRouterOnline(id)
}

// GetMetricsRegistry returns the metrics registry for collecting performance data.
func (ae *AppEnv) GetMetricsRegistry() metrics.Registry {
	return ae.HostController.GetMetricsRegistry()
}

// GetFingerprintGenerator returns the certificate fingerprint generator.
func (ae *AppEnv) GetFingerprintGenerator() cert.FingerprintGenerator {
	return ae.FingerprintGenerator
}

// GetRaftInfo returns Raft cluster information (node ID, leader, cluster state).
func (ae *AppEnv) GetRaftInfo() (string, string, string) {
	return ae.HostController.GetRaftInfo()
}

// GetApiAddresses returns the controller's API addresses and their fingerprint hash.
func (ae *AppEnv) GetApiAddresses() (map[string][]event.ApiAddress, []byte) {
	return ae.HostController.GetApiAddresses()
}

// GetCloseNotifyChannel returns a channel that signals when the controller is shutting down.
func (ae *AppEnv) GetCloseNotifyChannel() <-chan struct{} {
	return ae.HostController.GetCloseNotifyChannel()
}

// GetPeerSigners returns the certificates of peer controllers for signature verification.
func (ae *AppEnv) GetPeerSigners() []*x509.Certificate {
	return ae.HostController.GetPeerSigners()
}

// GetCommandDispatcher returns the command dispatcher for processing control plane commands.
func (ae *AppEnv) GetCommandDispatcher() command.Dispatcher {
	return ae.HostController.GetCommandDispatcher()
}

// AddRouterPresenceHandler registers a handler for router connect/disconnect events.
func (ae *AppEnv) AddRouterPresenceHandler(h model.RouterPresenceHandler) {
	ae.HostController.GetNetwork().AddRouterPresenceHandler(h)
}

// GetId returns the unique application identifier for this controller instance.
func (ae *AppEnv) GetId() string {
	return ae.HostController.GetNetwork().GetAppId()
}

func (ae *AppEnv) HandleServicePolicyChange(ctx boltz.MutateContext, policyChange *edge_ctrl_pb.DataState_ServicePolicyChange) {
	ae.Broker.GetRouterSyncStrategy().HandleServicePolicyChange(ctx, policyChange)
}

type HostController interface {
	GetConfig() *config.Config
	GetEnv() *AppEnv
	RegisterAgentBindHandler(bindHandler channel.BindHandler)
	RegisterXctrl(x xctrl.Xctrl) error
	RegisterXmgmt(x xmgmt.Xmgmt) error
	GetXWebInstance() xweb.Instance
	GetNetwork() *network.Network
	GetCloseNotifyChannel() <-chan struct{}
	Shutdown()
	Identity() identity.Identity
	IsRaftEnabled() bool
	IsRaftLeader() bool
	GetStartRaftIndex() uint64
	GetDb() boltz.Db
	GetCommandDispatcher() command.Dispatcher
	GetPeerSigners() []*x509.Certificate
	GetEventDispatcher() event.Dispatcher
	GetRaftIndex() uint64
	GetPeerAddresses() []string
	GetRaftInfo() (string, string, string)
	GetApiAddresses() (map[string][]event.ApiAddress, []byte)
	GetMetricsRegistry() metrics.Registry
}

type Schemes struct {
	Association             *BasicEntitySchema
	Authenticator           *BasicEntitySchema
	AuthenticatorSelf       *BasicEntitySchema
	Ca                      *BasicEntitySchema
	Config                  *BasicEntitySchema
	ConfigType              *BasicEntitySchema
	Enroller                *BasicEntitySchema
	EnrollEr                *BasicEntitySchema
	EnrollUpdb              *BasicEntitySchema
	EdgeRouter              *BasicEntitySchema
	EdgeRouterPolicy        *BasicEntitySchema
	TransitRouter           *BasicEntitySchema
	Identity                *IdentityEntitySchema
	Service                 *BasicEntitySchema
	ServiceEdgeRouterPolicy *BasicEntitySchema
	ServicePolicy           *BasicEntitySchema
	Session                 *BasicEntitySchema
	Terminator              *BasicEntitySchema
}

func (s Schemes) GetEnrollErPost() *gojsonschema.Schema {
	return s.EnrollEr.Post
}

func (s Schemes) GetEnrollUpdbPost() *gojsonschema.Schema {
	return s.EnrollUpdb.Post
}

type IdentityEntitySchema struct {
	Post           *gojsonschema.Schema
	Patch          *gojsonschema.Schema
	Put            *gojsonschema.Schema
	ServiceConfigs *gojsonschema.Schema
}

type BasicEntitySchema struct {
	Post  *gojsonschema.Schema
	Patch *gojsonschema.Schema
	Put   *gojsonschema.Schema
}

type AppHandler func(ae *AppEnv, rc *response.RequestContext)

type AppMiddleware func(*AppEnv, http.Handler) http.Handler

type authorizer struct {
}

const (
	EventualEventsGauge = "eventual.events"
)

// Authorize is a simple authorizer that acts as a front door to "authenticated endpoints". If no API Session is found
// in the current security context, there is no point in proceeding.
func (a authorizer) Authorize(request *http.Request, _ interface{}) error {
	rc, err := GetRequestContextFromHttpContext(request)

	if err != nil {
		return errorz.NewUnauthorized()
	}

	if rc == nil {
		return errorz.NewUnauthorized()
	}

	if rc.SecurityCtx == nil {
		return errorz.NewUnauthorized()
	}

	apiSession, err := rc.SecurityCtx.GetApiSession()

	if err != nil {
		return err
	}

	if apiSession == nil {
		return errorz.NewUnauthorized()
	}

	if apiSession.Identity == nil {
		return errorz.NewUnauthorized()
	}

	if apiSession.Identity.Disabled {
		return errorz.NewUnauthorized()
	}

	return nil
}

func NewAuthQueryZitiTotp() *rest_model.AuthQueryDetail {
	provider := rest_model.MfaProvidersZiti
	return &rest_model.AuthQueryDetail{
		TypeID:     rest_model.AuthQueryTypeMFA,
		Format:     rest_model.MfaFormatsAlphaNumeric,
		HTTPMethod: http.MethodPost,
		HTTPURL:    "./authenticate/mfa",
		MaxLength:  model.TotpMaxLength,
		MinLength:  model.TotpMinLength,
		Provider:   &provider,
	}
}

func NewAuthQueryExtJwt(signer *model.ExternalJwtSigner) *rest_model.AuthQueryDetail {
	provider := rest_model.MfaProvidersURL

	if signer == nil {
		return &rest_model.AuthQueryDetail{
			TypeID:   rest_model.AuthQueryTypeEXTDashJWT,
			Provider: &provider,
		}
	}

	return &rest_model.AuthQueryDetail{
		HTTPURL:  stringz.OrEmpty(signer.ExternalAuthUrl),
		TypeID:   rest_model.AuthQueryTypeEXTDashJWT,
		Provider: &provider,
		Scopes:   signer.Scopes,
		ClientID: stringz.OrEmpty(signer.ClientId),
		ID:       signer.Id,
	}
}

func NewAppEnv(host HostController) (*AppEnv, error) {
	var signingCert *x509.Certificate
	cfg := host.GetConfig()

	if cfg.Edge != nil && cfg.Edge.Enrollment.SigningCert != nil {
		signingCert = host.GetConfig().Edge.Enrollment.SigningCert.Cert().Leaf
	}

	stores, err := db.InitStores(host.GetDb(), host.GetCommandDispatcher().GetRateLimiter(), signingCert)
	if err != nil {
		return nil, err
	}

	fabricManagementSpec, err := loads.Embedded(fabricServer.SwaggerJSON, fabricServer.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}

	clientSpec, err := loads.Embedded(clientServer.SwaggerJSON, clientServer.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}

	managementSpec, err := loads.Embedded(managementServer.SwaggerJSON, managementServer.FlatSwaggerJSON)
	if err != nil {
		pfxlog.Logger().Fatalln(err)
	}

	fabricApi := fabricOperations.NewZitiFabricAPI(fabricManagementSpec)
	fabricApi.ServeError = ServeError

	clientApi := clientOperations.NewZitiEdgeClientAPI(clientSpec)
	clientApi.ServeError = ServeError

	managementApi := managementOperations.NewZitiEdgeManagementAPI(managementSpec)
	managementApi.ServeError = ServeError

	c := host.GetConfig().Edge

	timelineMode := boltz.TimelineModeDefault
	if !host.GetConfig().IsRaftEnabled() {
		timelineMode = boltz.TimelineModeInitIfEmpty
	}
	timelineId, err := host.GetConfig().Db.GetTimelineId(timelineMode, shortid.Generate)
	if err != nil {
		return nil, err
	}

	ae := &AppEnv{
		Stores: stores,
		Versions: &ziti.Versions{
			Api:           "1.0.0",
			EnrollmentApi: "1.0.0",
		},
		HostController:     host,
		InstanceId:         cuid.New(),
		AuthRegistry:       &model.AuthProcessorRegistryImpl{},
		EnrollRegistry:     &model.EnrollmentRegistryImpl{},
		FabricApi:          fabricApi,
		ManagementApi:      managementApi,
		ClientApi:          clientApi,
		IdentityRefreshMap: cmap.New[time.Time](),
		StartupTime:        time.Now().UTC(),
		AuthRateLimiter: command.NewAdaptiveRateLimiter(command.AdaptiveRateLimiterConfig{
			Enabled:          c.AuthRateLimiter.Enabled,
			MinSize:          c.AuthRateLimiter.MinSize,
			MaxSize:          c.AuthRateLimiter.MaxSize,
			WorkTimerMetric:  metricAuthLimiterWorkTimer,
			QueueSizeMetric:  metricAuthLimiterCurrentQueuedCount,
			WindowSizeMetric: metricAuthLimiterCurrentWindowSize,
		}, host.GetMetricsRegistry(), host.GetCloseNotifyChannel()),
		TraceManager: NewTraceManager(host.GetCloseNotifyChannel()),
	}

	ae.timelineId.Store(timelineId)

	ae.identityRefreshMeter = host.GetMetricsRegistry().Meter("identity.refresh")

	clientApi.APIAuthorizer = authorizer{}
	managementApi.APIAuthorizer = authorizer{}

	noOpConsumer := runtime.ConsumerFunc(func(reader io.Reader, data interface{}) error {
		return nil //do nothing
	})

	//enrollment consumer, leave content unread, allow modules to read
	clientApi.ApplicationXPemFileConsumer = noOpConsumer
	clientApi.ApplicationPkcs7Consumer = noOpConsumer
	clientApi.TextYamlProducer = &YamlProducer{}

	clientApi.Oauth2Auth = func(token string, scopes []string) (principal interface{}, err error) {
		return token, nil
	}

	clientApi.ZtSessionAuth = func(token string) (principal interface{}, err error) {
		return token, nil
	}

	managementApi.TextYamlProducer = &YamlProducer{}
	managementApi.ZtSessionAuth = clientApi.ZtSessionAuth
	managementApi.Oauth2Auth = clientApi.Oauth2Auth

	fabricApi.APIAuthorizer = authorizer{}
	fabricApi.ZtSessionAuth = clientApi.ZtSessionAuth
	fabricApi.Oauth2Auth = clientApi.Oauth2Auth

	if host.GetConfig().Edge.Enabled {
		enrollmentCert := host.GetConfig().Edge.Enrollment.SigningCert.Cert()
		ae.ApiClientCsrSigner = cert.NewClientSigner(enrollmentCert.Leaf, enrollmentCert.PrivateKey)
		ae.ApiServerCsrSigner = cert.NewServerSigner(enrollmentCert.Leaf, enrollmentCert.PrivateKey)
		ae.ControlClientCsrSigner = cert.NewClientSigner(enrollmentCert.Leaf, enrollmentCert.PrivateKey)
	}

	ae.FingerprintGenerator = cert.NewFingerprintGenerator()

	ae.Managers = model.NewManagers()
	ae.Managers.Init(ae)

	ae.TokenIssuerCache = model.NewTokenIssuerCache(ae)

	return ae, nil
}

func (ae *AppEnv) InitPersistence() error {
	var err error

	stores := ae.GetStores()

	stores.EventualEventer.AddListener(db.EventualEventAddedName, func(i ...interface{}) {
		if len(i) == 0 {
			pfxlog.Logger().Errorf("could not update metrics for %s gauge on add, event argument length was 0", EventualEventsGauge)
			return
		}

		if addEvent, ok := i[0].(*db.EventualEventAdded); ok {
			gauge := ae.GetHostController().GetMetricsRegistry().Gauge(EventualEventsGauge)
			gauge.Update(addEvent.Total)
		} else {
			pfxlog.Logger().Errorf("could not update metrics for %s gauge on add, event argument was %T expected *EventualEventAdded", EventualEventsGauge, i[0])
		}
	})
	stores.EventualEventer.AddListener(db.EventualEventRemovedName, func(i ...interface{}) {
		if len(i) == 0 {
			pfxlog.Logger().Errorf("could not update metrics for %s gauge on remove, event argument length was 0", EventualEventsGauge)
			return
		}

		if removeEvent, ok := i[0].(*db.EventualEventRemoved); ok {
			gauge := ae.GetHostController().GetMetricsRegistry().Gauge(EventualEventsGauge)
			gauge.Update(removeEvent.Total)
		} else {
			pfxlog.Logger().Errorf("could not update metrics for %s gauge on remove, event argument was %T expected *EventualEventRemoved", EventualEventsGauge, i[0])
		}
	})

	ae.GetHostController().GetEventDispatcher().(*events.Dispatcher).InitializeEdgeEvents(stores)

	db.ServiceEvents.AddServiceEventHandler(ae.HandleServiceEvent)
	stores.Identity.AddEntityIdListener(ae.IdentityRefreshMap.Remove, boltz.EntityDeletedAsync)

	return err
}

// ControllersKeyFunc provides public keys for JWT token verification from peer controllers.
func (ae *AppEnv) ControllersKeyFunc(token *jwt.Token) (interface{}, error) {
	kidVal, ok := token.Header["kid"]

	if !ok {
		return nil, nil
	}

	kid, ok := kidVal.(string)

	if !ok {
		return nil, nil
	}

	key := ae.GetControllerPublicKey(kid)

	if key == nil {
		return nil, fmt.Errorf("key for kid %s, not found", kid)
	}

	return key, nil
}

// GetControllerPublicKey retrieves a public key by key ID from peer controllers.
func (ae *AppEnv) GetControllerPublicKey(kid string) crypto.PublicKey {
	signers := ae.Broker.GetPublicKeys()
	return signers[kid]
}

// CreateRequestContext creates a new request context for handling HTTP requests.
func (ae *AppEnv) CreateRequestContext(rw http.ResponseWriter, r *http.Request) (*response.RequestContext, error) {
	rid := eid.New()

	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))

	securityTokenCtx, err := common.NewSecurityTokenCtx(r, ae.TokenIssuerCache)

	if err != nil {
		return nil, fmt.Errorf("could not create security token context: %w", err)
	}

	securityTokenCtx.AddToRequest(r)

	securityCtx := NewSecurityCtx(securityTokenCtx, ae)
	securityCtx.AddToRequest(r)

	requestContext := &response.RequestContext{
		Id:             rid,
		ResponseWriter: rw,
		Request:        r,
		Body:           body,
		StartTime:      time.Now(),
		SecurityCtx:    securityCtx,
	}

	requestContext.Responder = response.NewResponder(requestContext)

	return requestContext, nil
}

func GetRequestContextFromHttpContext(r *http.Request) (*response.RequestContext, error) {
	val := r.Context().Value(api.ZitiContextKey)
	if val == nil {
		return nil, fmt.Errorf("value for key %s no found in context", api.ZitiContextKey)
	}

	requestContext := val.(*response.RequestContext)

	if requestContext == nil {
		return nil, fmt.Errorf("value for key %s is not a request context", api.ZitiContextKey)
	}

	return requestContext, nil
}

// getMetricTimerName returns a metric timer name based on the incoming HTTP request's URL and method.
// Unique ids are removed from the URL and replaced with :id and :subid to group metrics from the same
// endpoint that happen to be working on different ids.
func getMetricTimerName(r *http.Request) string {
	cleanUrl := r.URL.Path

	rc, _ := api.GetRequestContextFromHttpContext(r)

	if rc != nil {
		if id, err := rc.GetEntityId(); err == nil && id != "" {
			cleanUrl = strings.ReplaceAll(cleanUrl, id, ":id")
		}

		if subId, err := rc.GetEntitySubId(); err == nil && subId != "" {
			cleanUrl = strings.ReplaceAll(cleanUrl, subId, ":subid")
		}
	}

	return fmt.Sprintf("%s.%s", cleanUrl, r.Method)
}

func (ae *AppEnv) InitPermissionsContext(request *http.Request, api permissions.Api, entityType string, action permissions.Action) {
	if rc, _ := GetRequestContextFromHttpContext(request); rc != nil {
		rc.InitPermissionsContext(api, entityType, action)
	}
}

// IsAllowed creates a middleware responder that checks permissions before executing the handler.
func (ae *AppEnv) IsAllowed(responderFunc func(ae *AppEnv, rc *response.RequestContext), request *http.Request, entityId string, entitySubId string, permissions ...permissions.Resolver) openApiMiddleware.Responder {
	return openApiMiddleware.ResponderFunc(func(writer http.ResponseWriter, producer runtime.Producer) {

		rc, err := GetRequestContextFromHttpContext(request)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not retrieve request context")

			if rc != nil {
				rc.RespondWithError(err)
			} else {
				WriteHttpError(writer, err)
			}

			return
		}

		if rc == nil {
			pfxlog.Logger().Error("request context was nil")
			WriteHttpApiError(writer, errorz.NewUnhandled(errors.New("request context was nil")))
			return
		}

		rc.SetProducer(producer)
		rc.SetEntityId(entityId)
		rc.SetEntitySubId(entitySubId)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("could not retrieve request context")
			rc.RespondWithError(err)
			return
		}

		for _, permission := range permissions {
			if !permission.IsAllowed(rc) {
				var unauthorizedError error = errorz.NewUnauthorized()

				securityErr := rc.SecurityCtx.GetError()
				if securityErr != nil {
					// if we have a specific security error, use that instead
					unauthorizedError = securityErr
				}

				rc.RespondWithError(unauthorizedError)
				return
			}
		}

		responderFunc(ae, rc)

		if !rc.StartTime.IsZero() {
			timer := ae.GetHostController().GetMetricsRegistry().Timer(getMetricTimerName(rc.Request))
			timer.UpdateSince(rc.StartTime)
		} else {
			pfxlog.Logger().WithFields(map[string]interface{}{
				"url": request.URL,
			}).Warn("could not mark metrics for REST ApiConfig endpoint, request context start time is zero")
		}

		if apiSession, _ := rc.SecurityCtx.GetApiSession(); apiSession != nil {
			connectEvent := &event.ConnectEvent{
				Namespace: event.ConnectEventNS,
				SrcType:   event.ConnectSourceIdentity,
				DstType:   event.ConnectDestinationController,
				SrcId:     apiSession.IdentityId,
				SrcAddr:   rc.Request.RemoteAddr,
				DstId:     ae.HostController.GetNetwork().GetAppId(),
				DstAddr:   rc.Request.Host,
				Timestamp: time.Now(),
			}
			ae.GetEventDispatcher().AcceptConnectEvent(connectEvent)
		}
	})
}

// HandleServiceEvent processes service change events and triggers identity refreshes.
func (ae *AppEnv) HandleServiceEvent(event *db.ServiceEvent) {
	ae.HandleServiceUpdatedEventForIdentityId(event.IdentityId)
}

// HandleServiceUpdatedEventForIdentityId marks an identity for refresh due to service changes.
func (ae *AppEnv) HandleServiceUpdatedEventForIdentityId(identityId string) {
	ae.IdentityRefreshMap.Set(identityId, time.Now().UTC())
	ae.identityRefreshMeter.Mark(1)
}

// SetClientApiDefaultCertificate configures the default JWT signer for client API operations.
func (ae *AppEnv) SetClientApiDefaultCertificate(serverCert *tls.Certificate) {
	newSigner := &jwtsigner.TlsJwtSigner{}
	err := newSigner.Set(serverCert)

	if err != nil {
		pfxlog.Logger().WithError(err).Panic("could not set default client api certificate")
	}

	ae.clientApiDefaultSigner = newSigner

}

// OidcIssuer returns the OIDC issuer URL for this controller.
func (ae *AppEnv) OidcIssuer() string {
	return ae.RootIssuer() + "/oidc"
}

// RootIssuer returns the base issuer URL for this controller.
func (ae *AppEnv) RootIssuer() string {
	return "https://" + ae.GetConfig().Edge.Api.Address
}

// InitTimelineId sets the timeline ID during startup, panics if already set.
func (ae *AppEnv) InitTimelineId(timelineId string) {
	if ae.timelineId.Load() == "" {
		ae.timelineId.Store(timelineId)
	} else {
		panic(errors.New("timelineId initialization attempted after startup"))
	}
}

// OverrideTimelineId forcibly sets the timeline ID bypassing startup checks.
func (ae *AppEnv) OverrideTimelineId(timelineId string) {
	ae.timelineId.Store(timelineId)
}

// TimelineId returns the current timeline identifier for event ordering.
func (ae *AppEnv) TimelineId() string {
	return ae.timelineId.Load()
}

// WriteHttpApiError is meant to be used in situations where no request context is available to provide responses.
func WriteHttpApiError(w http.ResponseWriter, apiError *errorz.ApiError) {
	producer := runtime.JSONProducer()
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(apiError.Status)
	_ = producer.Produce(w, apiError)
}

// WriteHttpError is meant to be used in situations where no request context is available to provide responses.
func WriteHttpError(w http.ResponseWriter, err error) {
	if err == nil {
		err = errors.New("unknown error")
	}

	var apiErr *errorz.ApiError
	ok := errors.As(err, &apiErr)

	if ok && apiErr != nil {
		WriteHttpApiError(w, apiErr)
		return
	}

	apiErr = errorz.NewUnhandled(err)
	WriteHttpApiError(w, apiErr)
}
