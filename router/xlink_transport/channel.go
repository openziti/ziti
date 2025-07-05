package xlink_transport

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"io"
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

func (self *BaseLinkChannel) HandleUnderlayAccepted(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay added")
}

type DialLinkChannelConfig struct {
	Dialer                 channel.DialUnderlayFactory
	Underlay               channel.Underlay
	MaxDefaultChannels     int
	MaxAckChannel          int
	UnderlayChangeCallback func(ch *DialLinkChannel)
}

func NewDialLinkChannel(config DialLinkChannelConfig) UnderlayHandlerLinkChannel {
	result := &DialLinkChannel{
		BaseLinkChannel: *NewBaseLinkChannel(config.Underlay),
		dialer:          config.Dialer,
		changeCallback:  config.UnderlayChangeCallback,
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
}

type DialLinkChannel struct {
	BaseLinkChannel
	dialer         channel.DialUnderlayFactory
	constraints    channel.UnderlayConstraints
	changeCallback func(ch *DialLinkChannel)
}

func (self *DialLinkChannel) Start(channel channel.MultiChannel) {
	self.constraints.Apply(channel, self)
}

func (self *DialLinkChannel) HandleUnderlayClose(ch channel.MultiChannel, underlay channel.Underlay) {
	pfxlog.Logger().
		WithField("id", ch.Label()).
		WithField("underlays", ch.GetUnderlayCountsByType()).
		WithField("underlayType", channel.GetUnderlayType(underlay)).
		Info("underlay closed")

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
