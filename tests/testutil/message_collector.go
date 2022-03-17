package testutil

import (
	"fmt"
	"github.com/openziti/channel"
	"github.com/openziti/fabric/pb/ctrl_pb"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"time"
)

func NewMessageCollector(id string) *MessageCollector {
	return &MessageCollector{
		id:       id,
		msgs:     make(chan *channel.Message, 16),
		decoders: []channel.TraceMessageDecoder{channel.Decoder{}, ctrl_pb.Decoder{}},
	}
}

type MessageCollector struct {
	id       string
	msgs     chan *channel.Message
	decoders []channel.TraceMessageDecoder
}

func (self *MessageCollector) HandleReceive(m *channel.Message, ch channel.Channel) {
	if m.ContentType == -33 {
		logrus.Debug("ignoring reconnect ping")
		return
	}
	select {
	case self.msgs <- m:
		decoded := fmt.Sprintf("ContentType: %v", m.ContentType)
		for _, decoder := range self.decoders {
			if val, ok := decoder.Decode(m); ok {
				decoded += ", decoded=" + string(val)
				break
			}
		}
		logrus.Infof("%v: received %v", self.id, decoded)
	case <-time.After(time.Second * 5):
		logrus.Error("timed out trying to queue message, closing channel")
		_ = ch.Close()
	}
}

func (self *MessageCollector) Next(timeout time.Duration) (*channel.Message, error) {
	select {
	case msg := <-self.msgs:
		return msg, nil
	case <-time.After(timeout):
		return nil, errors.New("timed out")
	}
}

func (self *MessageCollector) NoMessages(timeout time.Duration, req require.Assertions) {
	select {
	case msg := <-self.msgs:
		req.Nil(msg)
	case <-time.After(timeout):
	}
}
