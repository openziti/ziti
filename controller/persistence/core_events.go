package persistence

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/events"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/concurrenz"
	"go.etcd.io/bbolt"
	"sync"
	"sync/atomic"
)

const (
	FieldEventType = "__eventType__"
)

type storeEventLoader interface {
	loadEvent(bucket *boltz.TypedBucket) storeEvent
}

type storeEventDispatcher interface {
	registerEventLoader(eventType string, loader storeEventLoader)
	dispatch(tx *bbolt.Tx, event storeEvent) error
	stop()
}

type storeEvent interface {
	getType() string
	persist(bucket *boltz.TypedBucket)
	handle(tx *bbolt.Tx)
}

var invalidEventRecord = errors.New("invalid entry record")

type eventWrapper struct {
	key   []byte
	event storeEvent
}

func (wrapper *eventWrapper) String() string {
	if wrapper == nil {
		return "(nil)"
	}
	return fmt.Sprintf("wrapper[id=%v]", wrapper.getId())
}

func (wrapper *eventWrapper) getId() uint64 {
	return binary.BigEndian.Uint64(wrapper.key)
}

func newStoreEventDispatcher(db *bbolt.DB, path ...string) (storeEventDispatcher, error) {
	result := &storeEventDispatcherImpl{
		db:              db,
		eventC:          make(chan *eventWrapper, 100),
		shutdownC:       make(chan struct{}),
		nextEventId:     1,
		eventsStorePath: path,
	}
	result.eventTypes.Store(map[string]storeEventLoader{})

	err := db.Update(func(tx *bbolt.Tx) error {
		baseBucket := boltz.GetOrCreatePath(tx, result.eventsStorePath...)
		if baseBucket.HasError() {
			return baseBucket.GetError()
		}

		cursor := baseBucket.Cursor()
		first, _ := cursor.First()
		last, _ := cursor.Last()

		if first != nil {
			result.nextEventId = binary.LittleEndian.Uint64(first)
			result.counter = binary.LittleEndian.Uint64(last)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	result.start()
	return result, nil
}

type storeEventDispatcherImpl struct {
	eventTypes      atomic.Value
	lock            sync.Mutex
	running         concurrenz.AtomicBoolean
	shutdownC       chan struct{}
	db              *bbolt.DB
	eventsStorePath []string
	counter         uint64
	eventC          chan *eventWrapper
	cachedEvent     *eventWrapper
	nextEventId     uint64
}

func (dispatcher *storeEventDispatcherImpl) getEventTypes() map[string]storeEventLoader {
	return dispatcher.eventTypes.Load().(map[string]storeEventLoader)
}

func (dispatcher *storeEventDispatcherImpl) registerEventLoader(eventType string, loader storeEventLoader) {
	dispatcher.lock.Lock()
	defer dispatcher.lock.Unlock()

	currentEventTypes := dispatcher.getEventTypes()
	newEventTypes := map[string]storeEventLoader{}

	for k, v := range currentEventTypes {
		newEventTypes[k] = v
	}

	newEventTypes[eventType] = loader
	dispatcher.eventTypes.Store(newEventTypes)
}

func (dispatcher *storeEventDispatcherImpl) stop() {
	if dispatcher.running.CompareAndSwap(true, false) {
		close(dispatcher.shutdownC)
	}
}

func (dispatcher *storeEventDispatcherImpl) start() {
	if dispatcher.running.CompareAndSwap(false, true) {
		go dispatcher.eventLoop()
	}
}

func (dispatcher *storeEventDispatcherImpl) deleteEvent(key []byte) error {
	return dispatcher.db.View(func(tx *bbolt.Tx) error {
		return dispatcher.deleteEventInTx(tx, key)
	})
}

func (dispatcher *storeEventDispatcherImpl) deleteEventInTx(tx *bbolt.Tx, key []byte) error {
	baseBucket := boltz.Path(tx, dispatcher.eventsStorePath...)
	if baseBucket != nil {
		return baseBucket.DeleteBucket(key)
	}
	return nil
}

func (dispatcher *storeEventDispatcherImpl) readNextValidEvent() *eventWrapper {
	result, err := dispatcher.readNextEvent()
	for err != nil {
		if errors.Is(err, invalidEventRecord) {
			if err := dispatcher.deleteEvent(result.key); err != nil {
				// if we can't delete, something is seriously wrong, likely not recoverable
				panic(err)
			}
			result, err = dispatcher.readNextEvent()
		} else {
			// some other bbolt error, which is again, likely not recoverable
			panic(err)
		}
	}
	return result
}

func (dispatcher *storeEventDispatcherImpl) readNextEvent() (*eventWrapper, error) {
	var result *eventWrapper

	err := dispatcher.db.View(func(tx *bbolt.Tx) error {
		baseBucket := boltz.Path(tx, dispatcher.eventsStorePath...)
		if baseBucket == nil {
			return nil
		}
		key, _ := baseBucket.Cursor().First()
		if key == nil {
			return nil
		}
		result = &eventWrapper{
			key: key,
		}
		eventBucket := baseBucket.GetBucketByKey(key)
		eventType := eventBucket.GetString(FieldEventType)
		if eventType == nil {
			pfxlog.Logger().Errorf("event %v had no event type", result.getId())
			return invalidEventRecord
		}

		eventTypes := dispatcher.getEventTypes()
		loader, ok := eventTypes[*eventType]
		if !ok {
			pfxlog.Logger().Errorf("event %v has unregistered event type: %v", result.getId(), *eventType)
			return invalidEventRecord
		}

		event := loader.loadEvent(eventBucket)
		if eventBucket.HasError() {
			pfxlog.Logger().Errorf("unable to load event %v or type %v (%v)", result.getId(), eventTypes, eventBucket.GetError())
			return invalidEventRecord
		}

		result.event = event

		return nil
	})

	return result, err
}

func (dispatcher *storeEventDispatcherImpl) getNextEvent() *eventWrapper {
	var next = dispatcher.cachedEvent

	if next != nil && next.getId() <= dispatcher.nextEventId {
		dispatcher.cachedEvent = nil
		return next
	}

	if next == nil {
		select {
		case next = <-dispatcher.eventC:
		default:
			next = nil
		}
	}

	if next != nil {
		if next.getId() <= dispatcher.nextEventId {
			return next
		}

		// missed some events, cache the event, so we can check it next time
		dispatcher.cachedEvent = next

		// missing ID in channel, have to go back to datastore
		next = dispatcher.readNextValidEvent()
		if next != nil {
			return next
		}

		// nothing in datastore, which shouldn't happen. At worst, we should get the event we just read off the channel
		// return the channel event and bump up the current id
		pfxlog.Logger().Warnf("dispatcher is missing events in datastore. event %v of type %v not present",
			dispatcher.cachedEvent.getId(), dispatcher.cachedEvent.event.getType())

		dispatcher.nextEventId = dispatcher.cachedEvent.getId()
		dispatcher.cachedEvent = nil
		return dispatcher.cachedEvent
	}

	// nothing queued, see if there's anything in the datastore
	if next = dispatcher.readNextValidEvent(); next != nil {
		return next
	}

	// nothing in the datastore, wait for next event notification
	select {
	case <-dispatcher.shutdownC:
		return nil
	case next = <-dispatcher.eventC:
		return next // datastore was empty, so next thing on channel should be next record
	}
}

func (dispatcher *storeEventDispatcherImpl) eventLoop() {
	for dispatcher.running.Get() {
		eventWrapper := dispatcher.getNextEvent()
		if eventWrapper != nil {
			err := dispatcher.db.Update(func(tx *bbolt.Tx) error {
				eventWrapper.event.handle(tx)
				dispatcher.nextEventId = eventWrapper.getId() + 1
				return dispatcher.deleteEventInTx(tx, eventWrapper.key)
			})
			if err != nil {
				pfxlog.Logger().Errorf("error while handling %v event %v (%v)",
					eventWrapper.event.getType(), eventWrapper.getId(), err)
			}
		}
	}
}

func (dispatcher *storeEventDispatcherImpl) nextKey() []byte {
	nextId := atomic.AddUint64(&dispatcher.counter, 1)
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, nextId)
	return key
}

func (dispatcher *storeEventDispatcherImpl) dispatch(tx *bbolt.Tx, event storeEvent) error {
	if !dispatcher.running.Get() {
		return events.DispatcherNotRunningError
	}

	wrapper := &eventWrapper{
		event: event,
		key:   dispatcher.nextKey(),
	}

	baseBucket := boltz.GetOrCreatePath(tx, dispatcher.eventsStorePath...)
	if baseBucket.HasError() {
		return baseBucket.GetError()
	}
	eventBucket, err := baseBucket.CreateBucket(wrapper.key)
	if err != nil {
		return err
	}
	bucket := &boltz.TypedBucket{
		Bucket:          eventBucket,
		ErrorHolderImpl: baseBucket.ErrorHolderImpl,
	}
	event.persist(bucket)
	bucket.SetString(FieldEventType, event.getType(), nil)
	if bucket.HasError() {
		return err
	}

	// Shouldn't run into ordering issues, since only one goroutine can submit events at a time from an update tx
	tx.OnCommit(func() {
		select {
		case dispatcher.eventC <- wrapper:
			fmt.Printf("%v submitted\n", wrapper.getId())
		default:
			// don't block submitting, we can pick it up from the datastore if need be
			fmt.Printf("%v skipped\n", wrapper.getId())
		}
	})

	return nil
}
