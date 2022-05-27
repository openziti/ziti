/*
	Copyright NetFoundry, Inc.

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

package persistence

import (
	"fmt"
	"github.com/kataras/go-events"
	"github.com/lucsky/cuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/storage/boltz"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"sync"
	"sync/atomic"
	"time"
)

// An EventualEventer provides a method for storing events in a persistent manner
// that will be processed at a later date. Processing may include time intensive processing such
// as bulk deletion of other entities. Event persistence strategy, processing order, and processing
// synchronization are up to the implementation to decide.
//
// EventualEventers are also required to emit a series of events via the events.EventEmitter
// interface. See EventualEventAdded and subsequent events for more details.
type EventualEventer interface {
	// EventEmmiter is used to provide processing event status on processing state, which is useful
	// for instrumenting an EventualEventer for metric purposes (process runtime, process batch runtime,
	// event counts, etc.)
	events.EventEmmiter

	// AddEventualEvent adds an eventual event with a specific name and byte array data payload. Interpretation
	// of the event's data payload is upto the event emitter and consumer.
	AddEventualEvent(eventType string, data []byte)

	// AddEventualListener adds a function as call back when an eventual event is processed.
	AddEventualListener(eventType string, handler EventListenerFunc)

	// Start should be called at the start of the lifetime of the EventualEventer.
	// A closeNotify channel must be supplied for application shutdown eventing.
	//
	// If an EventualEventer has already been started, it will return an error.
	// Errors may be returned for other reasons causing Start to fail.
	Start(closeNotify <-chan struct{}) error

	// Stop may be called to manually end of the lifetime of the EventualEventer outside the
	// closeNotify signaling provided in the Start call. If not started, an error will be returned.
	// Errors may be returned for other reasons causing Stop to fail.
	Stop() error

	// Trigger forces an EventualEventer to check for work to be processed. Beyond this method,
	// it is the implementation's responsibility to provide other mechanisms or logic to determine
	// when work is performed (timers, events, etc.) which may be setup/torn down during Start/Stop.
	//
	// If the EventualEventer is not currently running or can't process work and error will
	// be returned. If it is running a channel will be returned which will be closed after
	// the current or next iteration of the event processor has completed.
	Trigger() (<-chan struct{}, error)
}

// EventListenerFunc is a function handler that will be triggered asynchronously some point in the future
type EventListenerFunc func(name string, data []byte)

type EventualEventAdded struct {
	// Id is a unique id for the event created
	Id string

	// Total is the total number of eventual events awaiting processing
	Total int64
}

type EventualEventRemoved struct {
	// Id is a unique id for the event deleted
	Id string

	// Total is the total number of eventual events awaiting processing
	Total int64
}

type EventualEventProcessingStart struct {
	// Id is a unique id for processing run
	Id string

	// StartTime is the time the processing began
	StartTime time.Time
}

type EventualEventProcessingBatchStart struct {
	// Id is a unique id for the batch
	Id string

	// Id is the unique processing run this batch is a member of
	ProcessId string

	// Count is the number of events in the current batch
	Count int

	// BatchSize is the batch size for the current batch (the maximum value of Count)
	BatchSize int

	// StartTime the time when the batch started processing
	StartTime time.Time
}

type EventualEventProcessingListenerStart struct {
	// Id is a unique id for the triggering of a listener
	Id string

	// BatchId is the unique id of the batch being processed
	BatchId string

	// ProcessId is the unique id of the currently executing process
	ProcessId string

	// ListenerFunc is the listener that was executed
	ListenerFunc EventListenerFunc

	// BatchEventIndex is the zero based offset of the currently executing event
	BatchEventIndex int64

	// TotalEventIndex is the total index across all batches of the currently executing event
	TotalEventIndex int64

	// EventType is the typeof the event that is triggering the listener
	EventType string

	// StartTime is the time when the listener was started
	StartTime time.Time
}

type EventualEventProcessingListenerDone struct {
	// Id is a unique id for the triggering of a listener
	Id string

	// BatchId is the unique id of the batch being processed
	BatchId string

	// ProcessId is the unique id of the currently executing process
	ProcessId string

	// ListenerFunc is the listener that was executed
	ListenerFunc EventListenerFunc

	// BatchEventIndex is the zero based offset of the currently executing event
	BatchEventIndex int64

	// TotalEventIndex is the total index across all batches of the currently executing event
	TotalEventIndex int64

	// Error is nil if no error occurred during execution, otherwise an error value
	Error error

	// EventType is the typeof the event that triggered the listener
	EventType string

	// StartTime is the time when the listener started execution
	StartTime time.Time

	// EndTime is the time when the listener ended execution
	EndTime time.Time
}

type EventualEventProcessingBatchDone struct {
	// Id is a unique id for the batch
	Id string

	// Id is the unique processing run this batch is a member of
	ProcessId string

	// Count is the number of events in the current batch
	Count int

	// BatchSize is the batch size for the current batch (the maximum value of Count)
	BatchSize int

	// StartTime the time the batch was started
	StartTime time.Time

	// EndTime the time the batch ended
	EndTime time.Time
}

type EventualEventProcessingDone struct {
	// Id is a unique id for processing run
	Id string

	// TotalBatches is the total number of batches executed during processing
	TotalBatches int64

	// TotalEvent is the total number of events processed
	TotalEvents int64

	// TotalListenersExecuted is the total number of listeners executed during processing
	TotalListenersExecuted int64

	// StartTime is the time when the processing began
	StartTime time.Time

	// EndTime is the time when the processing ended
	EndTime time.Time
}

const (
	// EventualEventAddedName is emitted when a new event is added via AddEventualEvent().
	//
	// Event arguments:
	//	0 - an EventualEventAdded struct
	EventualEventAddedName = events.EventName("EventualEventAdded")

	// EventualEventRemovedName is emitted when a previously added eventual event is processed
	//
	// Event arguments:
	//	0 - an EventualEventRemoved struct
	EventualEventRemovedName = events.EventName("EventualEventRemoved")

	// EventualEventProcessingStartName is emitted as the first action during processing
	// Event arguments:
	//	0 - an EventualEventProcessingStart struct
	EventualEventProcessingStartName = events.EventName("EventualEventProcessingStart")

	// EventualEventProcessingBatchStartName is emitted as the first set of events are processed
	// after EventualEventProcessingStartName. It is possible for 0+ batches to be processed. Each
	// patch should contain 1+ events.
	//
	// Event arguments:
	//	0 - an EventualEventProcessingBatchStart struct
	EventualEventProcessingBatchStartName = events.EventName("EventualEventProcessingBatchStart")

	// EventualEventProcessingListenerStartName is emitted for each function listener invoked
	// on each event.
	//
	// Event arguments:
	//	0 - an EventualEventProcessingListenerStart struct
	EventualEventProcessingListenerStartName = events.EventName("EventualEventProcessingListenerStart")

	// EventualEventProcessingListenerDoneName is emitted for each function listener after invocation
	//
	// Event arguments:
	//	0 - an EventualEventProcessingListenerDone struct
	EventualEventProcessingListenerDoneName = events.EventName("EventualEventProcessingListenerDone")

	// EventualEventProcessingBatchDoneName is emitted after the last event processed in a batch.
	//
	// Event arguments:
	//	0 - an EventualEventProcessingBatchDone struct
	EventualEventProcessingBatchDoneName = events.EventName("EventualEventProcessingBatchDone")

	// EventualEventProcessingDoneName is emitted as the last action during processing after
	// all events and batches.
	//
	// Event arguments:
	//	0 - an EventualEventProcessingDone struct
	EventualEventProcessingDoneName = events.EventName("EventualEventProcessingDone")
)

// EventualEventerBbolt implements EventualEventer with a bbolt back storage mechanism.
// Work is performed on a configurable basis via the Interval property in FIFO order.
//
// Events are stored in the following format:
//		id   - CUID   - a monotonic reference id
//      name - string - an event name, used for log output
//      data - []byte - a string array of arguments
type EventualEventerBbolt struct {
	events.EventEmmiter
	handlerMap        cmap.ConcurrentMap[[]EventListenerFunc] //eventName -> handlers
	Interval          time.Duration
	closeNotify       <-chan struct{}
	stopNotify        chan struct{}
	trigger           chan struct{}
	outstandingEvents *int64

	waiters sync.Map //id -> chan struct{}

	running    concurrenz.AtomicBoolean
	batchSize  int
	dbProvider DbProvider
	store      EventualEventStore
}

var _ EventualEventer = &EventualEventerBbolt{}

// NewEventualEventerBbolt creates a new bbolt backed asynchronous eventer that will check for new events at the given interval
// or when triggered. On each interval/trigger, the number of events processed is determined by batchSize.
func NewEventualEventerBbolt(dbProvider DbProvider, store EventualEventStore, interval time.Duration, batchSize int) *EventualEventerBbolt {
	outstanding := int64(0)
	return &EventualEventerBbolt{
		EventEmmiter:      events.New(),
		Interval:          interval,
		dbProvider:        dbProvider,
		store:             store,
		batchSize:         batchSize,
		trigger:           make(chan struct{}, 1),
		handlerMap:        cmap.New[[]EventListenerFunc](),
		outstandingEvents: &outstanding,
	}
}

func (a *EventualEventerBbolt) AddEventualEventWithCtx(ctx boltz.MutateContext, eventType string, data []byte) {
	newId := cuid.New()
	total := atomic.AddInt64(a.outstandingEvents, 1)
	a.Emit(EventualEventAddedName, &EventualEventAdded{
		Id:    newId,
		Total: total,
	})

	event := &EventualEvent{
		BaseExtEntity: boltz.BaseExtEntity{
			Id:        newId,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Type: eventType,
		Data: data,
	}

	var err error
	if ctx == nil {
		err = a.dbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
			ctx := boltz.NewMutateContext(tx)
			return a.store.Create(ctx, event)
		})
	} else {
		err = a.store.Create(ctx, event)
	}

	if err != nil {
		total := atomic.AddInt64(a.outstandingEvents, -1)
		a.Emit(EventualEventRemovedName, &EventualEventRemoved{
			Id:    newId,
			Total: total,
		})
		pfxlog.Logger().WithError(err).Error("error adding event for EventualEventerBbolt")
		return
	}
}

func (a *EventualEventerBbolt) AddEventualEvent(eventType string, data []byte) {
	a.AddEventualEventWithCtx(nil, eventType, data)
}

func (a *EventualEventerBbolt) AddEventualListener(eventType string, listener EventListenerFunc) {
	a.handlerMap.Upsert(eventType, nil, func(exist bool, handlers []EventListenerFunc, _ []EventListenerFunc) []EventListenerFunc {
		handlers = append(handlers, listener)
		return handlers
	})
}

func (a *EventualEventerBbolt) Start(closeNotify <-chan struct{}) error {
	if !a.running.CompareAndSwap(false, true) {
		return errors.New("already started")
	}
	a.stopNotify = make(chan struct{}, 0)
	a.closeNotify = closeNotify
	go a.run()

	return nil
}

// run should be executed once on start and is the main
// processing loop. Exiting is based on close/stop notify
// channels provided during creation. Processing is triggered
// when Trigger() is called or on the configured interval.
func (a *EventualEventerBbolt) run() {
	for {
		select {
		case <-a.closeNotify:
			a.running.Set(false)
			return
		case <-a.stopNotify:
			a.running.Set(false)
			return
		case <-a.trigger:
			a.process()
		case <-time.After(a.Interval):
			a.trigger <- struct{}{}
		}
	}
}

func (a *EventualEventerBbolt) Stop() error {
	if a.running.CompareAndSwap(true, false) {
		close(a.stopNotify)
		return nil
	}

	return nil
}

func (a *EventualEventerBbolt) Trigger() (<-chan struct{}, error) {
	if a.running.Get() {
		doneNotify := make(chan struct{})
		a.waiters.Store(cuid.New(), doneNotify)

		select {
		case a.trigger <- struct{}{}:
		default:
			//channel full, already queued for processing
		}

		return doneNotify, nil
	}

	return nil, errors.New("triggering is impossible, eventual eventer not started")
}

// deleteEventualEvent removes an eventual event by id from the bbolt backend store.
func (a *EventualEventerBbolt) deleteEventualEvent(id string) error {
	err := a.dbProvider.GetDb().Update(func(tx *bbolt.Tx) error {
		ctx := boltz.NewMutateContext(tx)
		return a.store.DeleteById(ctx, id)
	})

	if err == nil {
		total := atomic.AddInt64(a.outstandingEvents, -1)
		a.Emit(EventualEventRemovedName, &EventualEventRemoved{
			Id:    id,
			Total: total,
		})
	}

	return err
}

// notifyWaiters closes all signaling channels returned in calls
// to Trigger().
func (a *EventualEventerBbolt) notifyWaiters() {
	var waiterIds []string

	a.waiters.Range(func(key, value interface{}) bool {
		id := key.(string)
		waiterChan := value.(chan struct{})
		if waiterChan != nil {
			close(waiterChan)
		}

		waiterIds = append(waiterIds, id)
		return true
	})

	for _, id := range waiterIds {
		a.waiters.Delete(id)
	}
}

// getEventualEvents returns eventual events up from the persistent bbolt store. The number
// returned is determined by batchSize and the order of the events are based on key sorting
func (a *EventualEventerBbolt) getEventualEvents() ([]string, []*EventualEvent, error) {
	var ids []string
	var eventualEvents []*EventualEvent
	err := a.dbProvider.GetDb().View(func(tx *bbolt.Tx) error {
		var err error
		ids, _, err = a.store.QueryIds(tx, fmt.Sprintf("limit %d", a.batchSize))

		if err != nil {
			return err
		}

		for _, id := range ids {
			event, err := a.store.LoadOneById(tx, id)

			if err != nil {
				pfxlog.Logger().WithField("id", id).WithError(err).Errorf("error could not load event id %s", id)
			} else {
				eventualEvents = append(eventualEvents, event)
			}
		}
		return nil
	})

	return ids, eventualEvents, err
}

type runInfo struct {
	processId              string
	totalBatches           int64
	totalEvents            int64
	totalListenersExecuted int64
	totalEventIndex        int64
	batchEventIndex        int64
	batchId                string
	numIds                 int
}

// resolveEventualEvents will wait if there are newly added eventual events
// that have a transaction that has not cleared the event store yet. This covers
// situations where tests are expecting the next run to resolve their event. resolveEvents
// intentionally adds 2500 microseconds (2.5ms) when new events have been added, but
// none are returned when queried
func (a *EventualEventerBbolt) resolveEventualEvents() (int, []string, []*EventualEvent) {
	var numIds int
	var ids []string
	var eventualEvents []*EventualEvent

	// if we know we have outstanding events query for them up to 5 times then give up
	numEvents := atomic.LoadInt64(a.outstandingEvents)

	for attempt := 0; attempt < 5 && numEvents > 0; attempt++ {
		var err error
		ids, eventualEvents, err = a.getEventualEvents()
		numIds = len(ids)

		if err != nil {
			pfxlog.Logger().WithError(err).Error("error during EventualEventerBbolt processing, could not query events")
		}

		if numIds > 0 {
			break
		}

		time.Sleep(500 * time.Microsecond)
		numEvents = atomic.LoadInt64(a.outstandingEvents)
	}

	return numIds, ids, eventualEvents
}

// process is the main execution look for dealing with batches of eventual events, triggering listeners for
// the eventual events and emitting normal processing events
func (a *EventualEventerBbolt) process() {

	info := &runInfo{
		processId: cuid.New(),
	}

	startTime := time.Now()

	a.Emit(EventualEventProcessingStartName, &EventualEventProcessingStart{
		Id:        info.processId,
		StartTime: startTime,
	})

	defer func() {
		a.notifyWaiters()

		a.Emit(EventualEventProcessingDoneName, &EventualEventProcessingDone{
			Id:                     info.processId,
			TotalBatches:           info.totalBatches,
			TotalEvents:            info.totalEvents,
			TotalListenersExecuted: info.totalListenersExecuted,
			StartTime:              startTime,
			EndTime:                time.Now(),
		})
	}()

	for {
		info.batchId = cuid.New()
		info.numIds = 0

		var ids []string
		var eventualEvents []*EventualEvent

		info.numIds, ids, eventualEvents = a.resolveEventualEvents()

		if info.numIds == 0 {
			return
		}

		if info.numIds != len(eventualEvents) {
			pfxlog.Logger().
				WithFields(map[string]interface{}{
					"queriedCount": info.numIds,
					"eventCount":   len(eventualEvents),
				}).
				Warnf("%d event records were queried and only %d events were loaded", info.numIds, len(eventualEvents))
		}

		a.processBatch(info, eventualEvents)
		a.deleteEvents(ids)
	}
}

func (a *EventualEventerBbolt) processBatch(info *runInfo, eventualEvents []*EventualEvent) {
	info.totalBatches++
	startTime := time.Now()
	a.Emit(EventualEventProcessingBatchStartName, &EventualEventProcessingBatchStart{
		Id:        info.batchId,
		ProcessId: info.processId,
		Count:     info.numIds,
		BatchSize: a.batchSize,
		StartTime: startTime,
	})

	info.batchEventIndex = 0
	for _, eventualEvent := range eventualEvents {
		info.batchEventIndex++
		info.totalEventIndex++

		if handlers, found := a.handlerMap.Get(eventualEvent.Type); found {
			for _, handler := range handlers {
				a.executeHandler(info, eventualEvent, handler)
			}
		} else {
			pfxlog.Logger().
				WithFields(map[string]interface{}{
					"id":   eventualEvent.Id,
					"type": eventualEvent.Type,
				}).
				Debugf("event id %s with type %s has no handlers", eventualEvent.Id, eventualEvent.Type)
		}
	}

	a.Emit(EventualEventProcessingBatchDoneName, &EventualEventProcessingBatchDone{
		Id:        info.batchId,
		ProcessId: info.processId,
		Count:     info.numIds,
		BatchSize: a.batchSize,
		StartTime: startTime,
		EndTime:   time.Now(),
	})
}

func (a *EventualEventerBbolt) deleteEvents(ids []string) {
	for _, id := range ids {
		if err := a.deleteEventualEvent(id); err != nil {
			pfxlog.Logger().WithError(err).
				WithField("id", id).
				Errorf("could not delete event id %s", id)
		}
	}
}

func (a *EventualEventerBbolt) executeHandler(info *runInfo, eventualEvent *EventualEvent, handler EventListenerFunc) {
	listenerExecId := cuid.New()
	info.totalListenersExecuted++

	var startTime time.Time
	var endTime time.Time

	defer func() {
		endTime = time.Now()

		var err error
		if r := recover(); r != nil {

			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				err = x
			default:
				err = errors.New("unknown panic")
			}

			pfxlog.Logger().WithError(err).Errorf("panic caught during asynchronous event %s handler %v", eventualEvent.Type, handler)
		}

		a.Emit(EventualEventProcessingListenerDoneName, &EventualEventProcessingListenerDone{
			Id:              listenerExecId,
			BatchId:         info.batchId,
			ProcessId:       info.processId,
			ListenerFunc:    handler,
			BatchEventIndex: info.batchEventIndex,
			TotalEventIndex: info.totalEventIndex,
			EventType:       eventualEvent.Type,
			Error:           err,
			StartTime:       startTime,
			EndTime:         endTime,
		})
	}()

	startTime = time.Now()

	a.Emit(EventualEventProcessingListenerStartName, &EventualEventProcessingListenerStart{
		Id:              listenerExecId,
		BatchId:         info.batchId,
		ProcessId:       info.processId,
		ListenerFunc:    handler,
		BatchEventIndex: info.batchEventIndex,
		TotalEventIndex: info.totalEventIndex,
		EventType:       eventualEvent.Type,
		StartTime:       startTime,
	})

	handler(eventualEvent.Type, eventualEvent.Data)
}
