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

	t.Run("index 1 can be set and notifies", func(t *testing.T) {
		req := require.New(t)
		indexTracker := &testTracker{
			NewIndexTracker(),
		}

		indexTracker.NotifyOfIndex(1)
		req.NoError(indexTracker.WaitForIndex(1, time.Now())) // if it's already complete, should work

		// If it never completes, should fail
		req.Error(indexTracker.WaitForIndex(2, time.Now().Add(100*time.Millisecond)))
	})

	t.Run("index 2 is received. no error", func(t *testing.T) {
		req := require.New(t)

		indexTracker := &testTracker{
			NewIndexTracker(),
		}

		indexTracker.notifyAsync(1, 20*time.Millisecond)
		indexTracker.notifyAsync(2, 20*time.Millisecond)

		req.NoError(indexTracker.WaitForIndex(2, time.Now().Add(100*time.Millisecond)))
		req.NoError(indexTracker.WaitForIndex(2, time.Now()))

	})

	t.Run("index 3 has time out errors until it is added", func(t *testing.T) {
		req := require.New(t)

		indexTracker := &testTracker{
			NewIndexTracker(),
		}

		//move the index forward 2
		indexTracker.notifyAsync(1, 5*time.Millisecond)
		indexTracker.notifyAsync(2, 20*time.Millisecond)

		//wait for index 2 to appear
		req.NoError(indexTracker.WaitForIndex(2, time.Now().Add(100*time.Millisecond)))

		//notify of index 3 after a delay
		//during the delay check to see if the index has arrived after varying levels of timeouts
		indexTracker.notifyAsync(3, 200*time.Millisecond)

		var results []<-chan error

		//add waits for index 3 starting a 30ms and increased by 30ms till 330ms
		for i := 0; i < 10; i++ {
			results = append(results, indexTracker.waitAsync(3, time.Duration((i+1)*30)*time.Millisecond))
		}

		//once index3Notified is true, no timeout errors should be received
		index3Notified := false

		for _, result := range results {
			err := <-result

			if index3Notified {
				if err != nil {
					req.Fail("received error after first notification of index received, expected no more errors")
				}
			} else {
				//no notification yet, if no error, that is the notification
				if err == nil {
					index3Notified = true
				}
			}
		}

		//make sure we didn't receive all errors and index 3 was eventually notified
		req.True(index3Notified, "index 3 was never received")
	})
}

// testTracker adds helper function used to power async index notification used in the above tests.
type testTracker struct {
	IndexTracker
}

func (t *testTracker) notifyAsync(index uint64, after time.Duration) {
	go func() {
		time.Sleep(after)
		t.NotifyOfIndex(index)
	}()
}

func (t *testTracker) waitAsync(index uint64, timeout time.Duration) <-chan error {
	result := make(chan error, 1)
	go func() {
		err := t.WaitForIndex(index, time.Now().Add(timeout))
		if err == nil {
			close(result)
		} else {
			result <- err
			close(result)
		}
	}()
	return result
}
