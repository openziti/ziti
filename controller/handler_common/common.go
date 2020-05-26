package handler_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/channel2"
)

func SendSuccess(request *channel2.Message, ch channel2.Channel, message string) {
	SendResult(request, ch, message, true)
}

func SendFailure(request *channel2.Message, ch channel2.Channel, message string) {
	SendResult(request, ch, message, false)
}

func SendResult(request *channel2.Message, ch channel2.Channel, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label())
	if !success {
		log.Errorf("%v error (%s)", ch.LogicalName(), message)
	}

	response := channel2.NewResult(success, message)
	response.ReplyTo(request)
	_ = ch.Send(response)
	log.Debug("success")
}
