// Package ctrlchan provides multi-underlay control channel support for router-controller communication.
//
// This package implements a priority-based messaging system over channel/v5's unified Channel,
// allowing control plane traffic to be distributed across multiple TCP connections (underlays)
// with different priority levels. This enables separation of time-sensitive control messages from
// bulk traffic like metrics.
//
// # Architecture
//
// A control channel uses a multi-underlay channel.Channel to manage multiple underlays:
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
//	cfg := channel.Config{
//	    Senders:                dialCtrlChan,
//	    MessageSourceProvider:  dialCtrlChan,
//	    DialPolicy:             dialCtrlChan.GetDialPolicy(),
//	    Constraints:            dialCtrlChan.GetConstraints(),
//	    UnderlayEventListeners: []channel.UnderlayEventListener{dialCtrlChan},
//	    ...
//	}
//
// Controller side (listening):
//
//	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()
package ctrlchan

import (
	"io"

	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v5"
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
// It implements the channel/v5 Senders and MessageSourceProvider interfaces, maintaining
// separate message queues for each priority level and routing messages to the appropriate
// underlay based on type.
//
// Message flow:
//   - Callers send via GetDefaultSender(), GetHighPrioritySender(), or GetLowPrioritySender()
//   - Each underlay calls GetMessageSource() to get its message retrieval function
//   - The retrieval function pulls from the appropriate queue(s) based on underlay type
type BaseCtrlChannel struct {
	channel.SenderContext
	ch                 channel.Channel
	highPrioritySender channel.Sender
	defaultSender      channel.Sender
	lowPrioritySender  channel.Sender

	highPriorityMsgChan chan channel.Sendable
	defaultMsgChan      chan channel.Sendable
	lowPriorityMsgChan  chan channel.Sendable

	hasHighPriorityChan atomic.Bool
}

// InitChannel records the channel. It must be called from the bind handler, which runs on
// the construction goroutine before the channel is published, so a plain field is safe: the
// reference is invariant and set before any reader. The router relies on this because Add()
// registers the channel (firing ControllerAdded listeners that dereference Channel()) from
// within the bind handler, before UnderlayAdded fires - the C3 ordering hazard.
func (self *BaseCtrlChannel) InitChannel(ch channel.Channel) {
	self.ch = ch
}

func (self *BaseCtrlChannel) Close() error {
	if self.ch != nil {
		return self.ch.Close()
	}
	return nil
}

func (self *BaseCtrlChannel) IsClosed() bool {
	return self.ch == nil || self.ch.IsClosed()
}

func (self *BaseCtrlChannel) GetChannel() channel.Channel {
	return self.ch
}

func (self *BaseCtrlChannel) PeerId() string {
	return self.GetChannel().Id()
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
	if self.hasHighPriorityChan.Load() {
		select {
		case msg := <-self.defaultMsgChan:
			return msg, nil
		case msg := <-self.lowPriorityMsgChan:
			return msg, nil
		case <-self.GetCloseNotify():
			return nil, io.EOF
		case <-notifier.GetCloseNotify():
			return nil, io.EOF
		}
	} else {
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
}

func (self *BaseCtrlChannel) GetHighPriorityMsg(notifier *channel.CloseNotifier) (channel.Sendable, error) {
	select {
	case msg := <-self.highPriorityMsgChan:
		return msg, nil
	case <-self.GetCloseNotify():
		return nil, io.EOF
	case <-notifier.GetCloseNotify():
		return nil, io.EOF
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

// GetMessageSource implements channel.MessageSourceProvider, returning the message
// retrieval function for the given underlay type.
func (self *BaseCtrlChannel) GetMessageSource(underlayType string) channel.MessageSourceF {
	if underlayType == ChannelTypeHighPriority {
		return self.GetHighPriorityMsg
	}
	if underlayType == ChannelTypeLowPriority {
		return self.GetLowPriorityMsg
	}
	return self.GetNextMsgDefault
}

func (self *BaseCtrlChannel) HandleTxFailed(_ string, _ channel.Sendable) bool {
	// control channel senders know how to handle send failures. If we retry under the hood,
	// we introduce the possibility of unexpected ordering changes. Some subsystems, like
	// link management depend on in order delivery of messages
	return false
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
// It supplies a channel.DialPolicy and declarative constraints; the channel actively
// re-establishes underlays when they are lost. The underlays all use Min: 0, so the
// channel survives dropping to zero underlays and re-dials with a fresh group iteration.
func NewDialCtrlChannel(config DialCtrlChannelConfig) *DialCtrlChannel {
	result := &DialCtrlChannel{
		BaseCtrlChannel: NewBaseCtrlChannel(),
		changeCallback:  config.UnderlayChangeCallback,
		startupDelay:    config.StartupDelay,
		constraints: map[string]channel.UnderlayConstraint{
			ChannelTypeDefault:      {Desired: config.MaxDefaultChannels, Min: 0},
			ChannelTypeHighPriority: {Desired: config.MaxHighPriorityChannels, Min: 0},
			ChannelTypeLowPriority:  {Desired: config.MaxLowPriorityChannels, Min: 0},
		},
	}

	// The control channel prioritizes prompt reconnection. MinDialInterval paces redials
	// (the flap protection the v4 lastDial throttle provided), and dial *failures* still
	// accrue exponential backoff for an unreachable controller. But short-lived-connection
	// detection is disabled (MinStableDuration = 0): a clean close - controller restart,
	// deploy, transient blip - must reconnect right away rather than being treated as a flap
	// and backed off, which would add control-plane downtime. (#252 made short-lived
	// detection apply to reconnect-from-zero dials, which for the survive-to-zero ctrl
	// channel is exactly the reconnect path we want to keep fast.)
	backoffConfig := channel.DefaultBackoffConfig
	backoffConfig.MinDialInterval = time.Second
	backoffConfig.MinStableDuration = 0
	result.dialPolicy = channel.NewBackoffDialPolicyWithConfig(config.Dialer, backoffConfig)

	return result
}

// CtrlChannel provides access to priority-based message senders for control traffic.
type CtrlChannel interface {
	InitChannel(ch channel.Channel)
	PeerId() string
	GetChannel() channel.Channel
	GetDefaultSender() channel.Sender
	GetHighPrioritySender() channel.Sender
	GetLowPrioritySender() channel.Sender
	IsConnected() bool
	Close() error
	IsClosed() bool
}

// DialCtrlChannel implements CtrlChannel for the dialing side (router). The channel's
// DialPolicy maintains the desired underlay counts; this type tracks connectivity and
// notifies callers of changes via the callback.
type DialCtrlChannel struct {
	*BaseCtrlChannel
	dialPolicy     channel.DialPolicy
	constraints    map[string]channel.UnderlayConstraint
	changeCallback func(ch *DialCtrlChannel, oldCount, newCount uint32)
	startupDelay   time.Duration

	underlayCount atomic.Uint32
}

// GetDialPolicy returns the channel.DialPolicy used to (re)establish underlays.
func (self *DialCtrlChannel) GetDialPolicy() channel.DialPolicy {
	return self.dialPolicy
}

// GetConstraints returns the per-underlay-type constraints for the channel.
func (self *DialCtrlChannel) GetConstraints() map[string]channel.UnderlayConstraint {
	return self.constraints
}

// GetStartupDelay returns the delay before the channel begins dialing additional underlays.
func (self *DialCtrlChannel) GetStartupDelay() time.Duration {
	return self.startupDelay
}

// IsConnected returns true if the dial-side ctrl channel has at least one active underlay.
func (self *DialCtrlChannel) IsConnected() bool {
	return self.underlayCount.Load() != 0
}

// UnderlayAdded implements channel.UnderlayEventListener.
func (self *DialCtrlChannel) UnderlayAdded(ch channel.Channel, underlay channel.Underlay) {
	if channel.GetUnderlayType(underlay) == ChannelTypeHighPriority {
		self.hasHighPriorityChan.Store(true)
	}

	newCount := totalUnderlays(ch)
	oldCount := self.underlayCount.Swap(newCount)
	self.changeCallback(self, oldCount, newCount)
}

// UnderlayRemoved implements channel.UnderlayEventListener.
func (self *DialCtrlChannel) UnderlayRemoved(ch channel.Channel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		WithField("channelClosed", ch.IsClosed()).
		Info("underlay closed")

	if ch.GetUnderlayCountsByType()[ChannelTypeHighPriority] == 0 {
		self.hasHighPriorityChan.Store(false)
	}

	newCount := totalUnderlays(ch)
	oldCount := self.underlayCount.Swap(newCount)
	self.changeCallback(self, oldCount, newCount)
}

// NewListenerCtrlChannel creates a control channel handler for the listening side (controller).
// It dials no underlays; with no constraints and no dial policy the channel closes when its
// last underlay is lost, which is the desired controller-side behavior.
func NewListenerCtrlChannel() *ListenerCtrlChannel {
	return &ListenerCtrlChannel{
		BaseCtrlChannel: NewBaseCtrlChannel(),
	}
}

// ListenerCtrlChannel implements CtrlChannel for the listening side (controller). Unlike
// DialCtrlChannel it does not dial; the channel closes if all underlays are lost.
type ListenerCtrlChannel struct {
	*BaseCtrlChannel
}

// UnderlayAdded implements channel.UnderlayEventListener.
func (self *ListenerCtrlChannel) UnderlayAdded(ch channel.Channel, underlay channel.Underlay) {
	if channel.GetUnderlayType(underlay) == ChannelTypeHighPriority {
		self.hasHighPriorityChan.Store(true)
	}
}

// UnderlayRemoved implements channel.UnderlayEventListener.
func (self *ListenerCtrlChannel) UnderlayRemoved(ch channel.Channel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay closed")

	if ch.GetUnderlayCountsByType()[ChannelTypeHighPriority] == 0 {
		self.hasHighPriorityChan.Store(false)
	}
}

// IsConnected returns true if the listener-side ctrl channel has not been closed.
func (self *ListenerCtrlChannel) IsConnected() bool {
	// when the listener loses its last underlay, the channel is closed
	return !self.IsClosed()
}

// totalUnderlays sums the underlay counts across all types.
func totalUnderlays(ch channel.Channel) uint32 {
	total := uint32(0)
	for _, count := range ch.GetUnderlayCountsByType() {
		total += uint32(count)
	}
	return total
}
