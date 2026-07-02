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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type recordingHeartbeatControl struct {
	send  time.Duration
	check time.Duration
	calls int
}

func (self *recordingHeartbeatControl) UpdateIntervals(send, check time.Duration) {
	self.send = send
	self.check = check
	self.calls++
}

func Test_heartbeatControl_UpdatesCurrent(t *testing.T) {
	req := require.New(t)

	var hc heartbeatControl
	a := &recordingHeartbeatControl{}
	hc.SetHeartbeatControl(a)

	hc.UpdateHeartbeatIntervals(2*time.Second, 200*time.Millisecond)

	req.Equal(1, a.calls)
	req.Equal(2*time.Second, a.send)
	req.Equal(200*time.Millisecond, a.check)
}

// A deprecated split link registers a control per channel; only the last is
// retained, so a live retune reaches just that one.
func Test_heartbeatControl_LastRegisteredWins(t *testing.T) {
	req := require.New(t)

	var hc heartbeatControl
	a := &recordingHeartbeatControl{}
	b := &recordingHeartbeatControl{}
	hc.SetHeartbeatControl(a)
	hc.SetHeartbeatControl(b)

	hc.UpdateHeartbeatIntervals(time.Second, time.Second)

	req.Equal(0, a.calls, "the earlier control is replaced")
	req.Equal(1, b.calls)
}

func Test_heartbeatControl_NoControlIsNoop(t *testing.T) {
	var hc heartbeatControl
	// must not panic with nothing registered
	hc.UpdateHeartbeatIntervals(time.Second, time.Second)
}
