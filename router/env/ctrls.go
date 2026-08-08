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
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/inspect"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
)

type CtrlEventListener interface {
	NotifyOfCtrlEvent(event CtrlEvent)
}

type CtrlEventListenerFunc func(event CtrlEvent)

func (self CtrlEventListenerFunc) NotifyOfCtrlEvent(event CtrlEvent) {
	self(event)
}

type CtrlEventType string

const (
	ControllerAdded        CtrlEventType = "Added"
	ControllerDisconnected CtrlEventType = "Disconnected"
	ControllerReconnected  CtrlEventType = "Reconnected"
	ControllerRemoved      CtrlEventType = "Removed"
	ControllerLeaderChange CtrlEventType = "LeaderChange"
)

type CtrlEvent struct {
	Type       CtrlEventType
	Controller NetworkController
}

type DialEnv interface {
	GetChannelHeaders() (channel.Headers, error)
	GetConfig() *Config
	GetCtrlChannelBindHandler() channel.BindHandler
	NotifyOfReconnect(ch ctrlchan.CtrlChannel)
}

type NetworkControllers interface {
	GetControllerDetails() map[string]*ctrl_pb.CtrlDetail
	UpdateControllerDetails(controllers []*ctrl_pb.CtrlDetail) bool
	ConnectToInitialEndpoints(endpoints []string)
	UpdateLeader(leaderId string)
	GetAll() map[string]NetworkController
	GetNetworkController(ctrlId string) NetworkController
	AnyChannel() channel.Channel
	AnyCtrlChannel() ctrlchan.CtrlChannel
	GetModelUpdateCtrlChannel() channel.Channel
	GetIfResponsive(ctrlId string) (channel.Channel, bool)
	AllResponsiveCtrlChannels() []channel.Channel
	AnyValidCtrlChannel() channel.Channel
	GetCtrlChannel(ctrlId string) ctrlchan.CtrlChannel
	GetChannel(ctrlId string) channel.Channel
	DefaultRequestTimeout() time.Duration
	ForEach(f func(ctrlId string, ch channel.Channel))
	Close() error
	Inspect() *inspect.ControllerInspectDetails
	AddChangeListener(listener CtrlEventListener)
	NotifyOfDisconnect(ctrlId string)
	NotifyOfReconnect(ctrlId string)
	IsLeaderConnected() bool
	ControllersHaveMinVersion(version string) bool
	GetLeader() NetworkController
	AcceptCtrlChannel(address string, ctrlCh ctrlchan.CtrlChannel, binding channel.Binding, underlay channel.Underlay) error
}

type CtrlDialer func(address transport.Address, bindHandler channel.BindHandler) error

func NewNetworkControllers(dialEnv DialEnv, heartbeatOptions *HeartbeatOptions) NetworkControllers {
	return &networkControllers{
		dialEnv:               dialEnv,
		heartbeatOptions:      heartbeatOptions,
		defaultRequestTimeout: dialEnv.GetConfig().Ctrl.DefaultRequestTimeout,
		idsBeingDialed:        cmap.New[struct{}](),
	}
}

type networkControllers struct {
	lock                  sync.Mutex
	dialEnv               DialEnv
	heartbeatOptions      *HeartbeatOptions
	defaultRequestTimeout time.Duration
	idsBeingDialed        cmap.ConcurrentMap[string, struct{}]
	ctrls                 concurrenz.CopyOnWriteMap[string, NetworkController]
	leaderId              concurrenz.AtomicValue[string]
	ctrlChangeListeners   concurrenz.CopyOnWriteSlice[CtrlEventListener]
	controllerDetails     concurrenz.AtomicValue[map[string]*ctrl_pb.CtrlDetail]
}

func (self *networkControllers) ControllersHaveMinVersion(version string) bool {
	for _, ctrl := range self.ctrls.AsMap() {
		hasMinVersion, err := ctrl.GetVersion().HasMinimumVersion(version)
		if err != nil {
			pfxlog.Logger().WithError(err).WithField("version", version).Error("failed to check version")
			return false
		}
		if !hasMinVersion {
			return false
		}
	}
	return true
}

func (self *networkControllers) AddChangeListener(listener CtrlEventListener) {
	self.ctrlChangeListeners.Append(listener)
}

func (self *networkControllers) GetControllerDetails() map[string]*ctrl_pb.CtrlDetail {
	return maps.Clone(self.controllerDetails.Load())
}

func (self *networkControllers) UpdateControllerDetails(controllers []*ctrl_pb.CtrlDetail) bool {
	self.lock.Lock()
	defer self.lock.Unlock()

	newIdSet := map[string]*ctrl_pb.CtrlDetail{}
	for _, ctrl := range controllers {
		newIdSet[ctrl.Id] = ctrl
	}

	self.controllerDetails.Store(newIdSet)

	changed := false
	log := pfxlog.Logger()

	// Remove controllers being dialed that are no longer in the new list
	for _, ctrlId := range self.idsBeingDialed.Keys() {
		if _, ok := newIdSet[ctrlId]; !ok {
			log.WithField("ctrlId", ctrlId).Info("removing old ctrl (was being dialed)")
			changed = true
			self.closeAndRemoveById(ctrlId)
		}
	}

	// Remove connected controllers that are no longer in the new list
	for ctrlId := range self.ctrls.AsMap() {
		if _, ok := newIdSet[ctrlId]; !ok {
			log.WithField("ctrlId", ctrlId).Info("removing old ctrl (was connected)")
			changed = true
			self.closeAndRemoveById(ctrlId)
		}
	}

	// Start dialing new controllers that aren't already dialing or usably connected
	for ctrlId, detail := range newIdSet {
		if !self.idsBeingDialed.Has(ctrlId) && !isUsable(self.ctrls.Get(ctrlId)) {
			log.WithField("ctrlId", ctrlId).WithField("endpoints", detail.Endpoints).Info("adding new ctrl")
			changed = true
			self.connectToControllerWithBackoff(detail)
		}
	}

	return changed
}

func (self *networkControllers) ConnectToInitialEndpoints(endpoints []string) {
	for _, endpoint := range endpoints {
		self.connectToControllerWithBackoff(&ctrl_pb.CtrlDetail{
			Id:        "",
			Endpoints: []*ctrl_pb.CtrlEndpoint{{Address: endpoint}},
		})
	}
}

func (self *networkControllers) UpdateLeader(leaderId string) {
	oldLeaderId := self.leaderId.Swap(leaderId)
	if oldLeaderId != leaderId {
		if leader := self.ctrls.Get(leaderId); leader != nil {
			self.notifyOfChange(leader, ControllerLeaderChange)
		} else {
			self.notifyOfChange(nil, ControllerLeaderChange)
		}
	}
}

func (self *networkControllers) getControllerDetail(controllerId string) *ctrl_pb.CtrlDetail {
	return self.controllerDetails.Load()[controllerId]
}

func (self *networkControllers) connectToControllerWithBackoff(detail *ctrl_pb.CtrlDetail) {
	log := pfxlog.Logger().WithField("ctrlId", detail.Id).WithField("detail", detail)

	if len(detail.Endpoints) == 0 {
		log.Error("controller has no endpoints, unable to connect")
		return
	}

	if detail.Id != "" && !self.idsBeingDialed.SetIfAbsent(detail.Id, struct{}{}) {
		log.Info("already dialing controller, skipping")
		return
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 50 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = 100 * 365 * 24 * time.Hour

	idx := 0
	operation := func() error {
		if detail.Id != "" && !self.idsBeingDialed.Has(detail.Id) {
			return backoff.Permanent(errors.New("controller removed before connection established"))
		}

		// Already connected by id. An unusable registration is not a reason to stop: the dial displaces it,
		// where treating it as connected would end the retries and leave the router holding nothing.
		if isUsable(self.ctrls.Get(detail.Id)) {
			log.Info("already connected to controller, exiting retry")
			return nil
		}

		// Already connected by address
		for _, ep := range detail.Endpoints {
			for _, v := range self.ctrls.AsMap() {
				if v.Address() == ep.Address && isUsable(v) {
					log.WithField("endpoint", ep.Address).Info("already connected to controller by address, exiting retry")
					return nil
				}
			}

			if detail.Id == "" {
				for _, dialingCtrlId := range self.idsBeingDialed.Keys() {
					dialingCtrl := self.getControllerDetail(dialingCtrlId)
					if dialingCtrl == nil {
						continue
					}

					for _, knownCrlEp := range dialingCtrl.Endpoints {
						if knownCrlEp.Address == ep.Address {
							log.WithField("endpoint", ep.Address).Info("endpoint dial taken over, exiting retry")
							return nil
						}
					}
				}
			}
		}

		ep := detail.Endpoints[idx%len(detail.Endpoints)]
		idx++

		addr, err := transport.ParseAddress(ep.Address)
		if err != nil {
			log.WithField("endpoint", ep.Address).WithError(err).Error("unable to parse endpoint address, trying next")
			return err
		}

		err = self.connectToController(ep.Address, addr)
		if err != nil {
			if ctrlId, ok := self.duplicateResolved(err); ok {
				log.WithField("endpoint", ep.Address).WithField("ctrlId", ctrlId).
					Info("endpoint reached an already connected controller, exiting retry")
				return nil
			}
			log.WithField("endpoint", ep.Address).WithError(err).Error("unable to connect controller")
		}
		return err
	}

	log.Info("starting connection attempts")

	go func() {
		defer func() {
			self.idsBeingDialed.Remove(detail.Id)
		}()

		if err := backoff.Retry(operation, expBackoff); err != nil {
			log.WithError(err).Error("unable to connect controller, stopping retries.")
		} else {
			log.Info("successfully connected to controller")
		}
	}()
}

// firstUnderlayHeaders returns the hello headers for the initial underlay of a grouped control
// channel, the only underlay that carries IsFirstGroupConnection. These are deliberately built fresh
// rather than added to the dialer's base
// headers: additional underlays reuse the base headers, and an inherited first-connection flag would
// both spawn a new listener-side group and make each additional underlay look like a new channel to
// the controller's already-connected guard.
func firstUnderlayHeaders() channel.Headers {
	first := channel.Headers{}
	first.PutBoolHeader(channel.IsGroupedHeader, true)
	first.PutStringHeader(channel.TypeHeader, ctrlchan.ChannelTypeDefault)
	first.PutBoolHeader(channel.IsFirstGroupConnection, true)
	return first
}

func (self *networkControllers) connectToController(endpoint string, addr transport.Address) error {
	headers, err := self.dialEnv.GetChannelHeaders()
	if err != nil {
		return err
	}

	config := self.dialEnv.GetConfig()

	if config.Ctrl.LocalBinding != "" {
		logrus.Debugf("Using local interface %s to dial controller", config.Ctrl.LocalBinding)
	}

	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity:     config.Id,
		Endpoint:     addr,
		LocalBinding: config.Ctrl.LocalBinding,
		Headers:      headers, // base hello headers only; no group flags
		TransportConfig: transport.Configuration{
			transport.KeyProtocol:                 "ziti-ctrl",
			transport.KeyCachedProxyConfiguration: config.Proxy,
		},
	})

	// The initial underlay, and only it, carries IsFirstGroupConnection.
	firstDialHeaders := firstUnderlayHeaders()

	// Dial initial underlay
	underlay, err := dialer.CreateWithHeaders(config.Ctrl.Options.ConnectTimeout, firstDialHeaders)
	if err != nil {
		return fmt.Errorf("error connecting ctrl (%v)", err)
	}

	// Check if controller supports multi-underlay
	maxHigh := 0
	if capabilities.IsCapable(underlay.Headers(), capabilities.ControllerGroupedCtrlChan) {
		maxHigh = 1
	}

	// Track connectivity transitions for reconnect/disconnect notifications
	var wasDisconnected atomic.Bool
	changeCallback := func(ch *ctrlchan.DialCtrlChannel, oldCount, newCount uint32) {
		multiCh := ch.GetChannel()
		if multiCh == nil || multiCh.IsClosed() {
			return
		}
		if wasDisconnected.Load() && newCount > 0 {
			self.dialEnv.NotifyOfReconnect(ch)
			wasDisconnected.Store(false)
		} else if newCount == 0 {
			if wasDisconnected.CompareAndSwap(false, true) {
				self.NotifyOfDisconnect(ch.PeerId())
			}
		}
	}

	dialCtrlChan := ctrlchan.NewDialCtrlChannel(ctrlchan.DialCtrlChannelConfig{
		Dialer:                  dialer,
		MaxDefaultChannels:      1,
		MaxHighPriorityChannels: maxHigh,
		MaxLowPriorityChannels:  0,
		StartupDelay:            5 * time.Second,
		UnderlayChangeCallback:  changeCallback,
	})

	bindHandler := channel.BindHandlerF(func(binding channel.Binding) error {
		id := binding.GetChannel().Id()
		binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CurrentIndexMessageType), self.handleRouterDataModelIndexUpdate)

		// Record the channel before Add(), which fires ControllerAdded listeners that
		// dereference Channel(). UnderlayAdded fires after the bind handler, so relying on
		// it alone would leave Channel() nil during that notification.
		dialCtrlChan.InitChannel(binding.GetChannel())

		ctrl, err := self.Add(endpoint, dialCtrlChan, binding.GetChannel(), underlay)
		if err != nil {
			return err
		}

		binding.AddCloseHandler(channel.CloseHandlerF(func(channel.Channel) {
			self.handleChannelClose(id, ctrl, time.Second)
		}))

		return nil
	})

	combinedBindHandler := channel.BindHandlers(bindHandler, self.dialEnv.GetCtrlChannelBindHandler())

	multiChannelConfig := &channel.Config{
		LogicalName:            fmt.Sprintf("ctrl/%s", underlay.Id()),
		Options:                config.Ctrl.Options,
		Underlay:               underlay,
		Binder:                 channel.MakeBinder(combinedBindHandler),
		Senders:                dialCtrlChan,
		MessageSourceProvider:  dialCtrlChan,
		DialPolicy:             dialCtrlChan.GetDialPolicy(),
		Constraints:            dialCtrlChan.GetConstraints(),
		ConstraintStartupDelay: dialCtrlChan.GetStartupDelay(),
		UnderlayEventListeners: []channel.UnderlayEventListener{dialCtrlChan},
	}

	if _, err = channel.NewChannel(multiChannelConfig); err != nil {
		if closeErr := underlay.Close(); closeErr != nil {
			pfxlog.Logger().WithError(closeErr).Error("unable to close underlay")
		}

		if errors.Is(err, &backoff.PermanentError{}) {
			return err
		}

		return fmt.Errorf("error connecting ctrl (%w)", err)
	}

	// If there are multiple controllers we may have to catch up the controllers that connected later
	// with things that have already happened because we had state from other controllers, such as
	// links
	self.dialEnv.NotifyOfReconnect(dialCtrlChan)

	return nil
}

func (self *networkControllers) handleRouterDataModelIndexUpdate(m *channel.Message, ch channel.Channel) {
	if idx, ok := m.GetUint64Header(int32(edge_ctrl_pb.Header_RouterDataModelIndex)); ok {
		if ctrl := self.GetNetworkController(ch.Id()); ctrl != nil {
			ctrl.updateDataModelIndex(idx)
		}
	}
}

// Add registers a newly established control channel and returns the entry created for it. The returned
// entry is the identity a close handler must present to give the registration up again, so callers that
// wire a close handler have to hold on to it.
func (self *networkControllers) Add(address string, ctrlCh ctrlchan.CtrlChannel, ch channel.Channel, underlay channel.Underlay) (NetworkController, error) {
	ctrl := newNetworkCtrl(ctrlCh, address, self.heartbeatOptions)

	if versionValue, found := underlay.Headers()[channel.HelloVersionHeader]; found {
		if versionInfo, err := versions.StdVersionEncDec.Decode(versionValue); err == nil {
			ctrl.versionInfo = versionInfo
		} else {
			return nil, fmt.Errorf("could not parse version info from controller hello, closing connection (%w)", err)
		}
	} else {
		return nil, errors.New("no version header provided")
	}

	log := pfxlog.Logger().
		WithField("ctrlId", ch.Id()).
		WithField("ch", ch.Label()).
		WithField("address", address)

	// Atomic against a concurrent Add for the same controller and against UpdateControllerDetails, which
	// decides whether to dial from the same state. Two dials do race: initial endpoint dials carry no
	// controller id, so idsBeingDialed cannot keep them apart.
	self.lock.Lock()

	existing := self.ctrls.Get(ch.Id())
	if isUsable(existing) {
		self.lock.Unlock()
		return nil, &errDuplicateChannel{ctrlId: ch.Id()}
	}

	// Hand the registration over before closing what it displaced, so the displaced close handler finds
	// itself superseded rather than observing an empty registration and racing this dial with a redial.
	self.ctrls.Put(ch.Id(), ctrl)

	self.lock.Unlock()

	log.Info("controller registered")

	// Closing a channel runs its close handlers on this goroutine, so it must happen outside the lock.
	if existing != nil && !existing.Channel().IsClosed() {
		if closeErr := existing.Channel().Close(); closeErr != nil {
			log.WithError(closeErr).WithField("displacedCh", existing.Channel().Label()).
				Error("error closing displaced control channel")
		}
	}

	self.notifyOfChange(ctrl, ControllerAdded)

	return ctrl, nil
}

// errDuplicateChannel reports that a control channel was refused because the controller it reached is
// already usably connected. It carries the controller's id because a dial started from a bare endpoint has
// none of its own, so this is the only way the retry loop can learn which controller the endpoint resolved
// to and stop dialing it.
type errDuplicateChannel struct {
	ctrlId string
}

func (self *errDuplicateChannel) Error() string {
	return fmt.Sprintf("controller %v is already connected on another channel", self.ctrlId)
}

// duplicateResolved reports the controller id when err is a refusal by a controller whose registration is
// still usable, meaning another channel already did what this dial set out to do. A dial that only reaches
// an endpoint learns the controller's id here and nowhere else, which is why the refusal has to carry it.
// The registration is rechecked because the channel that won the race may since have died, leaving the
// dial's work to do after all.
func (self *networkControllers) duplicateResolved(err error) (string, bool) {
	var duplicate *errDuplicateChannel
	if errors.As(err, &duplicate) && isUsable(self.ctrls.Get(duplicate.ctrlId)) {
		return duplicate.ctrlId, true
	}
	return "", false
}

// isUsable reports whether a controller registration can still carry traffic. A registration whose channel
// has been closed, or which has lost every underlay, satisfies a presence check while carrying nothing.
// Every decision about whether the router is connected to a controller has to ask this rather than whether
// a registration exists, or an entry left behind by a channel that died leaves the router believing it is
// connected and suppresses the reconnect that would fix it.
func isUsable(ctrl NetworkController) bool {
	return ctrl != nil && !ctrl.CtrlChannel().IsClosed() && ctrl.IsConnected()
}

// removeIfCurrent gives up ctrl's registration for ctrlId, and reports whether it was still the
// registered entry. Overlapping channels to one controller share its id, so removing by id alone lets a
// superseded channel's close delete its replacement's registration. Nothing re-registers a channel that
// is already established, so the surviving channel would stay unreachable through the map and the router
// would be invisible to that controller until the channel happened to close.
func (self *networkControllers) removeIfCurrent(ctrlId string, ctrl NetworkController) bool {
	return self.ctrls.DeleteIf(func(key string, val NetworkController) bool {
		return key == ctrlId && val == ctrl
	})
}

// handleChannelClose releases ctrl's registration and starts reconnecting to the controller. It is a
// no-op when ctrl is no longer the registered entry, either because a newer channel has taken over or
// because the controller was removed from the cluster. Reconnects are delayed by redialDelay, which
// paces a dial the controller rejects outright.
func (self *networkControllers) handleChannelClose(ctrlId string, ctrl NetworkController, redialDelay time.Duration) {
	log := pfxlog.Logger().WithField("ctrlId", ctrlId).WithField("ch", ctrl.Channel().Label())

	if !self.removeIfCurrent(ctrlId, ctrl) {
		log.Info("superseded control channel closed, leaving the current registration in place")
		return
	}

	log.Info("control channel closed, controller unregistered")
	self.notifyOfChange(ctrl, ControllerDisconnected)

	detail := self.getControllerDetail(ctrlId)
	if detail == nil {
		log.Info("controller is no longer known, not reconnecting")
		return
	}

	if redialDelay > 0 {
		time.AfterFunc(redialDelay, func() {
			self.connectToControllerWithBackoff(detail)
		})
	} else {
		self.connectToControllerWithBackoff(detail)
	}
}

func (self *networkControllers) AcceptCtrlChannel(address string, ctrlCh ctrlchan.CtrlChannel, binding channel.Binding, underlay channel.Underlay) error {
	id := binding.GetChannel().Id()
	binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CurrentIndexMessageType), self.handleRouterDataModelIndexUpdate)

	// Record the channel before Add() fires ControllerAdded listeners (see the dial path).
	ctrlCh.InitChannel(binding.GetChannel())

	ctrl, err := self.Add(address, ctrlCh, binding.GetChannel(), underlay)
	if err != nil {
		return err
	}

	binding.AddCloseHandler(channel.CloseHandlerF(func(channel.Channel) {
		self.handleChannelClose(id, ctrl, 0)
	}))

	return nil
}

func (self *networkControllers) NotifyOfDisconnect(ctrlId string) {
	if ctrl := self.GetNetworkController(ctrlId); ctrl != nil {
		self.notifyOfChange(ctrl, ControllerDisconnected)
	}
}

func (self *networkControllers) NotifyOfReconnect(ctrlId string) {
	if ctrl := self.GetNetworkController(ctrlId); ctrl != nil {
		self.notifyOfChange(ctrl, ControllerReconnected)
	}
}

func (self *networkControllers) notifyOfChange(controller NetworkController, eventType CtrlEventType) {
	for _, l := range self.ctrlChangeListeners.Value() {
		go l.NotifyOfCtrlEvent(CtrlEvent{
			Type:       eventType,
			Controller: controller,
		})
	}
}

func (self *networkControllers) GetAll() map[string]NetworkController {
	return self.ctrls.AsMap()
}

func (self *networkControllers) AnyCtrlChannel() ctrlchan.CtrlChannel {
	var current NetworkController
	for _, ctrl := range self.ctrls.AsMap() {
		if current == nil || ctrl.isMoreResponsive(current) {
			current = ctrl
		}
	}
	if current == nil {
		return nil
	}
	return current.CtrlChannel()
}

func (self *networkControllers) AnyChannel() channel.Channel {
	if ctrlCh := self.AnyCtrlChannel(); ctrlCh != nil {
		return ctrlCh.GetChannel()
	}

	return nil
}

func (self *networkControllers) isLeader(controller NetworkController) bool {
	return self.leaderId.Load() == controller.Channel().Id()
}

func (self *networkControllers) GetModelUpdateCtrlChannel() channel.Channel {
	var current NetworkController
	for _, ctrl := range self.ctrls.AsMap() {
		if current == nil ||
			(ctrl.isMoreResponsive(current) && !self.isLeader(current)) ||
			(!ctrl.IsUnresponsive() && self.isLeader(ctrl)) {
			current = ctrl
		}
	}
	if current == nil {
		return nil
	}
	return current.Channel()
}

func (self *networkControllers) AllResponsiveCtrlChannels() []channel.Channel {
	var channels []channel.Channel
	for _, ctrl := range self.ctrls.AsMap() {
		if !ctrl.IsUnresponsive() {
			channels = append(channels, ctrl.Channel())
		}
	}
	return channels
}

func (self *networkControllers) GetIfResponsive(ctrlId string) (channel.Channel, bool) {
	ch := self.ctrls.Get(ctrlId)
	if ch == nil {
		return nil, false
	}
	if ch.IsConnected() && !ch.IsUnresponsive() {
		return ch.Channel(), true
	}
	return nil, true
}

func (self *networkControllers) AnyValidCtrlChannel() channel.Channel {
	delay := 10 * time.Millisecond
	for {
		result := self.AnyChannel()
		if result != nil {
			return result
		}
		time.Sleep(delay)
		delay = delay * 2
		if delay >= time.Minute {
			delay = time.Minute
		}
	}
}

func (self *networkControllers) GetChannel(controllerId string) channel.Channel {
	if ctrl := self.ctrls.Get(controllerId); ctrl != nil {
		return ctrl.Channel()
	}
	return nil
}

func (self *networkControllers) GetCtrlChannel(controllerId string) ctrlchan.CtrlChannel {
	if ctrl := self.ctrls.Get(controllerId); ctrl != nil {
		return ctrl.CtrlChannel()
	}
	return nil
}

func (self *networkControllers) GetNetworkController(controllerId string) NetworkController {
	return self.ctrls.Get(controllerId)
}

func (self *networkControllers) DefaultRequestTimeout() time.Duration {
	return self.defaultRequestTimeout
}

func (self *networkControllers) ForEach(f func(controllerId string, ch channel.Channel)) {
	for controllerId, ctrl := range self.ctrls.AsMap() {
		f(controllerId, ctrl.Channel())
	}
}

func (self *networkControllers) Close() error {
	self.idsBeingDialed.Clear()
	var errList []error
	self.ForEach(func(_ string, ch channel.Channel) {
		if err := ch.Close(); err != nil {
			errList = append(errList, err)
		}
	})
	return errors.Join(errList...)
}

func (self *networkControllers) closeAndRemoveById(ctrlId string) {
	self.idsBeingDialed.Remove(ctrlId)

	if ctrl := self.ctrls.Get(ctrlId); ctrl != nil {
		self.ctrls.Delete(ctrlId)
		if err := ctrl.Channel().Close(); err != nil {
			pfxlog.Logger().WithField("ctrlId", ctrlId).WithError(err).Error("error closing channel to controller")
		}
		self.notifyOfChange(ctrl, ControllerRemoved)
	}
}

func (self *networkControllers) IsLeaderConnected() bool {
	ctrl := self.ctrls.Get(self.leaderId.Load())
	return ctrl != nil && ctrl.IsConnected()
}

func (self *networkControllers) GetLeader() NetworkController {
	ctrl := self.ctrls.Get(self.leaderId.Load())
	return ctrl
}

func (self *networkControllers) Inspect() *inspect.ControllerInspectDetails {
	result := &inspect.ControllerInspectDetails{
		LeaderId:    self.leaderId.Load(),
		Controllers: map[string]*inspect.ControllerInspectDetail{},
	}

	for id, ctrl := range self.ctrls.AsMap() {
		version := ""
		if ctrl.GetVersion() != nil {
			version = ctrl.GetVersion().Version
		}
		result.Controllers[id] = &inspect.ControllerInspectDetail{
			ControllerId:         id,
			IsConnected:          ctrl.IsConnected(),
			IsResponsive:         !ctrl.IsUnresponsive(),
			Address:              ctrl.Address(),
			Latency:              ctrl.Latency().String(),
			Version:              version,
			TimeSinceLastContact: ctrl.TimeSinceLastContact().String(),
			IsLeader:             id == self.leaderId.Load(),
		}
	}

	return result
}
