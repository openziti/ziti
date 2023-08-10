package handler_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"time"
)

func SendSuccess(request *channel.Message, ch channel.Channel, message string) {
	SendResult(request, ch, message, true)
}

func SendFailure(request *channel.Message, ch channel.Channel, message string) {
	SendResult(request, ch, message, false)
}

func SendResult(request *channel.Message, ch channel.Channel, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label())
	if !success {
		log.Errorf("%v error (%s)", ch.LogicalName(), message)
	}

	response := channel.NewResult(success, message)
	response.ReplyTo(request)
	if err := response.WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
		log.WithError(err).Error("failed to send result")
	}
}

func SendOpResult(request *channel.Message, ch channel.Channel, op string, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label()).WithField("operation", op)
	if !success {
		log.Errorf("%v error performing %v: (%s)", ch.LogicalName(), op, message)
	}

	response := channel.NewResult(success, message)
	response.ReplyTo(request)
	if err := response.WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
		log.WithError(err).Error("failed to send result")
	}
}
