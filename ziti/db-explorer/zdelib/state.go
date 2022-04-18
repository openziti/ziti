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

package zdelib

import (
	"errors"
	"github.com/openziti/storage/boltz"
	"go.etcd.io/bbolt"
	"strings"
)

type State struct {
	DB             *bbolt.DB
	Path           []string
	History        []string
	pathEntryCache map[string][]Entry
	pathCountCache map[string]int64
}

// NewState creates a State which will attempt to open path as a bbolt database. If the path or db are invalid nil and
// an error are returned. Otherwise, a newly initialized State is returned at the root of the database.
func NewState(path string) (*State, error) {
	db, err := Open(path)

	if err != nil {
		return nil, err
	}

	return &State{
		DB:             db,
		Path:           nil,
		History:        nil,
		pathEntryCache: map[string][]Entry{},
		pathCountCache: map[string]int64{},
	}, nil
}

// DbStats returns the bbolt.DB database states
func (state *State) DbStats() bbolt.Stats {
	return state.DB.Stats()
}

// BucketStats returns the bbolt.BucketStats for the bucket the state's path currently points to
func (state *State) BucketStats() bbolt.BucketStats {
	var stats bbolt.BucketStats
	_ = state.DB.View(func(tx *bbolt.Tx) error {
		stats = state.CurrentBucket(tx).Stats()
		return nil
	})

	return stats
}

// CurrentBucket will return a *bbolt.Bucket matching the current state.path.
func (state *State) CurrentBucket(tx *bbolt.Tx) *bbolt.Bucket {
	if state.AtRoot() {
		return tx.Cursor().Bucket()
	}

	targetBucket := tx.Bucket([]byte(state.Path[0]))
	for _, nextBucket := range state.Path[1:] {
		if targetBucket == nil {
			return nil
		}
		targetBucket = targetBucket.Bucket([]byte(nextBucket))
	}

	return targetBucket
}

// AtRoot returns true if the current state path is at the root level.
func (state *State) AtRoot() bool {
	return len(state.Path) == 0
}

// ListEntries returns an array of all Entries for the current bucket
func (state *State) ListEntries() []Entry {
	path := strings.Join(state.Path, ".")

	if cachedEntries, ok := state.pathEntryCache[path]; ok {
		return cachedEntries
	}

	var entries []Entry

	_ = state.DB.View(func(tx *bbolt.Tx) error {
		cursor := state.CurrentBucket(tx).Cursor()

		key, value := cursor.First()
		for key != nil {
			fieldType, valueType := boltz.GetTypeAndValue(value)

			var valueString *string

			if len(valueType) != 0 {
				valueString = boltz.FieldToString(fieldType, valueType)
			} else {
				nilStr := "nil"
				valueString = &nilStr
			}

			entries = append(entries, Entry{Name: string(key), Type: fieldType, TypeString: TypeToString(fieldType), Value: value, ValueString: valueString})

			key, value = cursor.Next()
		}

		return nil
	})

	state.pathEntryCache[path] = entries
	return entries
}

// Done is meant to be called when the state is no longer needed.
func (state *State) Done() {
	if state != nil && state.DB != nil {
		_ = state.DB.Close()
	}
}

// Enter moves the state into the desired bucket name.
func (state *State) Enter(name string) error {
	return state.DB.View(func(tx *bbolt.Tx) error {
		cursor := state.CurrentBucket(tx).Cursor()

		key, value := cursor.Seek([]byte(name))

		if key == nil || string(key) != name {
			return errors.New("invalid bucket name")
		}

		if value != nil {
			return errors.New("not a bucket")
		}

		state.Path = append(state.Path, name)

		return nil
	})
}

// Back moves the state back one level if possible
func (state *State) Back() error {
	if len(state.Path) == 0 {
		return errors.New("already at root")
	}

	state.Path = state.Path[0 : len(state.Path)-1]

	return nil
}

// CurrentBucketKeyCount returns the number of keys in the bucket the state's path currently points to.
func (state *State) CurrentBucketKeyCount() int64 {
	count := int64(0)

	_ = state.DB.View(func(tx *bbolt.Tx) error {
		count = state.CurrentBucketKeyCountInTx(tx)
		return nil
	})

	return count
}

// CurrentBucketKeyCountInTx does the same thing as CurrentBucketKeyCount but withing an existing transaction
func (state *State) CurrentBucketKeyCountInTx(tx *bbolt.Tx) int64 {
	pathKey := strings.Join(state.Path, ".")
	count, ok := state.pathCountCache[pathKey]

	if ok {
		return count
	}

	cursor := state.CurrentBucket(tx).Cursor()

	for k, _ := cursor.First(); k != nil; k, _ = cursor.Next() {
		count++
	}

	state.pathCountCache[pathKey] = count

	return count
}

// GetValue returns the string value for a specific key in the current state's path location
func (state *State) GetValue(key string) string {
	var valueString *string
	_ = state.DB.View(func(tx *bbolt.Tx) error {
		value := state.CurrentBucket(tx).Get([]byte(key))

		fieldType, valueType := boltz.GetTypeAndValue(value)

		if len(valueType) != 0 {
			valueString = boltz.FieldToString(fieldType, valueType)
		} else {
			nilStr := "nil"
			valueString = &nilStr
		}

		return nil
	})

	return *valueString
}

// Entry is a struct that represents a value field from the bbolt database with the key being set to the Name property.
// Type information and string representations of the value are also provided.
type Entry struct {
	Name        string
	Type        boltz.FieldType
	TypeString  string
	Value       []byte
	ValueString *string
}
