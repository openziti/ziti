/*
	(c) Copyright NetFoundry Inc.

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

package link

import (
	"container/heap"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLinkStateHeap(t *testing.T) {
	h := &linkStateHeap{}

	start := time.Now()

	heap.Push(h, &linkState{
		nextDial: start.Add(time.Second),
	})

	heap.Push(h, &linkState{
		nextDial: start,
	})

	heap.Push(h, &linkState{
		nextDial: start.Add(5 * time.Second),
	})

	heap.Push(h, &linkState{
		nextDial: start.Add(3 * time.Second),
	})

	heap.Push(h, &linkState{
		nextDial: start.Add(7 * time.Second),
	})

	req := require.New(t)

	checkNext := func(t time.Time) {
		req.Equal(t, ((*h)[0]).nextDial)
		req.Equal(t, heap.Pop(h).(*linkState).nextDial)
	}

	checkNext(start)
	checkNext(start.Add(time.Second))
	checkNext(start.Add(3 * time.Second))
	checkNext(start.Add(5 * time.Second))
	checkNext(start.Add(7 * time.Second))
	req.Equal(0, h.Len())
}
