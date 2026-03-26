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
	"github.com/openziti/channel/v4"
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
	GetSubscriptionController() NetworkController
	AllControllersHaveCapability(cap int) bool
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

	// Start dialing new controllers that aren't already dialing or connected
	for ctrlId, detail := range newIdSet {
		if !self.idsBeingDialed.Has(ctrlId) && self.ctrls.Get(ctrlId) == nil {
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

		// Already connected by ID
		if self.ctrls.Get(detail.Id) != nil {
			log.Info("already connected to controller, exiting retry")
			return nil
		}

		// Already connected by address
		for _, ep := range detail.Endpoints {
			for _, v := range self.ctrls.AsMap() {
				if v.Address() == ep.Address {
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

func (self *networkControllers) connectToController(endpoint string, addr transport.Address) error {
	headers, err := self.dialEnv.GetChannelHeaders()
	if err != nil {
		return err
	}

	config := self.dialEnv.GetConfig()

	if config.Ctrl.LocalBinding != "" {
		logrus.Debugf("Using local interface %s to dial controller", config.Ctrl.LocalBinding)
	}

	// Build headers for the initial dial, including grouped channel flags
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutStringHeader(channel.TypeHeader, ctrlchan.ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity:     config.Id,
		Endpoint:     addr,
		LocalBinding: config.Ctrl.LocalBinding,
		Headers:      headers,
		TransportConfig: transport.Configuration{
			transport.KeyProtocol:                 "ziti-ctrl",
			transport.KeyCachedProxyConfiguration: config.Proxy,
		},
	})

	// Dial initial underlay
	underlay, err := dialer.CreateWithHeaders(config.Ctrl.Options.ConnectTimeout, headers)
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
		binding.AddReceiveHandlerF(int32(ctrl_pb.ContentType_CanaryStatusType), self.handleCanaryStatusUpdate)

		if err = self.Add(endpoint, dialCtrlChan, binding.GetChannel(), underlay); err != nil {
			return err
		}

		binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
			ctrl := self.GetNetworkController(id)
			self.ctrls.Delete(id)
			if ctrl != nil {
				self.notifyOfChange(ctrl, ControllerDisconnected)
			}
			if detail := self.getControllerDetail(id); detail != nil {
				time.AfterFunc(time.Second, func() {
					self.connectToControllerWithBackoff(detail)
				})
			}
		}))

		return nil
	})

	combinedBindHandler := channel.BindHandlers(bindHandler, self.dialEnv.GetCtrlChannelBindHandler())

	multiChannelConfig := &channel.MultiChannelConfig{
		LogicalName:     fmt.Sprintf("ctrl/%s", underlay.Id()),
		Options:         config.Ctrl.Options,
		UnderlayHandler: dialCtrlChan,
		BindHandler:     combinedBindHandler,
		Underlay:        underlay,
	}

	if _, err = channel.NewMultiChannel(multiChannelConfig); err != nil {
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

func (self *networkControllers) handleCanaryStatusUpdate(m *channel.Message, ch channel.Channel) {
	if seq, ok := m.GetUint64Header(int32(ctrl_pb.ControlHeaders_CanarySeqHeader)); ok {
		if ctrl := self.GetNetworkController(ch.Id()); ctrl != nil {
			ctrl.updateCanarySeq(seq)
		}
	}
}

// GetSubscriptionController returns the controller that should receive canaries
// and (when all controllers are gossip-capable) link reports. Selection prefers
// the leader, then falls back to the most responsive controller.
func (self *networkControllers) GetSubscriptionController() NetworkController {
	var current NetworkController
	for _, ctrl := range self.ctrls.AsMap() {
		if !ctrl.IsConnected() || ctrl.IsUnresponsive() {
			continue
		}
		if current == nil ||
			(!self.isLeader(current) && self.isLeader(ctrl)) ||
			(!self.isLeader(current) && ctrl.isMoreResponsive(current)) {
			current = ctrl
		}
	}
	return current
}

// AllControllersHaveCapability returns true if every connected controller
// advertises the given capability. Returns false if no controllers are connected.
func (self *networkControllers) AllControllersHaveCapability(cap int) bool {
	all := self.ctrls.AsMap()
	if len(all) == 0 {
		pfxlog.Logger().WithField("capability", cap).
			Warn("AllControllersHaveCapability: no controllers in map")
		return false
	}
	for ctrlId, ctrl := range all {
		if !ctrl.HasCapability(cap) {
			pfxlog.Logger().WithField("capability", cap).
				WithField("ctrlId", ctrlId).
				Warn("AllControllersHaveCapability: controller missing capability")
			return false
		}
	}
	return true
}

func (self *networkControllers) Add(address string, ctrlCh ctrlchan.CtrlChannel, ch channel.Channel, underlay channel.Underlay) error {
	ctrl := newNetworkCtrl(ctrlCh, address, self.heartbeatOptions)
	ctrl.setCapabilities(capabilities.GetCapabilities(underlay.Headers()))

	if versionValue, found := underlay.Headers()[channel.HelloVersionHeader]; found {
		if versionInfo, err := versions.StdVersionEncDec.Decode(versionValue); err == nil {
			ctrl.versionInfo = versionInfo
		} else {
			return fmt.Errorf("could not parse version info from controller hello, closing connection (%w)", err)
		}
	} else {
		return errors.New("no version header provided")
	}

	if existing := self.ctrls.Get(ch.Id()); existing != nil {
		if !existing.Channel().IsClosed() && existing.IsConnected() {
			// if an existing channel exists and is connected, reject the duplicate
			return backoff.Permanent(fmt.Errorf("duplicate channel with id %v", ctrl.Channel().Id()))
		}
		// existing channel is closed or disconnected (0 underlays) — close it and accept new one
		if !existing.Channel().IsClosed() {
			if closeErr := existing.Channel().Close(); closeErr != nil {
				pfxlog.Logger().WithError(closeErr).WithField("ch", existing.Channel().Label()).Error("error closing control channel")
			}
		}
	}
	self.ctrls.Put(ch.Id(), ctrl)

	self.notifyOfChange(ctrl, ControllerAdded)

	return nil
}

func (self *networkControllers) AcceptCtrlChannel(address string, ctrlCh ctrlchan.CtrlChannel, binding channel.Binding, underlay channel.Underlay) error {
	id := binding.GetChannel().Id()
	binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CurrentIndexMessageType), self.handleRouterDataModelIndexUpdate)
	binding.AddReceiveHandlerF(int32(ctrl_pb.ContentType_CanaryStatusType), self.handleCanaryStatusUpdate)

	if err := self.Add(address, ctrlCh, binding.GetChannel(), underlay); err != nil {
		return err
	}

	binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
		ctrl := self.GetNetworkController(id)
		self.ctrls.Delete(id)
		if ctrl != nil {
			self.notifyOfChange(ctrl, ControllerDisconnected)
		}
		if detail := self.getControllerDetail(id); detail != nil {
			self.connectToControllerWithBackoff(detail)
		}
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
