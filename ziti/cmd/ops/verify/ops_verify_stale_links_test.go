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

package verify

import (
	"bytes"
	"testing"

	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/stretchr/testify/require"
)

func newTestAction(buf *bytes.Buffer, bufferedResults ...*mgmt_pb.StaleLinkResult) *verifyStaleLinksAction {
	action := &verifyStaleLinksAction{out: buf}
	action.resultsChan = make(chan *mgmt_pb.StaleLinkResult, len(bufferedResults)+1)
	for _, r := range bufferedResults {
		action.resultsChan <- r
	}
	return action
}

func TestConsumeResults_AllArrive(t *testing.T) {
	req := require.New(t)
	buf := &bytes.Buffer{}
	action := newTestAction(buf,
		&mgmt_pb.StaleLinkResult{LinkId: "l1"},
		&mgmt_pb.StaleLinkResult{LinkId: "l2", Stale: true},
		&mgmt_pb.StaleLinkResult{LinkId: "l3", Partial: true},
	)

	stale, partial, missing := action.consumeResults(3, make(chan struct{}))
	req.Equal(1, stale)
	req.Equal(1, partial)
	req.Zero(missing, "a complete stream reports nothing missing")
}

func TestConsumeResults_StreamEndsEarly(t *testing.T) {
	// The management channel dropping after the initial response must not read
	// as a clean sweep: the results that never arrived are exactly the links
	// left unverified.
	req := require.New(t)
	buf := &bytes.Buffer{}
	action := newTestAction(buf, &mgmt_pb.StaleLinkResult{LinkId: "l1", Stale: true})

	closeNotify := make(chan struct{})
	close(closeNotify)

	stale, _, missing := action.consumeResults(5, closeNotify)
	req.Equal(1, stale, "the buffered result is still counted")
	req.Equal(4, missing, "exactly the un-delivered results are missing")
}

func TestConsumeResults_ClosedChannelStillDrainsBufferedResults(t *testing.T) {
	// A select with both cases ready picks at random, so a closed channel says
	// nothing about whether results are still buffered. Every expected result is
	// buffered here, so a complete run must be reported every time; repeated to
	// leave no room for the race to pass by luck.
	req := require.New(t)

	for i := 0; i < 500; i++ {
		buf := &bytes.Buffer{}
		action := newTestAction(buf,
			&mgmt_pb.StaleLinkResult{LinkId: "l1"},
			&mgmt_pb.StaleLinkResult{LinkId: "l2", Stale: true},
			&mgmt_pb.StaleLinkResult{LinkId: "l3", Partial: true},
		)

		closeNotify := make(chan struct{})
		close(closeNotify)

		stale, partial, missing := action.consumeResults(3, closeNotify)
		req.Zero(missing, "iteration %d: all results were buffered, nothing is missing", i)
		req.Equal(1, stale, "iteration %d", i)
		req.Equal(1, partial, "iteration %d", i)
	}
}

func TestConsumeResults_NothingExpected(t *testing.T) {
	req := require.New(t)
	buf := &bytes.Buffer{}
	action := newTestAction(buf)

	stale, partial, missing := action.consumeResults(0, make(chan struct{}))
	req.Zero(stale)
	req.Zero(partial)
	req.Zero(missing)
}
