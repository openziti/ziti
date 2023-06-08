package zitilib_runlevel_5_operation

import (
	"encoding/json"
	"github.com/Jeffail/gabs/v2"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/channel/v2/protobufs"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fabric/event"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/sirupsen/logrus"
	"time"
)

func CircuitMetrics(pollFreq time.Duration, closer <-chan struct{}, f func(string) string) model.OperatingStage {
	return &circuitMetrics{
		closer:             closer,
		circuits:           map[string]struct{}{},
		eventC:             make(chan func(), 10),
		pollFreq:           pollFreq,
		idToSelectorMapper: f,
	}
}

type circuitMetrics struct {
	ch                 channel.Channel
	model              *model.Model
	eventC             chan func()
	closer             <-chan struct{}
	circuits           map[string]struct{}
	pollFreq           time.Duration
	idToSelectorMapper func(string) string
}

func (self *circuitMetrics) Operate(run model.Run) error {
	self.model = run.GetModel()
	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_StreamEventsEventType), channel.ReceiveHandlerF(self.receiveCircuitEvents))
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_InspectResponseType), channel.ReceiveHandlerF(self.receiveCircuitInspectResults))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		panic(err)
	}
	self.ch = ch

	streamEventsRequest := map[string]interface{}{
		"format":        "json",
		"subscriptions": []*event.Subscription{{Type: event.CircuitEventsNs}},
	}

	msgBytes, err := json.Marshal(streamEventsRequest)
	if err != nil {
		return err
	}

	requestMsg := channel.NewMessage(int32(mgmt_pb.ContentType_StreamEventsRequestType), msgBytes)
	if err = requestMsg.WithTimeout(5 * time.Second).SendAndWaitForWire(ch); err != nil {
		return err
	}
	go self.runMetrics()
	return nil
}

func (self *circuitMetrics) receiveCircuitEvents(msg *channel.Message, _ channel.Channel) {
	log := pfxlog.Logger()
	circuitEvent := &event.CircuitEvent{}
	err := json.Unmarshal(msg.Body, &circuitEvent)
	if err != nil {
		panic(err)
	}

	if circuitEvent.EventType == event.CircuitDeleted {
		self.eventC <- func() {
			delete(self.circuits, circuitEvent.CircuitId)
			log.Infof("circuit removed: %v", circuitEvent.CircuitId)
		}
	} else if circuitEvent.EventType == event.CircuitCreated {
		self.eventC <- func() {
			self.circuits[circuitEvent.CircuitId] = struct{}{}
			log.Infof("circuit added: %v, path: %v", circuitEvent.CircuitId, circuitEvent.Path)
		}
	} else if circuitEvent.EventType == event.CircuitUpdated {
		log.Infof("circuit updated: %v, path: %v", circuitEvent.CircuitId, circuitEvent.Path)
	}
}

func (self *circuitMetrics) receiveCircuitInspectResults(msg *channel.Message, _ channel.Channel) {
	log := pfxlog.Logger()
	response := &mgmt_pb.InspectResponse{}
	if err := protobufs.TypedResponse(response).Unmarshall(msg, nil); err != nil {
		log.WithError(err).Error("error unmarshalling inspect response")
		return
	}

	for _, errStr := range response.Errors {
		pfxlog.Logger().WithField(logrus.ErrorKey, errStr).Error("error reported by inspect")
	}

	for _, val := range response.Values {
		c, err := gabs.ParseJSON([]byte(val.Value))
		if err != nil {
			log.WithError(err).Errorf("unable to parse inspect JSON: %v", val.Value)
		} else {
			self.ingestCircuitMetrics(val.AppId, api.Wrap2(c))
		}
	}
}

func (self *circuitMetrics) runMetrics() {
	logrus.Infof("starting")
	defer logrus.Infof("exiting")

	ticker := time.NewTicker(self.pollFreq)
	defer ticker.Stop()

	for {
		select {
		case <-self.closer:
			_ = self.ch.Close()
			return
		case evt := <-self.eventC:
			evt()
		case <-ticker.C:
			self.requestCircuitMetrics()
		}
	}
}

func (self *circuitMetrics) requestCircuitMetrics() {
	if len(self.circuits) > 0 {
		inspectRequest := &mgmt_pb.InspectRequest{}
		for circuitId := range self.circuits {
			inspectRequest.RequestedValues = append(inspectRequest.RequestedValues, "circuit:"+circuitId)
		}
		if err := protobufs.MarshalTyped(inspectRequest).Send(self.ch); err != nil {
			pfxlog.Logger().WithError(err).Error("failed to send circuit inspect request")
		}
	}
}

func (self *circuitMetrics) ingestCircuitMetrics(sourceId string, circuitDetail *api.Gabs2Wrapper) {
	log := pfxlog.Logger()
	circuitId := circuitDetail.String("circuitId")
	xgDetails := circuitDetail.Path("xgressDetails")
	if xgDetails == nil {
		return
	}
	for _, child := range xgDetails.ChildrenMap() {
		xg := api.Wrap2(child)
		modelEvent := &model.MetricsEvent{
			Timestamp: time.Now(),
			Metrics:   model.MetricSet{},
			Tags: map[string]string{
				"circuitId":  circuitId,
				"originator": xg.String("originator"),
			},
		}

		if sendBufferDetail := xg.Path("sendBufferDetail"); sendBufferDetail != nil {
			for k, val := range sendBufferDetail.ChildrenMap() {
				modelEvent.Metrics["circuit.sendBuffer."+k] = val.Data()
			}
		}

		if recvBufferDetail := xg.Path("recvBufferDetail"); recvBufferDetail != nil {
			for k, val := range recvBufferDetail.ChildrenMap() {
				modelEvent.Metrics["circuit.recvBuffer."+k] = val.Data()
			}
		}

		hostSelector := self.idToSelectorMapper(sourceId)
		host, err := self.model.SelectHost(hostSelector)
		if err == nil {
			self.model.AcceptHostMetrics(host, modelEvent)
			log.Infof("<$= [%s/%v]", sourceId, circuitId)
		} else {
			log.WithError(err).Error("circuitMetrics: unable to find host")
		}
	}
}
