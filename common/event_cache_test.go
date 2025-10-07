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
	"testing"

	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

var noOpCacheCallback = func(index uint64, event *edge_ctrl_pb.DataState_ChangeSet) {}

func TestLoggingEventCache_Store(t *testing.T) {
	t.Run("stores events sequentially", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		for i := uint64(1); i <= 5; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			var callbackIndex uint64
			err := cache.Store(event, func(index uint64, event *edge_ctrl_pb.DataState_ChangeSet) {
				callbackIndex = index
			})

			require.NoError(t, err)
			require.Equal(t, i, callbackIndex)
		}

		currentIndex := cache.CurrentIndex()
		require.Equal(t, uint64(5), currentIndex)
	})

	t.Run("rejects out of order events", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		event1 := &edge_ctrl_pb.DataState_ChangeSet{Index: 5}
		err := cache.Store(event1, noOpCacheCallback)
		require.NoError(t, err)

		// Try to store an event with a lower index
		event2 := &edge_ctrl_pb.DataState_ChangeSet{Index: 3}
		err = cache.Store(event2, noOpCacheCallback)
		require.Error(t, err)
		require.Contains(t, err.Error(), "out of order event detected")
	})

	t.Run("rejects duplicate events", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		event1 := &edge_ctrl_pb.DataState_ChangeSet{Index: 5}
		err := cache.Store(event1, noOpCacheCallback)
		require.NoError(t, err)

		// Try to store an event with the same index
		event2 := &edge_ctrl_pb.DataState_ChangeSet{Index: 5}
		err = cache.Store(event2, noOpCacheCallback)
		require.Error(t, err)
		require.Contains(t, err.Error(), "out of order event detected")
	})

	t.Run("handles synthetic events without storing", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store a normal event first
		event1 := &edge_ctrl_pb.DataState_ChangeSet{Index: 1}
		err := cache.Store(event1, noOpCacheCallback)
		require.NoError(t, err)

		// Store a synthetic event
		syntheticEvent := &edge_ctrl_pb.DataState_ChangeSet{Index: 100, IsSynthetic: true}
		var callbackCalled bool
		err = cache.Store(syntheticEvent, func(index uint64, event *edge_ctrl_pb.DataState_ChangeSet) {
			callbackCalled = true
			require.Equal(t, uint64(100), index)
		})
		require.NoError(t, err)
		require.True(t, callbackCalled)

		// Current index should still be 1
		currentIndex := cache.CurrentIndex()
		require.Equal(t, uint64(1), currentIndex)
	})

	t.Run("wraps around when cache is full", func(t *testing.T) {
		cacheSize := uint64(5)
		cache := NewLoggingEventCache(cacheSize)

		// Fill the cache
		for i := uint64(1); i <= 10; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Verify current index
		currentIndex := cache.CurrentIndex()
		require.Equal(t, uint64(10), currentIndex)

		// Verify that old events have been evicted
		require.Equal(t, uint64(6), cache.MinEntryIndex)
		require.Equal(t, int(cacheSize), len(cache.Events))

		// Verify that only events 6-10 are in the cache
		for i := uint64(6); i <= 10; i++ {
			_, exists := cache.Events[i]
			require.True(t, exists, "event %d should exist", i)
		}

		// Verify that events 1-5 have been evicted
		for i := uint64(1); i <= 5; i++ {
			_, exists := cache.Events[i]
			require.False(t, exists, "event %d should have been evicted", i)
		}
	})

	t.Run("handles gaps in event indexes", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store events with gaps
		event1 := &edge_ctrl_pb.DataState_ChangeSet{Index: 1}
		err := cache.Store(event1, noOpCacheCallback)
		require.NoError(t, err)

		event3 := &edge_ctrl_pb.DataState_ChangeSet{Index: 3}
		err = cache.Store(event3, noOpCacheCallback)
		require.NoError(t, err)

		event7 := &edge_ctrl_pb.DataState_ChangeSet{Index: 7}
		err = cache.Store(event7, noOpCacheCallback)
		require.NoError(t, err)

		// Verify current index
		currentIndex := cache.CurrentIndex()
		require.Equal(t, uint64(7), currentIndex)

		// Verify that only the stored events exist
		require.Equal(t, 3, len(cache.Events))
		_, exists := cache.Events[1]
		require.True(t, exists)
		_, exists = cache.Events[3]
		require.True(t, exists)
		_, exists = cache.Events[7]
		require.True(t, exists)
	})
}

func TestLoggingEventCache_ReplayFrom(t *testing.T) {
	t.Run("replays from the beginning", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store 5 events
		for i := uint64(1); i <= 5; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Replay from index 1
		events, result := cache.ReplayFrom(1)
		require.Equal(t, ReplayResultSuccess, result)
		require.Len(t, events, 5)

		for i, event := range events {
			require.Equal(t, uint64(i+1), event.Index)
		}
	})

	t.Run("replays from the end", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store 5 events
		for i := uint64(1); i <= 5; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Replay from the last event
		events, result := cache.ReplayFrom(5)
		require.Equal(t, ReplayResultSuccess, result)
		require.Len(t, events, 1)
		require.Equal(t, uint64(5), events[0].Index)
	})

	t.Run("replays from the middle", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store 5 events
		for i := uint64(1); i <= 5; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Replay from index 3
		events, result := cache.ReplayFrom(3)
		require.Equal(t, ReplayResultSuccess, result)
		require.Len(t, events, 3)

		for i, event := range events {
			require.Equal(t, uint64(i+3), event.Index)
		}
	})

	t.Run("returns request from future when index hasn't been stored yet", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store 5 events
		for i := uint64(1); i <= 5; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Try to replay from index 10 (not yet stored)
		events, result := cache.ReplayFrom(10)
		require.Equal(t, ReplayResultRequestFromFuture, result)
		require.Nil(t, events)
	})

	t.Run("requires full sync when index has been evicted", func(t *testing.T) {
		cacheSize := uint64(5)
		cache := NewLoggingEventCache(cacheSize)

		// Store 10 events (will evict first 5)
		for i := uint64(1); i <= 10; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Try to replay from index 1 (evicted)
		events, result := cache.ReplayFrom(1)
		require.Equal(t, ReplayResultFullSyncRequired, result)
		require.Nil(t, events)

		// Try to replay from index 5 (evicted)
		events, result = cache.ReplayFrom(5)
		require.Equal(t, ReplayResultFullSyncRequired, result)
		require.Nil(t, events)
	})

	t.Run("replays events after cache wrap-around", func(t *testing.T) {
		cacheSize := uint64(5)
		cache := NewLoggingEventCache(cacheSize)

		// Store 10 events (will cause wrap-around)
		for i := uint64(1); i <= 10; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Replay from index 6 (first available after eviction)
		events, result := cache.ReplayFrom(6)
		require.Equal(t, ReplayResultSuccess, result)
		require.Len(t, events, 5)

		for i, event := range events {
			require.Equal(t, uint64(i+6), event.Index)
		}
	})

	t.Run("handles gaps in stored indexes", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store events with gaps: 1, 3, 5, 7, 9
		for i := uint64(1); i <= 9; i += 2 {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Try to replay from missing index 2 (gap)
		events, result := cache.ReplayFrom(2)
		require.Equal(t, ReplayResultSuccess, result)
		// Should return events from index 3 onward (skipping the gap)
		require.Len(t, events, 4)
		require.Equal(t, uint64(3), events[0].Index)
		require.Equal(t, uint64(5), events[1].Index)
		require.Equal(t, uint64(7), events[2].Index)
		require.Equal(t, uint64(9), events[3].Index)
	})

	t.Run("handles gap at the end", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store events: 1, 2, 5
		for _, idx := range []uint64{1, 2, 5} {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: idx}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Try to replay from index 3 or 4 (in the gap)
		events, result := cache.ReplayFrom(3)
		require.Equal(t, ReplayResultSuccess, result)
		require.Len(t, events, 1)
		require.Equal(t, uint64(5), events[0].Index)
	})

	t.Run("replays with wrap-around in circular buffer", func(t *testing.T) {
		cacheSize := uint64(5)
		cache := NewLoggingEventCache(cacheSize)

		// Store 8 events to cause wrap-around
		for i := uint64(1); i <= 8; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Cache should contain events 4-8
		// Replay from index 5 (which is in the wrapped portion)
		events, result := cache.ReplayFrom(5)
		require.Equal(t, ReplayResultSuccess, result)
		require.Len(t, events, 4)

		for i, event := range events {
			require.Equal(t, uint64(i+5), event.Index)
		}
	})
}

func TestLoggingEventCache_CurrentIndex(t *testing.T) {
	t.Run("returns zero index for empty cache", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		index := cache.CurrentIndex()
		require.Equal(t, uint64(0), index)
	})

	t.Run("returns current index after storing events", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		event := &edge_ctrl_pb.DataState_ChangeSet{Index: 42}
		err := cache.Store(event, noOpCacheCallback)
		require.NoError(t, err)

		index := cache.CurrentIndex()
		require.Equal(t, uint64(42), index)
	})
}

func TestLoggingEventCache_SetCurrentIndex(t *testing.T) {
	t.Run("resets cache to new index", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		// Store some events
		for i := uint64(1); i <= 5; i++ {
			event := &edge_ctrl_pb.DataState_ChangeSet{Index: i}
			err := cache.Store(event, noOpCacheCallback)
			require.NoError(t, err)
		}

		// Reset to a new index
		cache.SetCurrentIndex(100)

		// Verify the cache was reset
		index := cache.CurrentIndex()
		require.Equal(t, uint64(100), index)

		// Old events should be cleared
		require.Equal(t, 0, len(cache.Events))
		require.Equal(t, 0, cache.HeadLogIndex)
	})
}

func TestLoggingEventCache_WhileLocked(t *testing.T) {
	t.Run("executes callback while locked", func(t *testing.T) {
		cache := NewLoggingEventCache(10)

		event := &edge_ctrl_pb.DataState_ChangeSet{Index: 42}
		err := cache.Store(event, noOpCacheCallback)
		require.NoError(t, err)

		var callbackIndex uint64
		cache.WhileLocked(func(index uint64) {
			callbackIndex = index
		})

		require.Equal(t, uint64(42), callbackIndex)
	})
}
