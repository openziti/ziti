package common

import (
	"fmt"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"sync"
)

type OnStoreSuccess func(index uint64, event *edge_ctrl_pb.DataState_Event)

type EventCache interface {
	// Store allows storage of an event and execution of an onSuccess callback while the event cache remains locked.
	// onSuccess may be nil. This function is blocking.
	Store(event *edge_ctrl_pb.DataState_Event, onSuccess OnStoreSuccess) error

	// CurrentIndex returns the latest event index applied. This function is blocking.
	CurrentIndex() (uint64, bool)

	// ReplayFrom returns an array of events from startIndex and true if the replay may be facilitated.
	// An empty slice and true is returned in cases where the requested startIndex is the current index.
	// An empty slice and false is returned in cases where the replay cannot be facilitated.
	// This function is blocking.
	ReplayFrom(startIndex uint64) ([]*edge_ctrl_pb.DataState_Event, bool)

	// WhileLocked allows the execution of arbitrary functionality while the event cache is locked. This function
	// is blocking.
	WhileLocked(func(uint64, bool))

	// SetCurrentIndex sets the current index to the supplied value. All event log history may be lost.
	SetCurrentIndex(uint64)
}

// ForgetfulEventCache does not store events or support replaying. It tracks
// the event index and that is it. It is a stand in for LoggingEventCache
// when replaying events is not expected (i.e. in routers)
type ForgetfulEventCache struct {
	lock  sync.Mutex
	index *uint64
}

func NewForgetfulEventCache() *ForgetfulEventCache {
	return &ForgetfulEventCache{}
}

func (cache *ForgetfulEventCache) SetCurrentIndex(index uint64) {
	cache.index = &index
}

func (cache *ForgetfulEventCache) WhileLocked(callback func(uint64, bool)) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	callback(cache.currentIndex())
}

func (cache *ForgetfulEventCache) Store(event *edge_ctrl_pb.DataState_Event, onSuccess OnStoreSuccess) error {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	// Synthetic events are not backed by any kind of data store that provides and index. They are not stored and
	// trigger the on success callback immediately.
	if event.IsSynthetic {
		onSuccess(event.Index, event)
		return nil
	}

	if cache.index != nil {
		if *cache.index >= event.Index {
			//already past this index
			return nil
		}

		if *cache.index >= event.Index {
			return fmt.Errorf("out of order event detected, currentIndex: %d, recievedIndex: %d, type :%T", *cache.index, event.Index, cache)
		}
	}

	cache.index = &event.Index

	if onSuccess != nil {
		onSuccess(*cache.index, event)
	}

	return nil
}

func (cache *ForgetfulEventCache) ReplayFrom(_ uint64) ([]*edge_ctrl_pb.DataState_Event, bool) {
	return nil, false
}

func (cache *ForgetfulEventCache) CurrentIndex() (uint64, bool) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	return cache.currentIndex()
}

func (cache *ForgetfulEventCache) currentIndex() (uint64, bool) {
	if cache.index == nil {
		return 0, false
	}

	return *cache.index, true
}

// LoggingEventCache stores events in order to support replaying (i.e. in controllers).
type LoggingEventCache struct {
	lock         sync.Mutex
	HeadLogIndex uint64
	LogSize      uint64
	Log          []uint64
	Events       map[uint64]*edge_ctrl_pb.DataState_Event
}

func NewLoggingEventCache(logSize uint64) *LoggingEventCache {
	return &LoggingEventCache{
		HeadLogIndex: 0,
		LogSize:      logSize,
		Log:          make([]uint64, logSize),
		Events:       map[uint64]*edge_ctrl_pb.DataState_Event{},
	}
}

func (cache *LoggingEventCache) SetCurrentIndex(index uint64) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	cache.HeadLogIndex = 0
	cache.Log = make([]uint64, cache.LogSize)
	cache.Log[0] = index
	cache.Events = map[uint64]*edge_ctrl_pb.DataState_Event{}
}

func (cache *LoggingEventCache) WhileLocked(callback func(uint64, bool)) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	callback(cache.currentIndex())
}

func (cache *LoggingEventCache) Store(event *edge_ctrl_pb.DataState_Event, onSuccess OnStoreSuccess) error {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	// Synthetic events are not backed by any kind of data store that provides and index. They are not stored and
	// trigger the on success callback immediately.
	if event.IsSynthetic {
		onSuccess(event.Index, event)
		return nil
	}

	currentIndex, ok := cache.currentIndex()

	if ok && currentIndex >= event.Index {
		return fmt.Errorf("out of order event detected, currentIndex: %d, recievedIndex: %d, type :%T", currentIndex, event.Index, cache)
	}

	targetLogIndex := uint64(0)
	targetLogIndex = (cache.HeadLogIndex + 1) % cache.LogSize

	// delete old value if we have looped
	prevKey := cache.Log[targetLogIndex]

	if prevKey != 0 {
		delete(cache.Events, prevKey)
	}

	// add new values
	cache.Log[targetLogIndex] = event.Index
	cache.Events[event.Index] = event

	//update head
	cache.HeadLogIndex = targetLogIndex

	onSuccess(event.Index, event)
	return nil
}

func (cache *LoggingEventCache) CurrentIndex() (uint64, bool) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	return cache.currentIndex()
}

func (cache *LoggingEventCache) currentIndex() (uint64, bool) {
	if len(cache.Log) == 0 {
		return 0, false
	}

	return cache.Log[cache.HeadLogIndex], true
}

func (cache *LoggingEventCache) ReplayFrom(startIndex uint64) ([]*edge_ctrl_pb.DataState_Event, bool) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	_, eventFound := cache.Events[startIndex]

	if !eventFound {
		return nil, false
	}

	var startLogIndex *uint64

	for logIndex, eventIndex := range cache.Log {
		if eventIndex == startIndex {
			tmp := uint64(logIndex)
			startLogIndex = &tmp
			break
		}
	}

	if startLogIndex == nil {
		return nil, false
	}

	// no replay
	if *startLogIndex == cache.HeadLogIndex {
		return nil, true
	}

	// ez replay
	if *startLogIndex < cache.HeadLogIndex {
		var result []*edge_ctrl_pb.DataState_Event
		for _, key := range cache.Log[*startLogIndex:cache.HeadLogIndex] {
			result = append(result, cache.Events[key])
		}
		return result, true
	}

	//looping replay
	var result []*edge_ctrl_pb.DataState_Event
	for _, key := range cache.Log[*startLogIndex:] {
		result = append(result, cache.Events[key])
	}

	for _, key := range cache.Log[0:cache.HeadLogIndex] {
		result = append(result, cache.Events[key])
	}

	return result, true
}
