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

package xlink_transport

import (
	"sync"
	"testing"

	"github.com/openziti/channel/v5"

	"github.com/stretchr/testify/require"
)

// TestSplitImpl_IsClosedOnHalfBuiltLink: the listener binds a split link's two channels independently, as
// each arrives, so the link exists with only one of them set. Close has always skipped a nil channel, while
// IsClosed dereferenced both, which made asking whether such a link was closed a panic.
func TestSplitImpl_IsClosedOnHalfBuiltLink(t *testing.T) {
	req := require.New(t)

	req.False((&splitImpl{}).IsClosed(), "a link with neither channel bound is not closed")
	req.NotPanics(func() { _ = (&splitImpl{}).IsClosed() })
}

// splitRaceTestChannel is a double supplying only what IsClosed reads. It embeds its interface, so a method
// the test does not expect panics rather than returning a zero value.
type splitRaceTestChannel struct {
	channel.Channel
}

func (self *splitRaceTestChannel) IsClosed() bool { return false }

// TestSplitImpl_IsClosedWhileChannelsBind: the two channels are bound independently, each under the link's
// lock, while IsClosed is called from the link registry's event loop without it. The event loop must not
// block on a bind, so the fields are read atomically rather than under the lock that guards the binding.
func TestSplitImpl_IsClosedWhileChannelsBind(t *testing.T) {
	link := &splitImpl{}

	bound := make(chan struct{})
	reading := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-reading // bind only once the reader is live, or the two never overlap
		_ = link.syncInit(func() error {
			link.payloadCh.Store(&splitRaceTestChannel{})
			return nil
		})
		_ = link.syncInit(func() error {
			link.ackCh.Store(&splitRaceTestChannel{})
			return nil
		})
		close(bound)
	}()

	go func() {
		defer wg.Done()
		close(reading)
		for {
			_ = link.IsClosed()
			select {
			case <-bound:
				return
			default:
			}
		}
	}()

	wg.Wait()
	require.False(t, link.IsClosed(), "a fully bound link with two open channels is not closed")
}
