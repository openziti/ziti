// Package ctrlchan provides multi-underlay control channel support for router-controller communication.
//
// This package implements a priority-based messaging system over channel.MultiChannel, allowing
// control plane traffic to be distributed across multiple TCP connections (underlays) with
// different priority levels. This enables separation of time-sensitive control messages from
// bulk traffic like metrics.
//
// # Architecture
//
// A control channel uses channel.MultiChannel to manage multiple underlays:
//   - Default underlay: Carries normal control traffic (terminators, metrics, etc.)
//   - High-priority underlay: Reserved for time-sensitive messages (heartbeats, routing, circuit requests)
//   - Low-priority underlay: For bulk/background traffic (inspections, file-transfers)
//
// Messages are routed to senders by priority level. Each underlay pulls from its designated
// message queue, with fallback behavior when dedicated underlays aren't available.
//
// # Usage
//
// Router side (dialing):
//
//	dialCtrlChan := ctrlchan.NewDialCtrlChannel(ctrlchan.DialCtrlChannelConfig{
//	    Dialer:                  dialer,
//	    MaxDefaultChannels:      1,
//	    MaxHighPriorityChannels: 1,  // Set to 0 if controller doesn't support multi-underlay
//	    MaxLowPriorityChannels:  0,
//	    UnderlayChangeCallback:  changeCallback,
//	})
//
// Controller side (listening):
//
//	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()
//
// Both implementations satisfy CtrlChannelUnderlayHandler which combines CtrlChannel
// (for sending messages) with channel.UnderlayHandler (for MultiChannel integration).
package ctrlchan

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/foundation/v2/concurrenz"
)

// Channel type constants identify the priority level of each underlay connection.
// These are used as the TypeHeader value when establishing grouped underlays.
const (
	ChannelTypeDefault      string = "ctrl.default"
	ChannelTypeHighPriority string = "ctrl.high"
	ChannelTypeLowPriority  string = "ctrl.low"
)

// NewBaseCtrlChannel creates the base control channel with priority message queues.
func NewBaseCtrlChannel() *BaseCtrlChannel {
	senderContext := channel.NewSenderContext()

	defaultMsgChan := make(chan channel.Sendable, 16)
	highPriorityMsgChan := make(chan channel.Sendable, 16)
	lowPriorityMsgChan := make(chan channel.Sendable, 16)

	result := &BaseCtrlChannel{
		SenderContext:       senderContext,
		defaultSender:       channel.NewSingleChSender(senderContext, defaultMsgChan),
		highPrioritySender:  channel.NewSingleChSender(senderContext, highPriorityMsgChan),
		lowPrioritySender:   channel.NewSingleChSender(senderContext, lowPriorityMsgChan),
		defaultMsgChan:      defaultMsgChan,
		highPriorityMsgChan: highPriorityMsgChan,
		lowPriorityMsgChan:  lowPriorityMsgChan,
	}
	return result
}

// BaseCtrlChannel provides the core priority-based message routing for control channels.
// It maintains separate message queues for each priority level and routes messages
// to the appropriate underlay based on type.
//
// Message flow:
//   - Callers send via GetDefaultSender(), GetHighPrioritySender(), or GetLowPrioritySender()
//   - Each underlay calls GetMessageSource() to get its message retrieval function
//   - The retrieval function pulls from the appropriate queue(s) based on underlay type
type BaseCtrlChannel struct {
	ch channel.MultiChannel
	channel.SenderContext
	highPrioritySender channel.Sender
	defaultSender      channel.Sender
	lowPrioritySender  channel.Sender

	highPriorityMsgChan chan channel.Sendable
	defaultMsgChan      chan channel.Sendable
	lowPriorityMsgChan  chan channel.Sendable

	hasDefaultChan atomic.Bool
	underlayCount  atomic.Uint32
}

func (self *BaseCtrlChannel) ChannelCreated(ch channel.MultiChannel) {
	self.ch = ch
}

func (self *BaseCtrlChannel) Close() error {
	return self.ch.Close()
}

func (self *BaseCtrlChannel) IsClosed() bool {
	return self.ch.IsClosed()
}

func (self *BaseCtrlChannel) GetChannel() channel.Channel {
	return self.ch
}

func (self *BaseCtrlChannel) PeerId() string {
	return self.GetChannel().Id()
}

func (self *BaseCtrlChannel) IsConnected() bool {
	return self.underlayCount.Load() != 0
}

func (self *BaseCtrlChannel) GetDefaultSender() channel.Sender {
	return self.defaultSender
}

func (self *BaseCtrlChannel) GetHighPrioritySender() channel.Sender {
	return self.highPrioritySender
}

func (self *BaseCtrlChannel) GetLowPrioritySender() channel.Sender {
	return self.lowPrioritySender
}

func (self *BaseCtrlChannel) GetNextMsgDefault(notifier *channel.CloseNotifier) (channel.Sendable, error) {
	select {
	case msg := <-self.defaultMsgChan:
		return msg, nil
	case msg := <-self.highPriorityMsgChan:
		return msg, nil
	case msg := <-self.lowPriorityMsgChan:
		return msg, nil
	case <-self.GetCloseNotify():
		return nil, io.EOF
	case <-notifier.GetCloseNotify():
		return nil, io.EOF
	}
}

func (self *BaseCtrlChannel) GetHighPriorityMsg(notifier *channel.CloseNotifier) (channel.Sendable, error) {
	if self.hasDefaultChan.Load() {
		select {
		case msg := <-self.highPriorityMsgChan:
			return msg, nil
		case <-self.GetCloseNotify():
			return nil, io.EOF
		case <-notifier.GetCloseNotify():
			return nil, io.EOF
		}
	} else {
		select {
		case msg := <-self.highPriorityMsgChan:
			return msg, nil
		case msg := <-self.defaultMsgChan:
			return msg, nil
		case <-self.GetCloseNotify():
			return nil, io.EOF
		case <-notifier.GetCloseNotify():
			return nil, io.EOF
		}
	}
}

func (self *BaseCtrlChannel) GetLowPriorityMsg(notifier *channel.CloseNotifier) (channel.Sendable, error) {
	select {
	case msg := <-self.lowPriorityMsgChan:
		return msg, nil
	case <-self.GetCloseNotify():
		return nil, io.EOF
	case <-notifier.GetCloseNotify():
		return nil, io.EOF
	}
}

func (self *BaseCtrlChannel) GetMessageSource(underlay channel.Underlay) channel.MessageSourceF {
	if channel.GetUnderlayType(underlay) == ChannelTypeHighPriority {
		return self.GetHighPriorityMsg
	}
	if channel.GetUnderlayType(underlay) == ChannelTypeLowPriority {
		return self.GetLowPriorityMsg
	}
	return self.GetNextMsgDefault
}

func (self *BaseCtrlChannel) HandleTxFailed(_ channel.Underlay, sendable channel.Sendable) bool {
	select {
	case self.defaultMsgChan <- sendable:
		return true
	default:
		return false
	}
}

// DialCtrlChannelConfig configures the dialing side of a control channel (router side).
type DialCtrlChannelConfig struct {
	// Dialer creates new underlay connections to the controller.
	Dialer channel.DialUnderlayFactory

	// MaxDefaultChannels is the target number of default priority underlays (typically 1).
	MaxDefaultChannels int

	// MaxHighPriorityChannels is the target number of high priority underlays.
	// Set to 1 if controller supports multi-underlay, 0 otherwise.
	MaxHighPriorityChannels int

	// MaxLowPriorityChannels is the target number of low priority underlays. Current 0, but anticipated to be used in future
	MaxLowPriorityChannels int

	// StartupDelay delays additional underlay establishment after the initial connection.
	StartupDelay time.Duration

	// UnderlayChangeCallback is invoked when the total underlay count changes.
	UnderlayChangeCallback func(ch *DialCtrlChannel, oldCount, newCount uint32)
}

// NewDialCtrlChannel creates a control channel handler for the dialing side (router).
// The handler manages underlay constraints and automatically re-establishes connections
// when underlays are lost.
func NewDialCtrlChannel(config DialCtrlChannelConfig) CtrlChannelUnderlayHandler {
	result := &DialCtrlChannel{
		BaseCtrlChannel: *NewBaseCtrlChannel(),
		dialer:          config.Dialer,
		changeCallback:  config.UnderlayChangeCallback,
		startupDelay:    config.StartupDelay,
	}

	// Router side allows underlay count to drop to 0 while keeping channel open
	// The constraints will actively attempt to re-establish connections
	result.constraints.AddConstraint(ChannelTypeDefault, config.MaxDefaultChannels, 0)
	result.constraints.AddConstraint(ChannelTypeHighPriority, config.MaxHighPriorityChannels, 0)
	result.constraints.AddConstraint(ChannelTypeLowPriority, config.MaxLowPriorityChannels, 0)

	return result
}

// CtrlChannelUnderlayHandler combines CtrlChannel with channel.UnderlayHandler.
// Implementations handle both message routing and underlay lifecycle management.
type CtrlChannelUnderlayHandler interface {
	CtrlChannel
	channel.UnderlayHandler
}

// CtrlChannel provides access to priority-based message senders for control traffic.
type CtrlChannel interface {
	PeerId() string
	GetChannel() channel.Channel
	GetDefaultSender() channel.Sender
	GetHighPrioritySender() channel.Sender
	GetLowPrioritySender() channel.Sender
	IsConnected() bool
	Close() error
	IsClosed() bool
}

// DialCtrlChannel implements CtrlChannelUnderlayHandler for the dialing side (router).
// It manages underlay constraints, automatically re-establishes lost connections,
// and notifies callers of connectivity changes via the callback.
type DialCtrlChannel struct {
	BaseCtrlChannel
	dialer         channel.DialUnderlayFactory
	constraints    channel.UnderlayConstraints
	changeCallback func(ch *DialCtrlChannel, oldCount, newCount uint32)
	startupDelay   time.Duration

	lock      sync.Mutex
	iteration atomic.Uint32
	lastDial  time.Time
	lastClose concurrenz.AtomicValue[time.Time]
}

func (self *DialCtrlChannel) Start(channel channel.MultiChannel) {
	if self.startupDelay == 0 {
		self.constraints.Apply(channel, self)
	} else {
		time.AfterFunc(self.startupDelay, func() {
			self.constraints.Apply(channel, self)
		})
	}
}

func (self *DialCtrlChannel) HandleUnderlayClose(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		WithField("channelClosed", ch.IsClosed()).
		Info("underlay closed")

	// Track if all underlays are gone so we know to treat next connection as first
	totalUnderlays := uint32(0)
	underlayCounts := ch.GetUnderlayCountsByType()
	if underlayCounts[ChannelTypeDefault] == 0 {
		self.hasDefaultChan.Store(false)
	}

	for _, count := range underlayCounts {
		totalUnderlays += uint32(count)
	}
	oldCount := self.underlayCount.Swap(totalUnderlays)

	self.changeCallback(self, oldCount, totalUnderlays)

	self.lastClose.Store(time.Now())
	self.constraints.Apply(ch, self)
}

func (self *DialCtrlChannel) HandleUnderlayAccepted(ch channel.MultiChannel, underlay channel.Underlay) {
	if underlayType := channel.GetUnderlayType(underlay); underlayType == ChannelTypeDefault {
		self.hasDefaultChan.Store(true)
	}

	totalUnderlays := uint32(0)
	for _, count := range ch.GetUnderlayCountsByType() {
		totalUnderlays += uint32(count)
	}
	oldCount := self.underlayCount.Swap(totalUnderlays)

	self.changeCallback(self, oldCount, totalUnderlays)
}

func (self *DialCtrlChannel) DialFailed(ch channel.MultiChannel, _ string, attempt int) {
	delay := 2 * time.Duration(attempt) * time.Second
	if delay > time.Minute {
		delay = time.Minute
	}
	time.Sleep(delay)

	// if the constraints are no longer valid after sleeping, close the channel
	self.constraints.CheckStateValid(ch, true)
}

func (self *DialCtrlChannel) CreateGroupedUnderlay(groupId string, groupSecret []byte, underlayType string, timeout time.Duration) (channel.Underlay, error) {
	self.lock.Lock()
	defer self.lock.Unlock()

	defer func() { self.lastDial = time.Now() }()

	if time.Since(self.lastDial) < time.Second {
		time.Sleep(time.Second - time.Since(self.lastDial))
	}

	if time.Since(self.lastClose.Load()) < time.Second {
		time.Sleep(time.Second - time.Since(self.lastClose.Load()))
	}

	newIteration := self.underlayCount.Load() == 0
	iteration := self.iteration.Load()
	if newIteration {
		iteration = self.iteration.Add(1)
	}
	iterGroupId := groupId
	if self.iteration.Load() > 0 {
		iterGroupId = fmt.Sprintf("%s-%d", groupId, iteration)
	}
	headers := channel.Headers{
		channel.TypeHeader:         []byte(underlayType),
		channel.ConnectionIdHeader: []byte(iterGroupId),
		channel.GroupSecretHeader:  groupSecret,
		channel.IsGroupedHeader:    {1},
	}

	// If we've never had underlays or lost all of them, mark this as first connection
	if newIteration {
		headers.PutBoolHeader(channel.IsFirstGroupConnection, true)
	}

	return self.dialer.CreateWithHeaders(timeout, headers)
}

// NewListenerCtrlChannel creates a control channel handler for the listening side (controller).
// The controller side requires at least one default underlay to remain connected.
func NewListenerCtrlChannel() CtrlChannelUnderlayHandler {
	result := &ListenerCtrlChannel{
		BaseCtrlChannel: *NewBaseCtrlChannel(),
	}

	result.constraints.SetMinTotal(1)

	return result
}

// ListenerCtrlChannel implements CtrlChannelUnderlayHandler for the listening side (controller).
// Unlike DialCtrlChannel, it requires at least one underlay to remain connected and will
// close the channel if all underlays are lost.
type ListenerCtrlChannel struct {
	BaseCtrlChannel
	constraints channel.UnderlayConstraints
}

func (self *ListenerCtrlChannel) Start(channel channel.MultiChannel) {
	self.constraints.CheckStateValid(channel, true)
}

func (self *ListenerCtrlChannel) HandleUnderlayClose(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay closed")
	self.constraints.CheckStateValid(ch, true)
}

func (self *ListenerCtrlChannel) HandleUnderlayAccepted(channel.MultiChannel, channel.Underlay) {
}
