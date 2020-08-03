package persistence

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/foundation/storage/boltz"
	"github.com/openziti/foundation/util/cowslice"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"io/ioutil"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testPersistedEvent struct {
	seq int64
	id  string
}

func (event *testPersistedEvent) getType() string {
	return "test"
}

func (event *testPersistedEvent) persist(bucket *boltz.TypedBucket) {
	bucket.SetInt64("seq", event.seq, nil)
	bucket.SetString("id", event.id, nil)
}

func (event *testPersistedEvent) handle(tx *bbolt.Tx) {
	for _, listener := range getTestEventListeners() {
		listener(tx, event)
	}
}

type testEventHandler func(tx *bbolt.Tx, event *testPersistedEvent)

var testEventRegistry = cowslice.NewCowSlice(make([]testEventHandler, 0))

func getTestEventListeners() []testEventHandler {
	return testEventRegistry.Value().([]testEventHandler)
}

func registerTestEvent(handler testEventHandler) {
	cowslice.Append(testEventRegistry, handler)
}

type testEventLoader struct{}

func (loader *testEventLoader) loadEvent(bucket *boltz.TypedBucket) storeEvent {
	id := bucket.GetStringOrError("id")
	seq := bucket.GetInt64WithDefault("seq", 0)
	return &testPersistedEvent{seq: seq, id: id}
}

func TestNewPersistedEventDispatcher(t *testing.T) {
	req := require.New(t)
	dbFile, err := ioutil.TempFile("", "query-bolt-test-db")
	req.NoError(err)
	req.NoError(dbFile.Close())
	db, err := bbolt.Open(dbFile.Name(), 0, bbolt.DefaultOptions)

	req.NoError(err)

	waitGroup := &sync.WaitGroup{}
	waitGroup.Add(1000)

	current := int64(-1)
	var orderErr error
	testF := func(tx *bbolt.Tx, event *testPersistedEvent) {
		nextVal := atomic.AddInt64(&current, 1)
		if event.seq != nextVal {
			orderErr = errors.Errorf("ordering error. Current %v, event: %v", nextVal, event.seq)
		}

		r := rand.Intn(6)
		if r > 5 {
			time.Sleep(5 * time.Millisecond)
		}
		fmt.Printf("handled %v: %v\n", event.seq, event.id)
		waitGroup.Done()
	}

	persistedDispatcher, err := newStoreEventDispatcher(db, "ziti", "events")
	req.NoError(err)

	persistedDispatcher.registerEventLoader("test", &testEventLoader{})
	registerTestEvent(testF)

	i := 0
	for i < 1000 {
		err := db.Update(func(tx *bbolt.Tx) error {
			count := rand.Intn(50) + 10
			for j := 0; j < count; j++ {
				if i < 1000 {
					event := &testPersistedEvent{
						id:  uuid.New().String(),
						seq: int64(i),
					}

					if err := persistedDispatcher.dispatch(tx, event); err != nil {
						return err
					}
					i++
					fmt.Printf("submitted event %v: %v\n", event.seq, event.id)
				}
			}
			return nil
		})

		req.NoError(err)
	}

	doneC := make(chan struct{})

	go func() {
		waitGroup.Wait()
		close(doneC)
	}()

	select {
	case <-doneC:
	case <-time.After(10 * time.Second):
		req.Fail("timed out waiting for messages to process")
	}

	req.NoError(orderErr)
}
