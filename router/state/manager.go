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

package state

import (
	"bufio"
	"crypto"
	"crypto/x509"
	"fmt"
	"math/rand"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/go-events"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/goroutines"
	"github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/metrics"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/common/runner"
	"github.com/openziti/ziti/controller/oidc_auth"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/posture"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

const (
	EventRemovedEdgeSession = "RemovedEdgeSession"

	EventAddedApiSession   = "AddedApiSession"
	EventUpdatedApiSession = "UpdatedApiSession"
	EventRemovedApiSession = "RemovedApiSession"

	RouterDataModelListerBufferSize = 100
	DefaultSubscriptionTimeout      = 5 * time.Minute
)

type RemoveListener func()

// ConnState encapsulates the authentication and authorization context for an
// edge connection, bundling API session credentials, service-specific tokens,
// and policy enforcement metadata for streamlined access control decisions.
type ConnState struct {
	ApiSessionToken     *ApiSessionToken
	ServiceSessionToken *ServiceSessionToken
	PolicyType          edge_ctrl_pb.PolicyType
}

// ConnProvider is an interface used to abstract specific conn implementations from lower level packages
// (such as xgress_edge) to avoid circular dependencies.
type ConnProvider interface {
	GetConnIdToSinks() map[uint32]edge.MsgSink[*ConnState]
	CloseConn(connId uint32, reason string) error
}

// ApiSessionTokenProvider abstracts access to API session tokens, enabling
// decoupled token retrieval across different components while maintaining
// consistent authentication context access patterns.
type ApiSessionTokenProvider interface {
	GetApiSessionToken() *ApiSessionToken
}

// Manager provides the central interface for router state management, encompassing
// session lifecycle, authentication token processing, posture monitoring, and
// distributed synchronization with controllers. This interface abstracts the
// complex state coordination required for zero-trust network operation.
//
// The Manager handles the evolution from legacy protobuf-based sessions to modern
// JWT-based authentication while maintaining backwards compatibility. It coordinates
// between multiple concerns: session validation, policy enforcement, network topology
// updates, and connection lifecycle management.
//
// Key responsibilities include:
// - Session token parsing and validation for both JWT and legacy formats
// - Real-time posture data processing and policy evaluation
// - Distributed state synchronization with controller clusters
// - Connection authorization and cleanup coordination
// - Certificate validation and public key management
type Manager interface {
	env.Xrctrl

	// ParseServiceSessionJwt validates and extracts service session tokens from JWT
	// strings, ensuring cryptographic integrity and claim validation while binding
	// the service session to its parent API session context.
	ParseServiceSessionJwt(jwtStr string, apiSessionToken *ApiSessionToken) (*ServiceSessionToken, error)

	// GetServiceSessionToken creates service session tokens from either JWT strings
	// or service IDs, providing a unified interface that handles both modern JWT-based
	// service access and legacy service ID lookups from the router data model.
	GetServiceSessionToken(serviceToken string, apiSessionToken *ApiSessionToken) (*ServiceSessionToken, error)

	// RemoveLegacyServiceSession handles the controlled termination of service sessions
	// that have been invalidated by controller policy changes or expiration.
	RemoveLegacyServiceSession(serviceSessionToken *ServiceSessionToken)

	// AddLegacyServiceSessionRemovedListener enables connection cleanup coordination
	// by notifying components when service sessions become invalid.
	AddLegacyServiceSessionRemovedListener(serviceSessionToken *ServiceSessionToken, callBack func(serviceSessionToken *ServiceSessionToken)) RemoveListener

	// WasLegacyServiceSessionRecentlyRemoved prevents spurious connection attempts
	// by tracking recently invalidated sessions.
	WasLegacyServiceSessionRecentlyRemoved(token string) bool

	// MarkLegacyServiceSessionRecentlyRemoved adds session invalidation timestamps
	// to enable fast-path rejection of connection attempts using recently removed sessions.
	MarkLegacyServiceSessionRecentlyRemoved(token string)

	// ParseApiSessionJwt validates and extracts API session tokens from JWT strings,
	// performing cryptographic signature verification and audience validation to
	// ensure token authenticity and proper scope for Ziti network access.
	ParseApiSessionJwt(jwtStr string) (*ApiSessionToken, error)

	// GetApiSessionToken retrieves API session tokens from either JWT strings or
	// legacy token lookups, abstracting the underlying token format from callers.
	GetApiSessionToken(apiSessionToken string) *ApiSessionToken

	// GetApiSessionTokenWithTimeout implements eventual consistency for session lookups
	// during the window between session creation notification and local cache population.
	GetApiSessionTokenWithTimeout(token string, timeout time.Duration) *ApiSessionToken

	// AddApiSessionRemovedListener provides event notification for API session
	// invalidation, enabling dependent resources to perform coordinated cleanup.
	AddApiSessionRemovedListener(apiSessionToken *ApiSessionToken, callBack func(*ApiSessionToken)) RemoveListener

	// AddLegacyApiSession registers controller-synchronized API sessions in the legacy
	// tracking store, specifically handling protobuf-based sessions.
	AddLegacyApiSession(apiSession *ApiSessionToken)

	// UpdateLegacyApiSession refreshes controller-synchronized API session state
	// for legacy sessions, maintaining backwards compatibility.
	UpdateLegacyApiSession(apiSession *ApiSessionToken)

	// RemoveLegacyApiSession removes controller-synchronized API sessions from
	// legacy tracking stores.
	RemoveLegacyApiSession(apiSession *ApiSessionToken)

	// RemoveMissingApiSessions reconciles router session state with controller state
	// by removing sessions not present in the authoritative controller list.
	RemoveMissingApiSessions(knownSessions []*ApiSessionToken, beforeSessionId string)

	// RouterDataModel returns the current router data model containing
	// network topology and policy information.
	RouterDataModel() *common.RouterDataModel

	// SetRouterDataModel replaces the current router data model with a new one,
	// optionally resetting the controller subscription.
	SetRouterDataModel(model *common.RouterDataModel, resetSubscription bool)

	// GetRouterDataModelPool returns the goroutine pool used for processing
	// router data model events and updates.
	GetRouterDataModelPool() goroutines.Pool

	// StartHeartbeat initiates periodic transmission of active legacy session tokens
	// to controllers, enabling distributed session state synchronization.
	StartHeartbeat(env env.RouterEnv, seconds int, closeNotify <-chan struct{})

	// ValidateSessions performs batch validation of active service sessions against
	// controller state, enabling detection of sessions that have been revoked or expired.
	ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration)

	// DumpApiSessions provides diagnostic output of all tracked API sessions
	// for operational debugging and system health monitoring.
	DumpApiSessions(c *bufio.ReadWriter) error

	// MarkSyncInProgress sets the current synchronization tracker ID.
	MarkSyncInProgress(trackerId string)

	// MarkSyncStopped clears the synchronization tracker if it matches the provided ID.
	MarkSyncStopped(trackerId string)

	// IsSyncInProgress returns whether a synchronization operation is currently active.
	IsSyncInProgress() bool

	// VerifyClientCert validates client certificates against the router's trusted
	// certificate authorities.
	VerifyClientCert(cert *x509.Certificate) error

	// StartRouterModelSave begins periodic saving of the router data model to disk.
	StartRouterModelSave(path string, duration time.Duration)

	// LoadRouterModel initializes the router data model from a saved file,
	// falling back to an empty model if the file doesn't exist.
	LoadRouterModel(filePath string)

	// ProcessPostureResponses handles incoming posture data from SDK clients and updates
	// the router's posture cache.
	ProcessPostureResponses(ch channel.Channel, response *edge_client_pb.PostureResponses)

	// GetEnv returns the router environment instance.
	GetEnv() env.RouterEnv

	// HandleClientApiSessionTokenUpdate propagates JWT token updates to active client
	// connections during token rotation.
	HandleClientApiSessionTokenUpdate(*ApiSessionToken) error

	// GetCurrentDataModelSource returns the ID of the controller currently providing
	// the router data model subscription.
	GetCurrentDataModelSource() string

	// SetConnectionTracker registers the connection tracking implementation with the state manager.
	SetConnectionTracker(tracker ConnectionTracker)

	// HasAccess evaluates whether an identity has access to a service based on
	// current posture data and policy configuration.
	HasAccess(identityId, apiSessionId, serviceId string, policyType edge_ctrl_pb.PolicyType) (*common.ServicePolicy, error)

	// HasDialAccess evaluates service dialing authorization for an identity.
	HasDialAccess(identityId, apiSessionId, serviceId string) (*common.ServicePolicy, error)

	// HasBindAccess evaluates service binding authorization for an identity.
	HasBindAccess(identityId, apiSessionId, serviceId string) (*common.ServicePolicy, error)

	ParseTotpToken(token string) (*common.TotpClaims, error)
}

// ConnectionTracker provides visibility into active channel connections,
// enabling session management operations to locate and interact with
// specific client connections based on identity context.
type ConnectionTracker interface {
	GetChannels() map[string][]channel.Channel
	GetChannelsByIdentityId(identityId string) []channel.Channel
}

var _ Manager = (*ManagerImpl)(nil)

// NewManager creates a new state manager instance with all necessary components
// for session tracking, posture monitoring, and router data model management.
// This is the primary factory function that initializes the complete state
// management infrastructure including goroutine pools, caches, and event handlers.
func NewManager(stateEnv env.RouterEnv) Manager {
	routerDataModelPoolConfig := goroutines.PoolConfig{
		QueueSize:   uint32(1000),
		MinWorkers:  1,
		MaxWorkers:  uint32(1),
		IdleTime:    30 * time.Second,
		CloseNotify: stateEnv.GetCloseNotify(),
		PanicHandler: func(err interface{}) {
			pfxlog.Logger().
				WithField(logrus.ErrorKey, err).
				WithField("backtrace", string(debug.Stack())).Error("panic during router data model event")
		},
		WorkerFunction: routerDataModelWorker,
	}

	metrics.ConfigureGoroutinesPoolMetrics(&routerDataModelPoolConfig, stateEnv.GetMetricsRegistry(), "pool.rdm.handler")

	routerDataModelPool, err := goroutines.NewPool(routerDataModelPoolConfig)
	if err != nil {
		panic(errors.Wrap(err, "error creating rdm goroutine pool"))
	}

	result := &ManagerImpl{
		EventEmmiter:             events.New(),
		legacyApiSessionsByToken: cmap.New[*ApiSessionToken](),
		recentlyRemovedSessions:  cmap.New[time.Time](),
		certCache:                cmap.New[*x509.Certificate](),
		env:                      stateEnv,
		routerDataModelPool:      routerDataModelPool,
		endpointsChanged:         make(chan env.CtrlEvent, 10),
		modelChanged:             make(chan struct{}, 1),
	}
	result.postureCache = posture.NewCache(result)

	result.postureCache.AddUpdateListener(result.onPostureDataUpdate)
	cfg := stateEnv.GetConfig()
	result.LoadRouterModel(stateEnv.GetConfig().Edge.Db)

	stateEnv.GetNetworkControllers().AddChangeListener(env.CtrlEventListenerFunc(func(event env.CtrlEvent) {
		if event.Type != env.ControllerLeaderChange {
			select {
			case result.endpointsChanged <- event:
			default:
			}
		}
	}))

	go result.manageRouterDataModelSubscription()
	result.StartRouterModelSave(cfg.Edge.Db, cfg.Edge.DbSaveInterval)

	return result
}

// onPostureDataUpdate responds to changes in posture data by re-evaluating access policies
// for all connections associated with the affected identity. When posture data changes
// (such as OS updates, domain changes, or MFA status), this function ensures that all
// active connections are still compliant with access policies.
//
// The function iterates through all channels for the identity, examines each connection,
// and re-evaluates access using the updated posture data. Connections that no longer
// meet policy requirements are automatically terminated with appropriate error messages.
//
// Parameters:
//   - data: The updated posture instance data containing current device state
func (sm *ManagerImpl) onPostureDataUpdate(data *posture.InstanceData) {
	rdm := sm.routerDataModel.Load()
	channels := sm.connectionTracker.GetChannelsByIdentityId(data.IdentityId)

	for _, ch := range channels {
		edgeConn, connIdToSink := GetConnProviderAndSinksFromCh(ch)

		for connId, sink := range connIdToSink {
			connState := sink.GetData()

			if connState.ApiSessionToken == nil || connState.ApiSessionToken.Id != data.ApiSessionId {
				continue
			}

			policy, err := posture.HasAccess(rdm, connState.ApiSessionToken.IdentityId, connState.ServiceSessionToken.ServiceId, data, connState.PolicyType)

			var closeErr error
			if err != nil {
				closeErr = edgeConn.CloseConn(connId, fmt.Sprintf("could not determine access, encountered error: %s", err))
			} else if policy == nil {
				closeErr = edgeConn.CloseConn(connId, "access revoked, not granting policies found")
			}

			if closeErr != nil {
				pfxlog.Logger().WithError(err).Error("error closing connection during access check")
			}
		}
	}
}

// HasAccess evaluates whether an identity has access to a service based on
// current posture data and policy configuration, providing comprehensive
// authorization decisions that incorporate real-time device compliance.
func (sm *ManagerImpl) HasAccess(identityId, apiSessionId, serviceId string, policyType edge_ctrl_pb.PolicyType) (*common.ServicePolicy, error) {
	var data *posture.InstanceData
	rdm := sm.routerDataModel.Load()

	instance := sm.postureCache.GetInstance(apiSessionId)

	if instance != nil {
		data = &instance.InstanceData
	}

	return posture.HasAccess(rdm, identityId, serviceId, data, policyType)
}

func routerDataModelWorker(_ uint32, f func()) {
	f()
}

// HasBindAccess evaluates service binding authorization, determining if an
// identity can host connections for a specific service based on policy and posture.
func (sm *ManagerImpl) HasBindAccess(identityId, apiSessionId, serviceId string) (*common.ServicePolicy, error) {
	return sm.HasAccess(identityId, apiSessionId, serviceId, edge_ctrl_pb.PolicyType_BindPolicy)
}

// HasDialAccess evaluates service dialing authorization, determining if an
// identity can initiate connections to a specific service based on policy and posture.
func (sm *ManagerImpl) HasDialAccess(identityId, apiSessionId, serviceId string) (*common.ServicePolicy, error) {
	return sm.HasAccess(identityId, apiSessionId, serviceId, edge_ctrl_pb.PolicyType_DialPolicy)
}

type ManagerImpl struct {
	env env.RouterEnv

	// legacyApiSessionsByToken is a store of legacy API Session delivered to the
	// router from the controller. These tokens are backed by UUIDs and not
	// JWTs.
	legacyApiSessionsByToken cmap.ConcurrentMap[string, *ApiSessionToken]

	// recentlyRemovedSessions tracks sessions that have just been removed and
	// allows the event system to immediately trigger registering callbacks.
	// stored as `id` => `time`. Where `id` and legacy `token` values are the
	// same as the `token` value is no longer used as a secret.
	recentlyRemovedSessions cmap.ConcurrentMap[string, time.Time]

	Hostname       string
	ControllerAddr string
	ClusterId      string
	NodeId         string
	events.EventEmmiter
	heartbeatRunner    runner.Runner
	heartbeatOperation *heartbeatOperation
	currentSync        string
	syncLock           sync.Mutex

	certCache           cmap.ConcurrentMap[string, *x509.Certificate]
	routerDataModel     atomic.Pointer[common.RouterDataModel]
	routerDataModelPool goroutines.Pool

	endpointsChanged    chan env.CtrlEvent
	modelChanged        chan struct{}
	dataModelSubCtrlId  concurrenz.AtomicValue[string]
	dataModelSubTimeout time.Time

	postureCache *posture.Cache

	connectionTracker ConnectionTracker
}

func (sm *ManagerImpl) ParseTotpToken(jwtStr string) (*common.TotpClaims, error) {
	totpClaims := &common.TotpClaims{}
	token, err := jwt.ParseWithClaims(jwtStr, totpClaims, sm.pubKeyLookup)

	if err != nil {
		return nil, err
	}

	if !totpClaims.HasAudience(common.ClaimAudienceOpenZiti) && !totpClaims.HasAudience(common.ClaimLegacyNative) {
		return nil, fmt.Errorf("provided a totp token with invalid audience '%s' of type [%T], expected: %s or %s", totpClaims.Audience, totpClaims.Audience, common.ClaimAudienceOpenZiti, common.ClaimLegacyNative)
	}

	if totpClaims.Type != common.TokenTypeTotp {
		return nil, fmt.Errorf("provided a totp token with invalid type '%s' expected '%s'", totpClaims.Type, common.TokenTypeTotp)
	}

	if !token.Valid {
		return nil, fmt.Errorf("provided totp token that is not valid")
	}

	return totpClaims, nil
}

// SetConnectionTracker registers the connection tracking implementation with the state manager.
// The connection tracker provides the state manager with visibility into active network
// connections, enabling policy enforcement and posture checking operations. This dependency
// injection pattern allows the xgress_edge package to provide connection tracking capabilities
// while avoiding circular dependencies.
//
// Parameters:
//   - tracker: The ConnectionTracker implementation to use for connection queries
func (sm *ManagerImpl) SetConnectionTracker(tracker ConnectionTracker) {
	sm.connectionTracker = tracker
}

// GetCurrentDataModelSource returns the ID of the controller currently providing
// the router data model subscription.
func (self *ManagerImpl) GetCurrentDataModelSource() string {
	return self.dataModelSubCtrlId.Load()
}

// manageRouterDataModelSubscription handles automatic subscription management
// for router data model updates, including controller failover and timeout handling.
func (self *ManagerImpl) manageRouterDataModelSubscription() {
	<-self.env.GetRouterDataModelEnabledConfig().GetInitNotifyChannel()
	if !self.env.IsRouterDataModelEnabled() {
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-self.env.GetCloseNotify():
			return
		case endpointChangedEvent := <-self.endpointsChanged:
			// if the controller we're subscribed to has changed, resubscribe
			if endpointChangedEvent.Controller.Channel().Id() == self.GetCurrentDataModelSource() {
				pfxlog.Logger().WithField("ctrlId", endpointChangedEvent.Controller.Channel().Id()).WithField("change", endpointChangedEvent.Type).
					Info("currently subscribed controller has changed, resubscribing")
				self.dataModelSubCtrlId.Store("")
			}
		case <-ticker.C:
		case <-self.modelChanged:
		}

		allEndpointChangesProcessed := false
		for !allEndpointChangesProcessed {
			select {
			case endpointChangedEvent := <-self.endpointsChanged:
				// if the controller we're subscribed to has changed, resubscribe
				if endpointChangedEvent.Controller.Channel().Id() == self.GetCurrentDataModelSource() {
					pfxlog.Logger().WithField("ctrlId", endpointChangedEvent.Controller.Channel().Id()).WithField("change", endpointChangedEvent.Type).
						Info("currently subscribed controller has changed, resubscribing")
					self.dataModelSubCtrlId.Store("")
				}
			default:
				allEndpointChangesProcessed = true
			}
		}

		self.checkRouterDataModelSubscription()
	}
}

// checkRouterDataModelSubscription verifies subscription health and switches
// controllers when necessary.
func (self *ManagerImpl) checkRouterDataModelSubscription() {
	if !self.env.IsRouterDataModelRequired() {
		return
	}

	ctrl := self.env.GetNetworkControllers().GetNetworkController(self.dataModelSubCtrlId.Load())
	if ctrl == nil || time.Now().After(self.dataModelSubTimeout) {
		if bestCtrl := self.env.GetNetworkControllers().AnyCtrlChannel(); bestCtrl != nil {
			logger := pfxlog.Logger().WithField("ctrlId", bestCtrl.Id()).WithField("prevCtrlId", self.dataModelSubCtrlId.Load())
			if ctrl == nil {
				logger.Info("no current data model subscription active, subscribing")
			} else {
				logger.Info("current data model subscription expired, resubscribing")
			}
			self.subscribeToDataModelUpdates(bestCtrl)
		}
	} else if !ctrl.IsConnected() || ctrl.TimeSinceLastContact() > 30*time.Second {
		bestCtrl := self.env.GetNetworkControllers().AnyCtrlChannel()
		if bestCtrl != nil && bestCtrl.Id() != ctrl.Channel().Id() {
			pfxlog.Logger().WithField("ctrlId", bestCtrl.Id()).
				WithField("prevCtrlId", self.dataModelSubCtrlId.Load()).
				Info("current data model subscription source unreliable, changing subscription")
			self.subscribeToDataModelUpdates(bestCtrl)
		}
	}
}

// subscribeToDataModelUpdates sends subscription requests to controllers
// for router data model changes.
func (self *ManagerImpl) subscribeToDataModelUpdates(ch channel.Channel) {
	renew := self.dataModelSubCtrlId.Load() == ch.Id()

	// if we store after success, we may miss an update because the ids don't match yet
	self.dataModelSubCtrlId.Store(ch.Id())

	var currentIndex uint64
	if rdm := self.routerDataModel.Load(); rdm != nil {
		currentIndex, _ = rdm.CurrentIndex()
	}

	timelineId := ""
	if rdm := self.routerDataModel.Load(); rdm != nil {
		timelineId = rdm.GetTimelineId()
	}

	subTimeout := time.Now().Add(DefaultSubscriptionTimeout)
	req := &edge_ctrl_pb.SubscribeToDataModelRequest{
		CurrentIndex:                currentIndex,
		SubscriptionDurationSeconds: uint32(DefaultSubscriptionTimeout.Seconds()),
		Renew:                       renew,
		TimelineId:                  timelineId,
	}

	logger := pfxlog.Logger().
		WithField("ctrlId", ch.Id()).
		WithField("currentIndex", req.CurrentIndex).
		WithField("renew", req.Renew)

	if err := protobufs.MarshalTyped(req).WithTimeout(self.env.GetNetworkControllers().DefaultRequestTimeout()).SendAndWaitForWire(ch); err != nil {
		self.dataModelSubCtrlId.Store("")
		logger.WithError(err).Error("error to subscribing to router data model changes")
	} else {
		logger.Info("subscribed to new controller for router data model changes")
		self.dataModelSubTimeout = subTimeout
	}
}

// GetRouterDataModelPool returns the goroutine pool used for processing
// router data model events and updates.
func (sm *ManagerImpl) GetRouterDataModelPool() goroutines.Pool {
	return sm.routerDataModelPool
}

// ProcessPostureResponses handles incoming posture data from SDK clients and updates
// the router's posture cache. Posture responses contain information about the client's
// device state such as operating system details, domain membership, MAC addresses,
// running processes, and MFA status. This data is used to evaluate whether the client
// meets policy requirements for network access.
//
// The function extracts identity and session information from the channel's API session
// and stores the posture data in the cache, which triggers automatic evaluation against
// configured posture checks and policy enforcement.
//
// Parameters:
//   - ch: The channel the posture response was received on
//   - response: The posture response containing device state information
func (sm *ManagerImpl) ProcessPostureResponses(ch channel.Channel, responses *edge_client_pb.PostureResponses) {
	apiSessionToken := GetApiSessionTokenFromCh(ch)
	sm.postureCache.AddResponses(apiSessionToken.IdentityId, apiSessionToken.Id, responses)
}

// HandleClientApiSessionTokenUpdate propagates JWT token updates to active client
// connections, ensuring all channels associated with an identity receive the
// refreshed authentication credentials during token rotation.
func (sm *ManagerImpl) HandleClientApiSessionTokenUpdate(newApiSession *ApiSessionToken) error {
	if newApiSession == nil {
		return errors.New("nil api session")
	}

	if newApiSession.Claims == nil {
		return errors.New("nil api session claims")
	}

	if newApiSession.Claims.Type != common.TokenTypeAccess {
		return fmt.Errorf("bearer token is of invalid type: expected %s, got: %s", common.TokenTypeAccess, newApiSession.Claims.Type)
	}

	channels := sm.connectionTracker.GetChannelsByIdentityId(newApiSession.IdentityId)

	for _, ch := range channels {
		anyData := ch.GetUserData()

		if anyData != nil {
			data, ok := anyData.(ApiSessionTokenProvider)

			if !ok || data == nil {
				continue
			}
			apiSession := data.GetApiSessionToken()
			if apiSession != nil && apiSession.Claims.ApiSessionId == newApiSession.Claims.ApiSessionId && apiSession.Claims.Subject == newApiSession.Claims.Subject {
				apiSession.UpdateToken(newApiSession)
			}
		}
	}

	return nil
}

// GetEnv returns the router environment instance.
func (sm *ManagerImpl) GetEnv() env.RouterEnv {
	return sm.env
}

// StartRouterModelSave begins periodic saving of the router data model
// to disk at the specified interval.
func (sm *ManagerImpl) StartRouterModelSave(filePath string, duration time.Duration) {
	go func() {
		for {
			select {
			case <-sm.env.GetCloseNotify():
				return
			case <-time.After(duration):
				sm.RouterDataModel().Save(filePath)
			}
		}
	}()
}

// LoadRouterModel initializes the router data model from a saved file,
// falling back to an empty model if the file doesn't exist.
func (sm *ManagerImpl) LoadRouterModel(filePath string) {
	model, err := common.NewReceiverRouterDataModelFromFile(filePath, RouterDataModelListerBufferSize, sm.env.GetCloseNotify())

	if err != nil {
		if !os.IsNotExist(err) {
			pfxlog.Logger().WithError(err).Errorf("could not load router model from file [%s]", filePath)
		} else {
			pfxlog.Logger().Infof("router data model file does not exist [%s]", filePath)
		}
		model = common.NewReceiverRouterDataModel(RouterDataModelListerBufferSize, sm.env.GetCloseNotify())
	} else {
		index, _ := model.CurrentIndex()
		pfxlog.Logger().WithField("path", filePath).WithField("index", index).Info("loaded router model from file")
	}

	sm.SetRouterDataModel(model, false)
}

// contains is a generic utility function for slice membership testing.
func contains[T comparable](values []T, element T) bool {
	for _, val := range values {
		if val == element {
			return true
		}
	}

	return false
}

// getX509FromData parses and caches X.509 certificates by key ID.
func (sm *ManagerImpl) getX509FromData(kid string, data []byte) (*x509.Certificate, error) {
	if cert, found := sm.certCache.Get(kid); found {
		return cert, nil
	}

	cert, err := x509.ParseCertificate(data)

	if err != nil {
		return nil, err
	}

	sm.certCache.Set(kid, cert)

	return cert, nil
}

// VerifyClientCert validates client certificates against the router's trusted
// certificate authorities, ensuring only properly signed certificates can
// establish authenticated connections.
func (sm *ManagerImpl) VerifyClientCert(cert *x509.Certificate) error {

	rootPool := x509.NewCertPool()

	rdm := sm.routerDataModel.Load()

	for keysTuple := range rdm.PublicKeys.IterBuffered() {
		if contains(keysTuple.Val.Usages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation) {
			cert, err := sm.getX509FromData(keysTuple.Val.Kid, keysTuple.Val.GetData())

			if err != nil {
				pfxlog.Logger().WithField("kid", keysTuple.Val.Kid).WithError(err).Error("could not parse x509 certificate data")
				continue
			}

			rootPool.AddCert(cert)
		}
	}

	opts := x509.VerifyOptions{
		Roots:         rootPool,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		CurrentTime:   cert.NotBefore,
	}

	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("could not verify client certificate %w", err)
	}

	return nil
}

// ParseServiceSessionJwt validates and extracts service session tokens from JWT
// strings, ensuring cryptographic integrity and claim validation while binding
// the service session to its parent API session context.
func (sm *ManagerImpl) ParseServiceSessionJwt(jwtStr string, apiSessionToken *ApiSessionToken) (*ServiceSessionToken, error) {
	serviceAccessClaims := &common.ServiceAccessClaims{}
	jwtToken, err := jwt.ParseWithClaims(jwtStr, serviceAccessClaims, sm.pubKeyLookup)

	if err != nil {
		return nil, err
	}

	if serviceAccessClaims.TokenType == common.TokenTypeServiceAccess {
		return NewServiceSessionToken(jwtToken, serviceAccessClaims, apiSessionToken)
	}

	if !jwtToken.Valid {
		return nil, fmt.Errorf("provided service session token that is not valid")
	}

	return nil, fmt.Errorf("invalid service session token type: %s", serviceAccessClaims.Type)
}

// GetServiceSessionToken creates service session tokens from either JWT strings
// or service IDs, providing a unified interface that handles both modern JWT-based
// service access and legacy service ID lookups from the router data model.
func (sm *ManagerImpl) GetServiceSessionToken(token string, apiSessionToken *ApiSessionToken) (*ServiceSessionToken, error) {
	if strings.HasPrefix(token, oidc_auth.JwtTokenPrefix) {
		serviceSessionToken, err := sm.ParseServiceSessionJwt(token, apiSessionToken)

		if err != nil {
			return nil, fmt.Errorf("failed to create service token from JWT: %w", err)
		}
		return serviceSessionToken, nil
	}

	// if we don't have a service session JWT, it might be by service id instead, hydrate from context
	rdm := sm.routerDataModel.Load()
	service, ok := rdm.Services.Get(token)

	if !ok {
		return nil, fmt.Errorf("unable to get service token either by JWT parsing or id lookup")
	}

	return &ServiceSessionToken{
		ServiceId:       service.GetId(),
		ApiSessionToken: apiSessionToken,
		JwtToken:        nil,
		Claims:          nil,
	}, nil
}

// ParseApiSessionJwt validates and extracts API session tokens from JWT strings,
// performing cryptographic signature verification and audience validation to
// ensure token authenticity and proper scope for Ziti network access.
func (sm *ManagerImpl) ParseApiSessionJwt(jwtStr string) (*ApiSessionToken, error) {
	accessClaims := &common.AccessClaims{}
	jwtToken, err := jwt.ParseWithClaims(jwtStr, accessClaims, sm.pubKeyLookup)

	if err != nil {
		return nil, err
	}

	if !accessClaims.HasAudience(common.ClaimAudienceOpenZiti) && !accessClaims.HasAudience(common.ClaimLegacyNative) {
		return nil, fmt.Errorf("provided an api session token with invalid audience '%s' of type [%T], expected: %s or %s", accessClaims.Audience, accessClaims.Audience, common.ClaimAudienceOpenZiti, common.ClaimLegacyNative)
	}

	if accessClaims.Type != common.TokenTypeAccess {
		return nil, fmt.Errorf("provided an api session token with invalid type '%s'", accessClaims.Type)
	}

	if !jwtToken.Valid {
		return nil, fmt.Errorf("provided an api session token that is not valid")
	}

	return NewApiSessionTokenFromJwt(jwtToken, accessClaims)
}

// pubKeyLookup retrieves public keys for JWT signature verification
// from the router data model.
func (sm *ManagerImpl) pubKeyLookup(token *jwt.Token) (any, error) {
	kidVal, ok := token.Header["kid"]

	if !ok {
		return nil, errors.New("could not lookup JWT signer, kid header missing")
	}

	kid, ok := kidVal.(string)

	if !ok {
		return nil, fmt.Errorf("kid header value is not a string, got type %T", kidVal)
	}

	kid = strings.TrimSpace(kid)

	rdm := sm.routerDataModel.Load()
	publicKeys := rdm.PublicKeys.IterBuffered()
	for keysTuple := range publicKeys {
		if contains(keysTuple.Val.Usages, edge_ctrl_pb.DataState_PublicKey_JWTValidation) {

			if kid == keysTuple.Val.Kid {
				return sm.parsePublicKey(keysTuple.Val)
			}
		}
	}

	return nil, errors.New("public key not found")
}

// RouterDataModel returns the current router data model containing
// network topology and policy information.
func (sm *ManagerImpl) RouterDataModel() *common.RouterDataModel {
	return sm.routerDataModel.Load()
}

// SetRouterDataModel replaces the current router data model with a new one,
// optionally resetting the controller subscription.
func (sm *ManagerImpl) SetRouterDataModel(model *common.RouterDataModel, resetSubscription bool) {
	index, _ := model.CurrentIndex()
	logger := pfxlog.Logger().WithField("index", index)

	publicKeys := model.PublicKeys.Items()
	logger.Debugf("number of public keys in rdm: %d", len(publicKeys))

	if resetSubscription {
		sm.dataModelSubCtrlId.Store("")
	}
	logger.Info("replacing router data model")
	existing := sm.routerDataModel.Swap(model)
	if existing != nil {
		existing.Stop()
		model.InheritLocalData(existing)
		existingIndex, _ := existing.CurrentIndex()
		logger = logger.WithField("existingIndex", existingIndex)
		if index < existingIndex {
			sm.env.GetIndexWatchers().NotifyOfIndexReset()
		}
	}
	model.SyncAllSubscribers()

	if resetSubscription {
		// notify subscription manager code to resubscribe with updated model and index
		select {
		case sm.modelChanged <- struct{}{}:
		default:
		}
	}

	logger.Infof("router data model replacement complete, old: %p, new: %p", existing, model)
}

// MarkSyncInProgress sets the current synchronization tracker ID.
func (sm *ManagerImpl) MarkSyncInProgress(trackerId string) {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	sm.currentSync = trackerId
}

// MarkSyncStopped clears the synchronization tracker if it matches
// the provided ID.
func (sm *ManagerImpl) MarkSyncStopped(trackerId string) {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	if sm.currentSync == trackerId {
		sm.currentSync = ""
	}
}

// IsSyncInProgress returns whether a synchronization operation
// is currently active.
func (sm *ManagerImpl) IsSyncInProgress() bool {
	sm.syncLock.Lock()
	defer sm.syncLock.Unlock()
	return sm.currentSync != ""
}

// AddLegacyApiSession registers controller-synchronized API sessions in the legacy
// tracking store, specifically handling protobuf-based sessions that require
// controller state synchronization rather than self-contained JWT tokens.
func (sm *ManagerImpl) AddLegacyApiSession(apiSessionToken *ApiSessionToken) {
	logger := pfxlog.Logger().Entry
	logger = apiSessionToken.AddLoggingFields(logger)

	if apiSessionToken.Type == ApiSessionTokenLegacyProtobuf {
		logger.Debug("adding legacy api session")
		//for legacy api sessions, token is a UUID
		sm.legacyApiSessionsByToken.Set(apiSessionToken.Token(), apiSessionToken)
		sm.Emit(EventAddedApiSession, apiSessionToken)
	} else {
		logger.Debug("attempted to add a non-legacy api session to legacy tracking")
	}
}

// UpdateLegacyApiSession refreshes controller-synchronized API session state
// for legacy sessions, maintaining backwards compatibility with pre-JWT
// authentication systems that rely on centralized session management.
func (sm *ManagerImpl) UpdateLegacyApiSession(apiSessionToken *ApiSessionToken) {
	logger := pfxlog.Logger().Entry
	logger = apiSessionToken.AddLoggingFields(logger)

	if apiSessionToken.Type == ApiSessionTokenLegacyProtobuf {
		logger.Debug("update legacy api session")

		sm.legacyApiSessionsByToken.Set(apiSessionToken.Token(), apiSessionToken)
		sm.Emit(EventUpdatedApiSession, apiSessionToken)
	} else {
		logger.Debug("attempted to update a non-legacy api session to legacy tracking")
	}
}

// RemoveLegacyApiSession removes controller-synchronized API sessions from
// legacy tracking stores, handling cleanup for sessions that originated from
// pre-JWT authentication systems requiring centralized state management.
func (sm *ManagerImpl) RemoveLegacyApiSession(apiSessionToken *ApiSessionToken) {
	logger := pfxlog.Logger().Entry
	logger = apiSessionToken.AddLoggingFields(logger)

	if ns, ok := sm.legacyApiSessionsByToken.Get(apiSessionToken.Token()); ok {
		logger.Debug("removing legacy api session")
		sm.legacyApiSessionsByToken.Remove(apiSessionToken.Token())
		eventName := apiSessionToken.RemovedEventName()
		sm.Emit(eventName)
		sm.RemoveAllListeners(eventName)
		sm.Emit(EventRemovedApiSession, ns)
	} else {
		logger.Debug("could not remove legacy api session, not found")
	}
}

// RemoveMissingApiSessions removes API Sessions not present in the knownApiSessions argument. If the beforeSessionId
// value is not empty string, it will be used as a monotonic comparison between it and API session ids. API session ids
// later than the sync will be ignored.
// RemoveMissingApiSessions reconciles router session state with controller state
// by removing sessions not present in the authoritative controller list. The
// beforeSessionId parameter enables monotonic comparison to avoid removing sessions
// created after the synchronization snapshot, preventing race conditions during
// high-frequency session creation scenarios.
func (sm *ManagerImpl) RemoveMissingApiSessions(knownApiSessions []*ApiSessionToken, beforeSessionId string) {
	validTokens := map[string]bool{}
	for _, apiSession := range knownApiSessions {
		validTokens[apiSession.Token()] = true
	}

	var tokensToRemove []*ApiSessionToken
	sm.legacyApiSessionsByToken.IterCb(func(token string, apiSessionToken *ApiSessionToken) {
		if _, ok := validTokens[token]; !ok && (beforeSessionId == "" || apiSessionToken.Id <= beforeSessionId) {
			tokensToRemove = append(tokensToRemove, apiSessionToken)
		}
	})

	for _, token := range tokensToRemove {
		sm.RemoveLegacyApiSession(token)
	}
}

// RemoveLegacyServiceSession handles the controlled termination of service sessions
// that have been invalidated by controller policy changes or expiration. This function
// not only removes the session from tracking but also proactively closes all network
// connections using the session, ensuring clients receive immediate notification rather
// than encountering authorization failures on subsequent requests.
func (sm *ManagerImpl) RemoveLegacyServiceSession(serviceSessionToken *ServiceSessionToken) {
	logger := pfxlog.Logger().Entry
	logger = serviceSessionToken.AddLoggingFields(logger)

	logger.Debug("removing network session")

	eventName := serviceSessionToken.RemovedEventName()
	sm.Emit(eventName)
	sm.RemoveAllListeners(eventName)

	activeChannels := sm.connectionTracker.GetChannelsByIdentityId(serviceSessionToken.Claims.IdentityId)

	for _, activeChannel := range activeChannels {
		edgeConn, connIdToSink := GetConnProviderAndSinksFromCh(activeChannel)

		for connId, sink := range connIdToSink {
			if serviceSessionToken.TokenId() == sink.GetData().ServiceSessionToken.TokenId() {
				err := edgeConn.CloseConn(connId, fmt.Sprintf("closing connId %d, legacy service session was removed by controller sync", connId))

				if err != nil {
					logger.WithError(err).WithField("connId", connId).Warnf("failed to close conn")
				}
			}
		}
	}

	sm.recentlyRemovedSessions.Set(serviceSessionToken.TokenId(), time.Now())
}

// GetApiSessionTokenWithTimeout implements eventual consistency for session lookups
// during the window between session creation notification and local cache population.
// This addresses race conditions where clients attempt to use newly created sessions
// before synchronization completes, using exponential backoff to balance responsiveness
// with system load during high session creation rates.
func (sm *ManagerImpl) GetApiSessionTokenWithTimeout(token string, timeout time.Duration) *ApiSessionToken {
	deadline := time.Now().Add(timeout)
	session := sm.GetApiSessionToken(token)

	if session == nil {
		//convert this to return a channel instead of sleeping
		waitTime := time.Millisecond
		for time.Now().Before(deadline) {
			session = sm.GetApiSessionToken(token)
			if session != nil {
				return session
			}
			time.Sleep(waitTime)
			if waitTime < time.Second {
				waitTime = waitTime * 2
			}
		}
	}
	return session
}

// GetApiSessionToken retrieves API session tokens from either JWT strings or
// legacy token lookups, abstracting the underlying token format from callers.
func (sm *ManagerImpl) GetApiSessionToken(token string) *ApiSessionToken {
	if strings.HasPrefix(token, oidc_auth.JwtTokenPrefix) {
		apiSessionToken, err := sm.ParseApiSessionJwt(token)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("failed to create api session from JWT")
			return nil
		}
		return apiSessionToken
	}

	if apiSession, ok := sm.legacyApiSessionsByToken.Get(token); ok {
		return apiSession
	}
	return nil
}

// WasLegacyServiceSessionRecentlyRemoved prevents spurious connection attempts
// by tracking recently invalidated sessions. This addresses the distributed systems
// challenge where session removal notifications may arrive out-of-order or be delayed,
// allowing the router to provide immediate feedback rather than expensive controller
// round-trips for sessions known to be invalid.
func (sm *ManagerImpl) WasLegacyServiceSessionRecentlyRemoved(token string) bool {
	return sm.recentlyRemovedSessions.Has(token)
}

// MarkLegacyServiceSessionRecentlyRemoved adds session invalidation timestamps
// to enable fast-path rejection of connection attempts using recently removed
// sessions. This optimization reduces controller query load and improves client
// error response times during session cleanup scenarios.
func (sm *ManagerImpl) MarkLegacyServiceSessionRecentlyRemoved(token string) {
	sm.recentlyRemovedSessions.Set(token, time.Now())
}

// AddLegacyServiceSessionRemovedListener enables connection cleanup coordination
// by notifying components when service sessions become invalid. This event-driven
// architecture prevents connections from persisting with stale authorization,
// immediately triggering cleanup callbacks when controller synchronization
// indicates session removal.
func (sm *ManagerImpl) AddLegacyServiceSessionRemovedListener(serviceSessionToken *ServiceSessionToken, callBack func(serviceSessionToken *ServiceSessionToken)) RemoveListener {
	// only legacy service sessions will emit these events as newer controllers use JWTs or raw service ids and
	// do not rely on service session syncs from the controller
	if !serviceSessionToken.Claims.IsLegacy {
		return func() {}
	}

	// service session has already been removed, immediately trigger callback
	if sm.recentlyRemovedSessions.Has(serviceSessionToken.TokenId()) {
		go callBack(serviceSessionToken)
		return func() {}
	}

	eventName := serviceSessionToken.RemovedEventName()

	listener := func(args ...interface{}) {
		go callBack(serviceSessionToken)
	}
	sm.AddListener(eventName, listener)

	once := &sync.Once{}
	return func() {
		once.Do(func() {
			go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
		})
	}
}

// AddApiSessionRemovedListener provides event notification for API session
// invalidation, enabling dependent resources to perform coordinated cleanup.
// This decouples session lifecycle management from connection handling logic.
func (sm *ManagerImpl) AddApiSessionRemovedListener(apiSessionToken *ApiSessionToken, callBack func(apiSessionToken *ApiSessionToken)) RemoveListener {
	eventName := apiSessionToken.RemovedEventName()
	listener := func(args ...interface{}) {
		callBack(apiSessionToken)
	}
	sm.AddListener(eventName, listener)

	return func() {
		go sm.RemoveListener(eventName, listener) // likely to be called from Emit, which will cause a deadlock
	}
}

// StartHeartbeat initiates periodic transmission of active legacy session tokens
// to controllers, enabling distributed session state synchronization. This mechanism
// allows controllers to detect router failures and perform session cleanup, while
// also serving as a distributed liveness check for the router infrastructure.
func (sm *ManagerImpl) StartHeartbeat(env env.RouterEnv, intervalSeconds int, closeNotify <-chan struct{}) {
	sm.heartbeatOperation = newHeartbeatOperation(env, time.Duration(intervalSeconds)*time.Second, sm)

	var err error
	sm.heartbeatRunner, err = runner.NewRunner(1*time.Second, 24*time.Hour, func(e error, operation runner.Operation) {
		pfxlog.Logger().WithError(err).Error("error during heartbeat runner")
	})

	if err != nil {
		pfxlog.Logger().WithError(err).Panic("could not create heartbeat runner")
	}

	if err := sm.heartbeatRunner.AddOperation(sm.heartbeatOperation); err != nil {
		pfxlog.Logger().WithError(err).Panic("could not add heartbeat operation to runner")
	}

	if err := sm.heartbeatRunner.Start(closeNotify); err != nil {
		pfxlog.Logger().WithError(err).Panic("could not start heartbeat runner")
	}

	pfxlog.Logger().Info("heartbeat starting")
}

// ActiveServiceSessionTokens gathers all service session tokens from active
// connections, enabling session validation and cleanup operations across
// the router's connection pool.
func (sm *ManagerImpl) ActiveServiceSessionTokens() []*ServiceSessionToken {
	identityToChannels := sm.connectionTracker.GetChannels()

	activeTokens := map[string]*ServiceSessionToken{}

	for _, channels := range identityToChannels {
		for _, ch := range channels {
			if !ch.IsClosed() {
				_, sinks := GetConnProviderAndSinksFromCh(ch)

				for _, sink := range sinks {
					connState := sink.GetData()

					if connState != nil && connState.ServiceSessionToken != nil {
						activeTokens[connState.ServiceSessionToken.TokenId()] = connState.ServiceSessionToken
					}
				}
			}
		}
	}

	result := make([]*ServiceSessionToken, 0, len(activeTokens))
	for _, serviceSessionToken := range activeTokens {
		result = append(result, serviceSessionToken)
	}

	return result
}

// ActiveApiSessionTokens collects all API session tokens currently in use
// across active client connections, providing visibility into live sessions
// for heartbeat and synchronization operations.
func (sm *ManagerImpl) ActiveApiSessionTokens() []*ApiSessionToken {
	identityToChannels := sm.connectionTracker.GetChannels()

	activeTokens := map[string]*ApiSessionToken{}

	for _, channels := range identityToChannels {
		for _, ch := range channels {
			if !ch.IsClosed() {
				apiSessionTokenProvider := GetApiSessionTokenProviderFromCh(ch)

				if apiSessionTokenProvider != nil {
					apiSessionToken := apiSessionTokenProvider.GetApiSessionToken()

					if apiSessionToken != nil {
						activeTokens[apiSessionToken.Id] = apiSessionToken
					}
				}
			}
		}
	}

	result := make([]*ApiSessionToken, 0, len(activeTokens))
	for _, apiSessionToken := range activeTokens {
		result = append(result, apiSessionToken)
	}

	return result
}

// flushRecentlyRemoved cleans up expired entries from the recently
// removed sessions tracking cache.
func (sm *ManagerImpl) flushRecentlyRemoved() {
	now := time.Now()
	var toRemove []string
	sm.recentlyRemovedSessions.IterCb(func(key string, t time.Time) {
		remove := false

		if now.Sub(t) >= 5*time.Minute {
			remove = true
		}

		if remove {
			toRemove = append(toRemove, key)
		}
	})

	for _, key := range toRemove {
		sm.recentlyRemovedSessions.Remove(key)
	}
}

// DumpApiSessions provides diagnostic output of all tracked API sessions
// for operational debugging and system health monitoring. The implementation
// includes timeout protection to prevent blocking during high session volumes.
func (sm *ManagerImpl) DumpApiSessions(c *bufio.ReadWriter) error {
	ch := make(chan string, 15)

	go func() {
		defer close(ch)
		i := 0
		deadline := time.After(time.Second)
		timedOut := false

		for _, session := range sm.legacyApiSessionsByToken.Items() {
			i++
			val := fmt.Sprintf("%v: id: %v, token: %v\n", i, session.Id, session.Token())
			select {
			case ch <- val:
			case <-deadline:
				timedOut = true
				break
			}
			if i%10000 == 0 {
				// allow a second to dump each 10k entries
				deadline = time.After(time.Second)
			}

		}

		if timedOut {
			select {
			case ch <- "timed out":
			case <-time.After(time.Second):
			}
		}
	}()

	for val := range ch {
		if _, err := c.WriteString(val); err != nil {
			return err
		}
	}
	return c.Flush()
}

// ValidateSessions performs batch validation of active service sessions against
// controller state, enabling detection of sessions that have been revoked or expired.
// The chunked approach with randomized intervals prevents thundering herd effects
// while maintaining reasonable validation latency for large session volumes.
func (sm *ManagerImpl) ValidateSessions(ch channel.Channel, chunkSize uint32, minInterval, maxInterval time.Duration) {

	sessionTokens := sm.ActiveServiceSessionTokens()

	legacySessionTokens := make([]*ServiceSessionToken, 0, len(sessionTokens))
	for _, sessionToken := range sessionTokens {
		if ok, _ := sessionToken.IsLegacyApiSession(); ok {
			legacySessionTokens = append(legacySessionTokens, sessionToken)
		}
	}

	for len(legacySessionTokens) > 0 {
		var chunk []*ServiceSessionToken

		if len(legacySessionTokens) > int(chunkSize) {
			chunk = legacySessionTokens[:chunkSize]
			legacySessionTokens = legacySessionTokens[chunkSize:]
		} else {
			chunk = legacySessionTokens
			legacySessionTokens = nil
		}
		tokens := make([]string, 0, len(chunk))
		for _, sessionToken := range chunk {
			tokens = append(tokens, sessionToken.TokenId())
		}

		request := &edge_ctrl_pb.ValidateSessionsRequest{
			SessionTokens: tokens,
		}

		logrus.Debugf("validating edge sessions: %v", chunk)

		body, err := proto.Marshal(request)
		if err != nil {
			logrus.WithError(err).Error("failed to marshal validate sessions request")
			return
		}

		msg := channel.NewMessage(request.GetContentType(), body)
		if err := ch.Send(msg); err != nil {
			logrus.WithError(err).Error("failed to send validate sessions request")
			return
		}

		if len(legacySessionTokens) > 0 {
			interval := minInterval
			if minInterval < maxInterval {
				/* #nosec */
				delta := rand.Int63n(int64(maxInterval - minInterval))
				interval += minInterval + time.Duration(delta)
			}
			time.Sleep(interval)
		}
	}

}

// parsePublicKey converts protobuf public key data into Go crypto types
// based on the specified format.
func (sm *ManagerImpl) parsePublicKey(publicKey *edge_ctrl_pb.DataState_PublicKey) (crypto.PublicKey, error) {
	switch publicKey.Format {
	case edge_ctrl_pb.DataState_PublicKey_X509CertDer:
		certs, err := x509.ParseCertificates(publicKey.Data)
		if err != nil {
			return nil, err
		}

		if len(certs) == 0 {
			return nil, errors.New("could not parse certificates, der was empty")
		}

		return certs[0].PublicKey, nil
	case edge_ctrl_pb.DataState_PublicKey_PKIXPublicKey:
		return x509.ParsePKIXPublicKey(publicKey.Data)
	}

	return nil, fmt.Errorf("unsupported public key format: %s", publicKey.Format.String())
}

// LoadConfig satisfies the configuration interface but performs no operations
// as the state manager's configuration is handled during initialization.
func (sm *ManagerImpl) LoadConfig(_ map[interface{}]interface{}) error {
	return nil
}

// BindChannel registers message handlers for controller communication protocols,
// establishing the router's ability to receive session updates, data model changes,
// and other control plane messages essential for distributed operation.
func (sm *ManagerImpl) BindChannel(binding channel.Binding) error {
	binding.AddTypedReceiveHandler(NewHelloHandler(sm, sm.env.GetConfig().Edge.EdgeListeners))
	binding.AddTypedReceiveHandler(NewExtendEnrollmentCertsHandler(sm.env))

	binding.AddTypedReceiveHandler(NewSessionRemovedHandler(sm))
	binding.AddTypedReceiveHandler(NewApiSessionAddedHandler(sm, binding))
	binding.AddTypedReceiveHandler(NewApiSessionRemovedHandler(sm))
	binding.AddTypedReceiveHandler(NewApiSessionUpdatedHandler(sm))
	binding.AddTypedReceiveHandler(NewDataStateHandler(sm))
	binding.AddTypedReceiveHandler(NewDataStateEventHandler(sm))
	binding.AddTypedReceiveHandler(NewValidateDataStateRequestHandler(sm, sm.env))
	return nil
}

// Enabled indicates this component is always active in router operation.
func (sm *ManagerImpl) Enabled() bool {
	return true
}

// Run satisfies the component interface but performs no ongoing operations
// as state management is event-driven rather than polling-based.
func (sm *ManagerImpl) Run(env.RouterEnv) error {
	return nil
}

// NotifyOfReconnect handles controller reconnection events but currently
// performs no specific actions as session resynchronization is handled
// through other mechanisms.
func (sm *ManagerImpl) NotifyOfReconnect(_ channel.Channel) {
}

// GetTraceDecoders returns message decoders for debugging and monitoring
// purposes, currently returning nil as no specialized tracing is implemented.
func (sm *ManagerImpl) GetTraceDecoders() []channel.TraceMessageDecoder {
	return nil
}

// GetConnProviderAndSinksFromCh extracts connection provider and sinks
// from channel user data for connection management operations.
func GetConnProviderAndSinksFromCh(ch channel.Channel) (ConnProvider, map[uint32]edge.MsgSink[*ConnState]) {
	userData := ch.GetUserData()

	if userData != nil {
		edgeConn := userData.(ConnProvider)
		connIdToSink := edgeConn.GetConnIdToSinks()
		return edgeConn, connIdToSink
	}
	return nil, nil
}

// GetApiSessionTokenProviderFromCh extracts the API session token provider
// from channel user data.
func GetApiSessionTokenProviderFromCh(ch channel.Channel) ApiSessionTokenProvider {
	userData := ch.GetUserData()
	if userData != nil {
		edgeConn := userData.(ApiSessionTokenProvider)
		return edgeConn
	}

	return nil
}

// GetApiSessionTokenFromCh retrieves the API session token associated
// with a channel.
func GetApiSessionTokenFromCh(ch channel.Channel) *ApiSessionToken {
	tokenProvider := GetApiSessionTokenProviderFromCh(ch)

	if tokenProvider == nil {
		return nil
	}

	return tokenProvider.GetApiSessionToken()
}
