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

package raft

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIndexTracker(t *testing.T) {
	indexTracker := NewIndexTracker()

	req := require.New(t)
	indexTracker.NotifyOfIndex(1)
	req.NoError(indexTracker.WaitForIndex(1, time.Now())) // if it's already complete, should work

	// If it never completes, should fail
	req.Error(indexTracker.WaitForIndex(2, time.Now().Add(20*time.Millisecond)))

	notifyAsync := func(index uint64, after time.Duration) {
		go func() {
			time.Sleep(after)
			indexTracker.NotifyOfIndex(index)
		}()
	}

	notifyAsync(2, 20*time.Millisecond)
	req.NoError(indexTracker.WaitForIndex(2, time.Now().Add(30*time.Millisecond)))
	req.NoError(indexTracker.WaitForIndex(2, time.Now()))

	waitAsync := func(index uint64, timeout time.Duration) <-chan error {
		result := make(chan error, 1)
		go func() {
			err := indexTracker.WaitForIndex(index, time.Now().Add(timeout))
			if err == nil {
				close(result)
			} else {
				result <- err
				close(result)
			}
		}()
		return result
	}

	notifyAsync(3, 20*time.Millisecond)
	var results []<-chan error
	for i := 0; i < 10; i++ {
		results = append(results, waitAsync(3, 30*time.Millisecond))
	}
	req.Error(indexTracker.WaitForIndex(3, time.Now().Add(10*time.Millisecond)))
	time.Sleep(15 * time.Millisecond)

	for _, result := range results {
		var err error
		select {
		case err = <-result:
		default:
		}
		req.NoError(err)

		closed := false
		select {
		case <-result:
			closed = true
		default:
		}
		req.True(closed)
	}
}
