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
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_readLeadershipTerm locks in that leadership is reported with the raft term it was read in, so a
// controller never claims to lead a raft term another controller led.
func Test_readLeadershipTerm(t *testing.T) {
	t.Run("leadership in a settled raft term is reported with that term", func(t *testing.T) {
		req := require.New(t)

		// arrange
		r := &scriptedRaft{terms: []uint64{5}}

		// act
		raftTerm, leader := readLeadershipTerm(r.currentTerm, r.isLeader)

		// assert
		req.True(leader)
		req.Equal(uint64(5), raftTerm)
	})

	t.Run("leadership won between reads is reported with the raft term it was won in", func(t *testing.T) {
		req := require.New(t)

		// arrange: a follower in raft term 5 is elected leader of raft term 6 after the first term read
		r := &scriptedRaft{terms: []uint64{5, 6}, leaderAfter: 1}

		// act
		raftTerm, leader := readLeadershipTerm(r.currentTerm, r.isLeader)

		// assert
		req.True(leader)
		req.Equal(uint64(6), raftTerm, "reported with raft term 5, it would claim a term another controller led")
	})

	t.Run("elections that never settle report no leadership", func(t *testing.T) {
		req := require.New(t)

		// arrange
		r := &scriptedRaft{electing: true}

		// act
		_, leader := readLeadershipTerm(r.currentTerm, r.isLeader)

		// assert
		req.False(leader, "without two agreeing raft term reads, leadership must not be claimed")
	})
}

// scriptedRaft stands in for raft's term and state reads. currentTerm returns terms in order, repeating
// the last, and isLeader reports leadership once leaderAfter terms have been read. With electing set,
// currentTerm instead returns a higher term on every read and isLeader always reports leadership.
type scriptedRaft struct {
	terms       []uint64
	leaderAfter int
	electing    bool
	reads       int
}

func (self *scriptedRaft) currentTerm() uint64 {
	self.reads++
	if self.electing {
		return uint64(self.reads)
	}
	return self.terms[min(self.reads, len(self.terms))-1]
}

func (self *scriptedRaft) isLeader() bool {
	return self.electing || self.reads >= self.leaderAfter
}
