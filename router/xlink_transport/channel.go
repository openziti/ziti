package xlink_transport

import (
	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/router/env"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ChannelTypeAck     string = "link.ack"
	ChannelTypeDefault string = "link.default"
)

func NewBaseLinkChannel(underlay channel.Underlay) *BaseLinkChannel {
	senderContext := channel.NewSenderContext()

	defaultMsgChan := make(chan channel.Sendable, 64)
	controlMsgChan := make(chan channel.Sendable, 4)
	retryMsgChan := make(chan channel.Sendable, 4)

	result := &BaseLinkChannel{
		SenderContext:  senderContext,
		id:             underlay.ConnectionId(),
		defaultSender:  channel.NewSingleChSender(senderContext, defaultMsgChan),
		ackSender:      channel.NewSingleChSender(senderContext, controlMsgChan),
		ackMsgChan:     controlMsgChan,
		defaultMsgChan: defaultMsgChan,
		retryMsgChan:   retryMsgChan,
	}
	return result
}

type BaseLinkChannel struct {
	id string
	ch channel.MultiChannel
	channel.SenderContext
	ackSender     channel.Sender
	defaultSender channel.Sender

	ackMsgChan     chan channel.Sendable
	defaultMsgChan chan channel.Sendable
	retryMsgChan   chan channel.Sendable
	connIteration  atomic.Uint32
}

func (self *BaseLinkChannel) InitChannel(ch channel.MultiChannel) {
	self.ch = ch
}

func (self *BaseLinkChannel) GetChannel() channel.Channel {
	return self.ch
}

func (self *BaseLinkChannel) GetDefaultSender() channel.Sender {
	return self.defaultSender
}

func (self *BaseLinkChannel) GetAckSender() channel.Sender {
	return self.ackSender
}

func (self *BaseLinkChannel) GetNextMsgDefault(notifier *channel.CloseNotifier) (channel.Sendable, error) {
	select {
	case msg := <-self.defaultMsgChan:
		return msg, nil
	case msg := <-self.ackMsgChan:
		return msg, nil
	case msg := <-self.retryMsgChan:
		return msg, nil
	case <-self.GetCloseNotify():
		return nil, io.EOF
	case <-notifier.GetCloseNotify():
		return nil, io.EOF
	}
}

func (self *BaseLinkChannel) GetNextAckMsg(notifier *channel.CloseNotifier) (channel.Sendable, error) {
	select {
	case msg := <-self.ackMsgChan:
		return msg, nil
	case msg := <-self.retryMsgChan:
		return msg, nil
	case <-self.GetCloseNotify():
		return nil, io.EOF
	case <-notifier.GetCloseNotify():
		return nil, io.EOF
	}
}

func (self *BaseLinkChannel) GetMessageSource(underlay channel.Underlay) channel.MessageSourceF {
	if channel.GetUnderlayType(underlay) == ChannelTypeAck {
		return self.GetNextAckMsg
	}
	return self.GetNextMsgDefault
}

func (self *BaseLinkChannel) HandleTxFailed(_ channel.Underlay, sendable channel.Sendable) bool {
	select {
	case self.retryMsgChan <- sendable:
		return true
	case self.defaultMsgChan <- sendable:
		return true
	default:
		return false
	}
}

func (self *BaseLinkChannel) GetConnStateIteration() uint32 {
	return self.connIteration.Load()
}

func (self *BaseLinkChannel) HandleUnderlayAccepted(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay added")
	self.connIteration.Add(1)
}

type DialLinkChannelConfig struct {
	Dialer                 channel.DialUnderlayFactory
	Underlay               channel.Underlay
	MaxDefaultChannels     int
	MaxAckChannel          int
	StartupDelay           time.Duration
	UnderlayChangeCallback func(ch *DialLinkChannel)
}

func NewDialLinkChannel(config DialLinkChannelConfig) UnderlayHandlerLinkChannel {
	result := &DialLinkChannel{
		BaseLinkChannel: *NewBaseLinkChannel(config.Underlay),
		dialer:          config.Dialer,
		changeCallback:  config.UnderlayChangeCallback,
		syncRequired:    map[string]struct{}{},
		startupDelay:    config.StartupDelay,
	}

	result.constraints.AddConstraint(ChannelTypeDefault, config.MaxDefaultChannels, 1)
	result.constraints.AddConstraint(ChannelTypeAck, config.MaxAckChannel, 0)

	return result
}

type UnderlayHandlerLinkChannel interface {
	LinkChannel
	channel.UnderlayHandler
}

type LinkChannel interface {
	InitChannel(channel.MultiChannel)
	GetChannel() channel.Channel
	GetDefaultSender() channel.Sender
	GetAckSender() channel.Sender
	GetConnStateIteration() uint32
}

type StateTrackingLinkChannel interface {
	LinkChannel
	MarkLinkStateSynced(ctrlId string)
	MarkLinkStateSyncedForState(ctrlId string, stateId string)
	GetCtrlRequiringSync() (string, []string)
}

type DialLinkChannel struct {
	BaseLinkChannel
	dialer         channel.DialUnderlayFactory
	constraints    channel.UnderlayConstraints
	changeCallback func(ch *DialLinkChannel)
	startupDelay   time.Duration

	syncLock       sync.Mutex
	syncRequired   map[string]struct{}
	currentStateId string
}

func (self *DialLinkChannel) MarkLinkStateSynced(ctrlId string) {
	self.syncLock.Lock()
	defer self.syncLock.Unlock()
	delete(self.syncRequired, ctrlId)
}

func (self *DialLinkChannel) MarkLinkStateSyncedForState(ctrlId string, stateId string) {
	self.syncLock.Lock()
	defer self.syncLock.Unlock()
	if stateId == self.currentStateId {
		delete(self.syncRequired, ctrlId)
	}
}

func (self *DialLinkChannel) GetCtrlRequiringSync() (string, []string) {
	self.syncLock.Lock()
	defer self.syncLock.Unlock()
	var result []string
	for k := range self.syncRequired {
		result = append(result, k)
	}
	return self.currentStateId, result
}

func (self *DialLinkChannel) LinkConnectionsChanged(ctrls env.NetworkControllers) (string, bool) {
	self.syncLock.Lock()
	defer self.syncLock.Unlock()
	first := self.currentStateId == ""
	self.currentStateId = uuid.NewString()

	if !first {
		self.syncRequired = map[string]struct{}{}

		ctrls.ForEach(func(ctrlId string, _ channel.Channel) {
			self.syncRequired[ctrlId] = struct{}{}
		})
	}

	return self.currentStateId, first
}

func (self *DialLinkChannel) Start(channel channel.MultiChannel) {
	if self.startupDelay == 0 {
		self.constraints.Apply(channel, self)
	} else {
		time.AfterFunc(self.startupDelay, func() {
			self.constraints.Apply(channel, self)
		})
	}
}

func (self *DialLinkChannel) HandleUnderlayClose(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		WithField("channelClosed", ch.IsClosed()).
		Info("underlay closed")

	self.connIteration.Add(1)
	self.changeCallback(self)
	self.constraints.Apply(ch, self)
}

func (self *DialLinkChannel) HandleUnderlayAccepted(ch channel.MultiChannel, underlay channel.Underlay) {
	self.BaseLinkChannel.HandleUnderlayAccepted(ch, underlay)
	self.changeCallback(self)
}

func (self *DialLinkChannel) DialFailed(_ channel.MultiChannel, _ string, attempt int) {
	delay := 2 * time.Duration(attempt) * time.Second
	if delay > time.Minute {
		delay = time.Minute
	}
	time.Sleep(delay)
}

func (self *DialLinkChannel) CreateGroupedUnderlay(groupId string, groupSecret []byte, underlayType string, timeout time.Duration) (channel.Underlay, error) {
	return self.dialer.CreateWithHeaders(timeout, map[int32][]byte{
		channel.TypeHeader:         []byte(underlayType),
		channel.ConnectionIdHeader: []byte(groupId),
		channel.GroupSecretHeader:  groupSecret,
		channel.IsGroupedHeader:    {1},
	})
}

func NewListenerLinkChannel(underlay channel.Underlay) UnderlayHandlerLinkChannel {
	result := &ListenerLinkChannel{
		BaseLinkChannel: *NewBaseLinkChannel(underlay),
	}

	result.constraints.AddConstraint(ChannelTypeDefault, 1, 1)
	result.constraints.AddConstraint(ChannelTypeAck, 1, 0)

	return result
}

type ListenerLinkChannel struct {
	BaseLinkChannel
	constraints channel.UnderlayConstraints
}

func (self *ListenerLinkChannel) Start(channel channel.MultiChannel) {
	self.constraints.CheckStateValid(channel, true)
}

func (self *ListenerLinkChannel) HandleUnderlayClose(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay closed")
	self.constraints.CheckStateValid(ch, true)
}

func (self *ListenerLinkChannel) MarkLinkStateSynced(string) {
	// no action required
}

func NewSingleLinkChannel(ch channel.Channel) LinkChannel {
	return &SingleLinkChannel{
		ch: ch,
	}
}

type SingleLinkChannel struct {
	ch channel.Channel
}

func (self *SingleLinkChannel) InitChannel(channel.MultiChannel) {
}

func (self *SingleLinkChannel) GetChannel() channel.Channel {
	return self.ch
}

func (self *SingleLinkChannel) GetDefaultSender() channel.Sender {
	return self.ch
}

func (self *SingleLinkChannel) GetAckSender() channel.Sender {
	return self.ch
}

func (self *SingleLinkChannel) MarkLinkStateSynced(string) {
	// no action required
}

func (self *SingleLinkChannel) GetConnStateIteration() uint32 {
	return 1
}
