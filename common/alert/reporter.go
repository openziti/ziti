/*
	Copyright NetFoundry Inc.

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

package alert

import (
	"container/list"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
)

const (
	EntityTypeService = "service"
	EntityTypeSelf    = "self"
	EntityTypeSelfErt = "ert"
)

type NetworkControllers interface {
	ForEach(f func(ctrlId string, ch channel.Channel))
}

type Severity string

const (
	SeverityError = "error"
)

type Reporter struct {
	controllers  NetworkControllers
	queue        *sizeLimitedQueue[*ctrl_pb.Alert]
	queueMutex   sync.Mutex
	maxBatchSize int
	routerId     string
	stopCh       chan struct{}
	alertCh      chan struct{}
	stopped      atomic.Bool
}

func NewAlertReporter(controllers NetworkControllers, routerId string, maxQueueSize, maxBatchSize int) *Reporter {
	if maxQueueSize <= 0 {
		maxQueueSize = 100
	}
	if maxBatchSize <= 0 {
		maxBatchSize = 10
	}

	logDroppedMsg := func(droppedAlert *ctrl_pb.Alert) {
		pfxlog.Logger().
			WithField("message", droppedAlert.Message).
			WithField("details", droppedAlert.Details).
			WithField("severity", droppedAlert.Severity).
			WithField("relatedEntities", droppedAlert.RelatedEntities).
			Warn("dropped oldest alert from queue due to capacity limit")
	}

	result := &Reporter{
		controllers:  controllers,
		queue:        newSizeLimitedQueue[*ctrl_pb.Alert](maxQueueSize, logDroppedMsg),
		maxBatchSize: maxBatchSize,
		routerId:     routerId,
		stopCh:       make(chan struct{}),
		alertCh:      make(chan struct{}, 10),
	}

	result.Start()

	return result
}

func (self *Reporter) Start() {
	go self.processAlerts()
}

func (self *Reporter) Stop() {
	if self.stopped.CompareAndSwap(false, true) {
		close(self.stopCh)
	}
}

func (self *Reporter) ReportError(message string, details []string, relatedEntities map[string]string) {
	self.ReportAlert(message, SeverityError, details, relatedEntities)
}

func (self *Reporter) ReportAlert(message string, severity Severity, details []string, relatedEntities map[string]string) {
	if self.stopped.Load() {
		return
	}

	alert := &ctrl_pb.Alert{
		SourceType:      "self",
		Timestamp:       time.Now().UnixMilli(),
		Severity:        string(severity),
		Message:         message,
		Details:         details,
		RelatedEntities: relatedEntities,
	}

	self.queue.Push(alert)

	select {
	case self.alertCh <- struct{}{}:
	default:
	}
}

func (self *Reporter) processAlerts() {
	for {
		select {
		case <-self.stopCh:
			return
		case <-self.alertCh:
			self.sendQueuedAlerts()
		}
	}
}

func (self *Reporter) sendQueuedAlerts() {
	for {
		batch := self.getBatch()
		if len(batch) == 0 {
			return
		}

		expBackoff := backoff.NewExponentialBackOff()
		expBackoff.InitialInterval = 1 * time.Second
		expBackoff.MaxInterval = 5 * time.Minute
		expBackoff.MaxElapsedTime = 0 // Never stop retrying

		operation := func() error {
			return self.sendAlertBatch(batch)
		}

		if err := backoff.Retry(operation, expBackoff); err != nil {
			pfxlog.Logger().
				WithError(err).
				WithField("batchSize", len(batch)).
				Error("failed to send alert batch after all retries")
		} else {
			pfxlog.Logger().
				WithField("batchSize", len(batch)).
				Debug("successfully sent alert batch to controller")
		}
	}
}

func (self *Reporter) getBatch() []*ctrl_pb.Alert {
	self.queueMutex.Lock()
	defer self.queueMutex.Unlock()

	if self.queue.Len() == 0 {
		return nil
	}

	batch := make([]*ctrl_pb.Alert, 0, min(self.maxBatchSize, self.queue.Len()))
	for len(batch) < self.maxBatchSize {
		alert, _ := self.queue.Pop()
		if alert == nil {
			break
		}
		batch = append(batch, alert)
	}

	return batch
}

func (self *Reporter) sendAlertBatch(alertMsgs []*ctrl_pb.Alert) error {
	alertsBatch := &ctrl_pb.Alerts{
		Alerts: alertMsgs,
	}

	successfulSend := false
	self.controllers.ForEach(func(ctrlId string, ch channel.Channel) {
		if successfulSend {
			return
		}

		pfxlog.Logger().
			WithField("ctrlId", ch.Id()).
			WithField("batchSize", len(alertMsgs)).
			Trace("sending alerts batch")

		if err := protobufs.MarshalTyped(alertsBatch).WithTimeout(time.Second).SendAndWaitForWire(ch); err != nil {
			pfxlog.Logger().
				WithField("ctrlId", ctrlId).
				WithError(err).
				Error("failed to send alerts batch to controller")
		} else {
			successfulSend = true
		}
	})

	if !successfulSend {
		if self.stopped.Load() {
			return backoff.Permanent(errors.New("alert reporter stopped"))
		}
		return errors.New("failed to send alert batch to any controller")
	}

	return nil
}

type sizeLimitedQueue[T any] struct {
	queue           *list.List
	maxSize         int
	lock            sync.Mutex
	discardCallback func(T)
}

func newSizeLimitedQueue[T any](maxSize int, discardCallback func(T)) *sizeLimitedQueue[T] {
	return &sizeLimitedQueue[T]{
		queue:           list.New(),
		maxSize:         maxSize,
		discardCallback: discardCallback,
	}
}

func (self *sizeLimitedQueue[T]) Len() int {
	self.lock.Lock()
	defer self.lock.Unlock()

	return self.queue.Len()
}

func (self *sizeLimitedQueue[T]) Pop() (T, bool) {
	self.lock.Lock()
	defer self.lock.Unlock()

	result := self.queue.Front()

	if result == nil {
		var defaultValue T
		return defaultValue, false
	}

	self.queue.Remove(result)
	return result.Value.(T), true
}

func (self *sizeLimitedQueue[T]) Push(v T) {
	self.lock.Lock()
	defer self.lock.Unlock()
	for self.queue.Len() >= self.maxSize {
		next, _ := self.Pop()
		self.queue.PushBack(next)
	}
	self.queue.PushBack(v)
}

func ErrListToDetailList(errList []error) []string {
	detailList := make([]string, 0, len(errList))
	for _, err := range errList {
		detailList = append(detailList, err.Error())
	}
	return detailList
}
