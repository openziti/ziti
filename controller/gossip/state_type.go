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

package gossip

import (
	"context"
	"time"
)

// StateListener receives notifications when gossip state changes.
type StateListener[T any] interface {
	EntryChanged(key string, value T, version uint64, owner string, isCreate bool, origin ChangeOrigin)
	EntryRemoved(key string, owner string, origin ChangeOrigin)
}

// StateTypeConfig configures a typed gossip state registration.
type StateTypeConfig[T any] struct {
	Name                string
	Encode              func(T) ([]byte, error)
	Decode              func([]byte) (T, error)
	Tombstones          bool
	TombstoneTTL        time.Duration
	AntiEntropy         bool
	AntiEntropyInterval time.Duration
	Listener            StateListener[T]
}

// StateTypeInfo provides non-generic access to gossip state metadata.
type StateTypeInfo interface {
	MaxVersionForOwner(owner string) uint64
	NonTombstoneCount(owner string) int64
	HashForOwner(owner string) uint64
}

// StateType is a typed handle to a gossip stateMap.
type StateType[T any] struct {
	sm     *stateMap
	encode func(T) ([]byte, error)
	decode func([]byte) (T, error)
}

// Register creates a new typed gossip state in the store and starts any
// background goroutines (anti-entropy, tombstone reaping).
func Register[T any](store *Store, config StateTypeConfig[T]) *StateType[T] {
	var listener untypedListener
	if config.Listener != nil {
		listener = &typedListenerAdapter[T]{
			decode:   config.Decode,
			listener: config.Listener,
		}
	}

	cfg := stateMapConfig{
		tombstones:          config.Tombstones,
		tombstoneTTL:        config.TombstoneTTL,
		antiEntropy:         config.AntiEntropy,
		antiEntropyInterval: config.AntiEntropyInterval,
	}
	sm := newStateMap(config.Name, cfg, store, listener)
	store.types.Store(config.Name, sm)

	if config.AntiEntropy && config.AntiEntropyInterval > 0 {
		go runAntiEntropy(sm, store)
	}

	if config.Tombstones && config.TombstoneTTL > 0 {
		go runTombstoneReaper(sm, store)
	}

	return &StateType[T]{
		sm:     sm,
		encode: config.Encode,
		decode: config.Decode,
	}
}

// Set stores a value, assigns a new version, notifies the listener, and broadcasts to peers.
func (st *StateType[T]) Set(key, owner string, value T) error {
	encoded, err := st.encode(value)
	if err != nil {
		return err
	}
	st.sm.set(key, owner, encoded, OriginLocal)
	return nil
}

// SetConfirmed stores a value and waits for acknowledgment from all peers.
func (st *StateType[T]) SetConfirmed(ctx context.Context, key, owner string, value T) error {
	encoded, err := st.encode(value)
	if err != nil {
		return err
	}
	return st.sm.store.setConfirmed(ctx, st.sm, key, owner, encoded)
}

// Delete removes or tombstones an entry and broadcasts the change.
func (st *StateType[T]) Delete(key, owner string) {
	st.sm.delete(key, owner, OriginLocal)
}

// DeleteByOwner removes or tombstones all entries belonging to the given owner.
func (st *StateType[T]) DeleteByOwner(owner string) {
	st.sm.deleteByOwner(owner, OriginLocal)
}

// DeleteByOwnerBefore removes or tombstones entries belonging to the given
// owner whose epoch compares less than the provided epoch. Used to clean up
// entries from a previous router lifetime when a new epoch is detected.
func (st *StateType[T]) DeleteByOwnerBefore(owner string, epoch []byte) {
	st.sm.deleteByOwnerBefore(owner, epoch, OriginLocal)
}

// Reconcile removes entries for the given owner that are not in the provided key set.
func (st *StateType[T]) Reconcile(owner string, currentKeys map[string]struct{}) {
	st.sm.reconcile(owner, currentKeys, OriginLocal)
}

// Get retrieves a decoded value by key.
func (st *StateType[T]) Get(key string) (T, uint64, bool) {
	var zero T
	e, ok := st.sm.entries.Get(key)
	if !ok || e.Tombstone {
		return zero, 0, false
	}
	val, err := st.decode(e.Value)
	if err != nil {
		return zero, 0, false
	}
	return val, e.Version, true
}

// Iter calls fn for each non-tombstoned entry.
func (st *StateType[T]) Iter(fn func(key string, value T, owner string)) {
	st.sm.entries.IterCb(func(key string, e *entry) {
		if !e.Tombstone {
			val, err := st.decode(e.Value)
			if err == nil {
				fn(key, val, e.Owner)
			}
		}
	})
}

// IterWithVersion calls fn for each non-tombstoned entry, including the version.
func (st *StateType[T]) IterWithVersion(fn func(key string, value T, owner string, version uint64)) {
	st.sm.entries.IterCb(func(key string, e *entry) {
		if !e.Tombstone {
			val, err := st.decode(e.Value)
			if err == nil {
				fn(key, val, e.Owner, e.Version)
			}
		}
	})
}

// IterFull calls fn for each non-tombstoned entry with all metadata.
func (st *StateType[T]) IterFull(fn func(key string, value T, owner string, version uint64, epoch []byte)) {
	st.sm.entries.IterCb(func(key string, e *entry) {
		if !e.Tombstone {
			val, err := st.decode(e.Value)
			if err == nil {
				fn(key, val, e.Owner, e.Version, e.Epoch)
			}
		}
	})
}

// IterByOwner calls fn for each non-tombstoned entry belonging to the given owner.
func (st *StateType[T]) IterByOwner(owner string, fn func(key string, value T)) {
	st.sm.entries.IterCb(func(key string, e *entry) {
		if e.Owner == owner && !e.Tombstone {
			val, err := st.decode(e.Value)
			if err == nil {
				fn(key, val)
			}
		}
	})
}

// MaxVersionForOwner returns the highest version among all entries (including
// tombstones) belonging to the given owner. Returns 0 if no entries exist.
func (st *StateType[T]) MaxVersionForOwner(owner string) uint64 {
	return st.sm.maxVersionForOwner(owner)
}

// NonTombstoneCount returns the number of non-tombstone entries for the given owner.
func (st *StateType[T]) NonTombstoneCount(owner string) int64 {
	return st.sm.nonTombstoneCount(owner)
}

// HashForOwner returns a FNV-64a hash of the sorted non-tombstone entry keys
// belonging to the given owner. Used for staleness detection via canary hashes.
func (st *StateType[T]) HashForOwner(owner string) uint64 {
	return st.sm.hashForOwner(owner)
}

// typedListenerAdapter wraps a StateListener[T] as an untypedListener.
type typedListenerAdapter[T any] struct {
	decode   func([]byte) (T, error)
	listener StateListener[T]
}

func (a *typedListenerAdapter[T]) entryChanged(key string, value []byte, version uint64, owner string, isCreate bool, origin ChangeOrigin) {
	val, err := a.decode(value)
	if err != nil {
		return
	}
	a.listener.EntryChanged(key, val, version, owner, isCreate, origin)
}

func (a *typedListenerAdapter[T]) entryRemoved(key string, owner string, origin ChangeOrigin) {
	a.listener.EntryRemoved(key, owner, origin)
}

func runTombstoneReaper(sm *stateMap, store *Store) {
	ticker := time.NewTicker(sm.config.tombstoneTTL)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			sm.reapTombstones()
		case <-store.closeCh:
			return
		}
	}
}
