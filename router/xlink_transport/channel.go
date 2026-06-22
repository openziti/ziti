package xlink_transport

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/router/env"
)

const (
	ChannelTypeAck     string = "link.ack"
	ChannelTypeDefault string = "link.default"
)

func NewBaseLinkChannel(underlay channel.Underlay, payloadSenderQueueSize, ackSenderQueueSize int) *BaseLinkChannel {
	senderContext := channel.NewSenderContext()

	defaultMsgChan := make(chan channel.Sendable, payloadSenderQueueSize)
	controlMsgChan := make(chan channel.Sendable, ackSenderQueueSize)
	retryMsgChan := make(chan channel.Sendable, 4)

	result := &BaseLinkChannel{
		SenderContext:  senderContext,
		defaultSender:  channel.NewSingleChSender(senderContext, defaultMsgChan),
		ackSender:      channel.NewSingleChSender(senderContext, controlMsgChan),
		ackMsgChan:     controlMsgChan,
		defaultMsgChan: defaultMsgChan,
		retryMsgChan:   retryMsgChan,
	}
	return result
}

// BaseLinkChannel implements the channel/v5 Senders, MessageSourceProvider and
// UnderlayEventListener interfaces for a grouped link channel, routing ack messages
// onto the ack underlay and payload messages onto the default underlay.
type BaseLinkChannel struct {
	channel.SenderContext
	ch            channel.Channel
	ackSender     channel.Sender
	defaultSender channel.Sender

	ackMsgChan     chan channel.Sendable
	defaultMsgChan chan channel.Sendable
	retryMsgChan   chan channel.Sendable
	connIteration  atomic.Uint32
}

// InitChannel records the channel. It must be called from the bind handler, which runs on
// the construction goroutine before the channel is published, so a plain field is safe: the
// reference is invariant and set before any reader. The listener relies on this because it
// registers the link (LinkAccepted -> applyLink -> link.IsClosed() -> GetChannel()) while
// still inside NewChannel, before UnderlayAdded fires - the C3 hazard.
func (self *BaseLinkChannel) InitChannel(ch channel.Channel) {
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

// GetMessageSource implements channel.MessageSourceProvider.
func (self *BaseLinkChannel) GetMessageSource(underlayType string) channel.MessageSourceF {
	if underlayType == ChannelTypeAck {
		return self.GetNextAckMsg
	}
	return self.GetNextMsgDefault
}

func (self *BaseLinkChannel) HandleTxFailed(_ string, sendable channel.Sendable) bool {
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

// UnderlayAdded implements channel.UnderlayEventListener. Each added underlay bumps the
// connection-state iteration so link-state tracking can detect topology changes.
func (self *BaseLinkChannel) UnderlayAdded(ch channel.Channel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay added")
	self.connIteration.Add(1)
}

// UnderlayRemoved implements channel.UnderlayEventListener.
func (self *BaseLinkChannel) UnderlayRemoved(ch channel.Channel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		WithField("channelClosed", ch.IsClosed()).
		Info("underlay closed")
}

type DialLinkChannelConfig struct {
	Dialer                 channel.DialUnderlayFactory
	Underlay               channel.Underlay
	MaxDefaultChannels     int
	MaxAckChannel          int
	PayloadSenderQueueSize int
	AckSenderQueueSize     int
	StartupDelay           time.Duration
	UnderlayChangeCallback func(ch *DialLinkChannel)
}

// NewDialLinkChannel creates the dial-side grouped link channel. The supplied Dialer
// already carries the cloned link-id identity (ShallowCloneWithNewToken), so wrapping it
// in a channel.BackoffDialPolicy preserves the link-id-as-channel-id behavior for every
// underlay the policy dials. The default underlay keeps Min: 1, so losing it closes the
// channel (the data path then fails over) rather than recovering from zero.
func NewDialLinkChannel(config DialLinkChannelConfig) *DialLinkChannel {
	result := &DialLinkChannel{
		BaseLinkChannel: NewBaseLinkChannel(config.Underlay, config.PayloadSenderQueueSize, config.AckSenderQueueSize),
		changeCallback:  config.UnderlayChangeCallback,
		syncRequired:    map[string]struct{}{},
		startupDelay:    config.StartupDelay,
		// links target multiple underlays via consecutive successful dials and want fast
		// bring-up, so no MinDialInterval floor.
		dialPolicy: channel.NewBackoffDialPolicy(config.Dialer),
		constraints: map[string]channel.UnderlayConstraint{
			ChannelTypeDefault: {Desired: config.MaxDefaultChannels, Min: 1},
			ChannelTypeAck:     {Desired: config.MaxAckChannel, Min: 0},
		},
	}

	return result
}

type LinkChannel interface {
	InitChannel(ch channel.Channel)
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
	*BaseLinkChannel
	dialPolicy     channel.DialPolicy
	constraints    map[string]channel.UnderlayConstraint
	changeCallback func(ch *DialLinkChannel)
	startupDelay   time.Duration

	syncLock       sync.Mutex
	syncRequired   map[string]struct{}
	currentStateId string
}

// GetDialPolicy returns the channel.DialPolicy used to (re)establish underlays.
func (self *DialLinkChannel) GetDialPolicy() channel.DialPolicy {
	return self.dialPolicy
}

// GetConstraints returns the per-underlay-type constraints for the channel.
func (self *DialLinkChannel) GetConstraints() map[string]channel.UnderlayConstraint {
	return self.constraints
}

// GetStartupDelay returns the delay before the channel begins dialing additional underlays.
func (self *DialLinkChannel) GetStartupDelay() time.Duration {
	return self.startupDelay
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

// UnderlayAdded implements channel.UnderlayEventListener.
func (self *DialLinkChannel) UnderlayAdded(ch channel.Channel, underlay channel.Underlay) {
	self.BaseLinkChannel.UnderlayAdded(ch, underlay)
	self.changeCallback(self)
}

// UnderlayRemoved implements channel.UnderlayEventListener.
func (self *DialLinkChannel) UnderlayRemoved(ch channel.Channel, underlay channel.Underlay) {
	self.BaseLinkChannel.UnderlayRemoved(ch, underlay)
	self.connIteration.Add(1)
	self.changeCallback(self)
}

// NewListenerLinkChannel creates the listen-side grouped link channel. It dials nothing;
// the default underlay's Min: 1 closes the channel if the required underlay is lost.
func NewListenerLinkChannel(underlay channel.Underlay, payloadSenderQueueSize, ackSenderQueueSize int) *ListenerLinkChannel {
	result := &ListenerLinkChannel{
		BaseLinkChannel: NewBaseLinkChannel(underlay, payloadSenderQueueSize, ackSenderQueueSize),
		constraints: map[string]channel.UnderlayConstraint{
			ChannelTypeDefault: {Desired: 1, Min: 1},
			ChannelTypeAck:     {Desired: 1, Min: 0},
		},
	}

	return result
}

type ListenerLinkChannel struct {
	*BaseLinkChannel
	constraints map[string]channel.UnderlayConstraint
}

// GetConstraints returns the per-underlay-type constraints for the channel. The listener
// has no dial policy, so these only drive the close-when-below-Min check, not dialing.
func (self *ListenerLinkChannel) GetConstraints() map[string]channel.UnderlayConstraint {
	return self.constraints
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

func (self *SingleLinkChannel) InitChannel(ch channel.Channel) {
	self.ch = ch
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
