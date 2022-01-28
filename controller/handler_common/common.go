package handler_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel"
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
	_ = ch.Send(response)
	log.Debug("success")
}
