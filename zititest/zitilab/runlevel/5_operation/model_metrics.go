/*
	Copyright 2019 NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package zitilib_runlevel_5_operation

import (
	"encoding/json"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fabric/event"
	"github.com/openziti/fabric/pb/mgmt_pb"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/sirupsen/logrus"
	"time"
)

func ModelMetrics(closer <-chan struct{}) model.OperatingStage {
	return ModelMetricsWithIdMapper(closer, func(id string) string {
		return "#" + id
	})
}

func ModelMetricsWithIdMapper(closer <-chan struct{}, f func(string) string) model.OperatingStage {
	return &modelMetrics{
		closer:             closer,
		idToSelectorMapper: f,
	}
}

type modelMetrics struct {
	ch                 channel.Channel
	m                  *model.Model
	closer             <-chan struct{}
	idToSelectorMapper func(string) string
}

func (self *modelMetrics) Operate(run model.Run) error {
	self.m = run.GetModel()

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandler(int32(mgmt_pb.ContentType_StreamEventsEventType), channel.ReceiveHandlerF(self.handleMetricsMessages))
		return nil
	}

	ch, err := api.NewWsMgmtChannel(channel.BindHandlerF(bindHandler))
	if err != nil {
		panic(err)
	}
	self.ch = ch

	streamEventsRequest := map[string]interface{}{
		"format":        "json",
		"subscriptions": []*event.Subscription{{Type: event.MetricsEventsNs}},
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

func (self *modelMetrics) handleMetricsMessages(msg *channel.Message, _ channel.Channel) {
	evt := &event.MetricsEvent{}
	err := json.Unmarshal(msg.Body, evt)
	if err != nil {
		logrus.Error("error handling metrics receive (%w)", err)
	}

	hostSelector := self.idToSelectorMapper(evt.SourceAppId)
	host, err := self.m.SelectHost(hostSelector)
	if err == nil {
		modelEvent := self.toModelMetricsEvent(evt)
		self.m.AcceptHostMetrics(host, modelEvent)
		logrus.Infof("<$= [%s]", evt.SourceAppId)
	} else {
		logrus.Errorf("modelMetrics: unable to find host (%v)", err)
	}
}

func (self *modelMetrics) runMetrics() {
	logrus.Infof("starting")
	defer logrus.Infof("exiting")

	<-self.closer
	_ = self.ch.Close()
}

func (self *modelMetrics) toModelMetricsEvent(fabricEvent *event.MetricsEvent) *model.MetricsEvent {
	modelEvent := &model.MetricsEvent{
		Timestamp: fabricEvent.Timestamp,
		Metrics:   model.MetricSet{},
		Tags:      fabricEvent.Tags,
	}

	for name, val := range fabricEvent.Metrics {
		modelEvent.Metrics.AddGroupedMetric(fabricEvent.Metric, name, val)
	}

	return modelEvent
}
