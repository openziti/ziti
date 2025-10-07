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

package common

import (
	"fmt"
	"sync"

	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
)

type ReplayResult int

func (r ReplayResult) String() string {
	switch r {
	case ReplayResultSuccess:
		return "Success"
	case ReplayResultFullSyncRequired:
		return "FullSyncRequired"
	case ReplayResultRequestFromFuture:
		return "RequestFromFuture"
	default:
		return "Unknown"
	}
}

const (
	ReplayResultSuccess           = ReplayResult(1)
	ReplayResultFullSyncRequired  = ReplayResult(2)
	ReplayResultRequestFromFuture = ReplayResult(3)
)

type OnStoreSuccess func(index uint64, event *edge_ctrl_pb.DataState_ChangeSet)

type EventCache interface {
	// Store allows storage of an event and execution of an onSuccess callback while the event cache remains locked.
	// onSuccess may be nil. This function is blocking.
	Store(event *edge_ctrl_pb.DataState_ChangeSet, onSuccess OnStoreSuccess) error

	// CurrentIndex returns the latest event index applied. This function is blocking.
	CurrentIndex() uint64

	// ReplayFrom returns an array of events from startIndex and true if the replay may be facilitated.
	// An empty slice and true is returned in cases where the requested startIndex is greater than the current index.
	// An empty slice and false is returned in cases where the replay cannot be facilitated.
	// This function is blocking.
	ReplayFrom(startIndex uint64) ([]*edge_ctrl_pb.DataState_ChangeSet, ReplayResult)

	// WhileLocked allows the execution of arbitrary functionality while the event cache is locked. This function
	// is blocking.
	WhileLocked(func(uint64))

	// SetCurrentIndex sets the current index to the supplied value. All event log history may be lost.
	SetCurrentIndex(uint64)
}

// LoggingEventCache stores events in order to support replaying (i.e. in controllers).
type LoggingEventCache struct {
	lock          sync.Mutex
	HeadLogIndex  int                                          `json:"-"`
	MinEntryIndex uint64                                       `json:"-"`
	LogSize       int                                          `json:"-"`
	Log           []uint64                                     `json:"-"`
	Events        map[uint64]*edge_ctrl_pb.DataState_ChangeSet `json:"-"`
}

func NewLoggingEventCache(logSize uint64) *LoggingEventCache {
	return &LoggingEventCache{
		HeadLogIndex: 0,
		LogSize:      int(logSize),
		Log:          make([]uint64, logSize),
		Events:       map[uint64]*edge_ctrl_pb.DataState_ChangeSet{},
	}
}

func (cache *LoggingEventCache) SetCurrentIndex(index uint64) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	cache.HeadLogIndex = 0
	cache.Log = make([]uint64, cache.LogSize)
	cache.Log[0] = index
	cache.Events = map[uint64]*edge_ctrl_pb.DataState_ChangeSet{}
}

func (cache *LoggingEventCache) WhileLocked(callback func(uint64)) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	callback(cache.currentIndex())
}

func (cache *LoggingEventCache) Store(event *edge_ctrl_pb.DataState_ChangeSet, onSuccess OnStoreSuccess) error {
	cache.lock.Lock()
	defer cache.lock.Unlock()
	return cache.storeLocked(event, onSuccess)
}

func (cache *LoggingEventCache) storeLocked(event *edge_ctrl_pb.DataState_ChangeSet, onSuccess OnStoreSuccess) error {
	// Synthetic events are not backed by any kind of data store that provides and index. They are not stored and
	// trigger the on success callback immediately.
	if event.IsSynthetic {
		onSuccess(event.Index, event)
		return nil
	}

	currentIndex := cache.currentIndex()

	if currentIndex >= event.Index {
		return fmt.Errorf("out of order event detected, currentIndex: %d, receivedIndex: %d, type :%T", currentIndex, event.Index, cache)
	}

	targetLogIndex := (cache.HeadLogIndex + 1) % cache.LogSize

	// delete old value if we have looped
	prevCachedIndex := cache.Log[targetLogIndex]

	if prevCachedIndex != 0 {
		delete(cache.Events, prevCachedIndex)
		cache.MinEntryIndex = prevCachedIndex + 1
	}

	// add new values
	cache.Log[targetLogIndex] = event.Index
	cache.Events[event.Index] = event

	// update head
	cache.HeadLogIndex = targetLogIndex
	if cache.MinEntryIndex == 0 {
		cache.MinEntryIndex = event.Index
	}

	onSuccess(event.Index, event)
	return nil
}

func (cache *LoggingEventCache) CurrentIndex() uint64 {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	return cache.currentIndex()
}

func (cache *LoggingEventCache) currentIndex() uint64 {
	return cache.Log[cache.HeadLogIndex]
}

func (cache *LoggingEventCache) ReplayFrom(startIndex uint64) ([]*edge_ctrl_pb.DataState_ChangeSet, ReplayResult) {
	cache.lock.Lock()
	defer cache.lock.Unlock()

	_, eventFound := cache.Events[startIndex]
	headIndex := cache.Log[cache.HeadLogIndex]

	for !eventFound {
		// if we're asked to replay an index we haven't reached yet, return an empty list
		if startIndex > headIndex {
			return nil, ReplayResultRequestFromFuture
		}

		if startIndex < cache.MinEntryIndex {
			return nil, ReplayResultFullSyncRequired
		}

		startIndex++
		if startIndex > headIndex {
			return nil, ReplayResultFullSyncRequired
		}
		_, eventFound = cache.Events[startIndex]
	}

	var startLogIndex *int

	tryOffset := int(headIndex - startIndex)
	var tryStartIndex int
	if cache.HeadLogIndex-tryOffset >= 0 {
		tryStartIndex = cache.HeadLogIndex - tryOffset
	}

	for i := tryStartIndex; i < len(cache.Log); i++ {
		if cache.Log[i] == startIndex {
			startLogIndex = &i
			break
		}
	}

	if startLogIndex == nil && tryStartIndex > 0 {
		for i := 0; i < tryStartIndex; i++ {
			if cache.Log[i] == startIndex {
				startLogIndex = &i
				break
			}
		}
	}

	if startLogIndex == nil {
		return nil, ReplayResultFullSyncRequired
	}

	// replay, no loop required
	if *startLogIndex <= cache.HeadLogIndex {
		var result []*edge_ctrl_pb.DataState_ChangeSet
		for _, key := range cache.Log[*startLogIndex : cache.HeadLogIndex+1] {
			result = append(result, cache.Events[key])
		}
		return result, ReplayResultSuccess
	}

	// looping replay
	var result []*edge_ctrl_pb.DataState_ChangeSet
	for _, key := range cache.Log[*startLogIndex:] {
		result = append(result, cache.Events[key])
	}

	for _, key := range cache.Log[:cache.HeadLogIndex+1] {
		result = append(result, cache.Events[key])
	}

	return result, ReplayResultSuccess
}
