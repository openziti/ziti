package handler_common

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/channel"
	"github.com/openziti/foundation/channel2"
)

func SendChannel2Success(request *channel2.Message, ch channel2.Channel, message string) {
	SendChannel2Result(request, ch, message, true)
}

func SendChannel2Failure(request *channel2.Message, ch channel2.Channel, message string) {
	SendChannel2Result(request, ch, message, false)
}

func SendChannel2Result(request *channel2.Message, ch channel2.Channel, message string, success bool) {
	log := pfxlog.ContextLogger(ch.Label())
	if !success {
		log.Errorf("%v error (%s)", ch.LogicalName(), message)
	}

	response := channel2.NewResult(success, message)
	response.ReplyTo(request)
	_ = ch.Send(response)
	log.Debug("success")
}

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
