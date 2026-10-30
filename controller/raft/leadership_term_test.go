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

func Test_readLeadershipTerm(t *testing.T) {
	t.Run("stable", func(t *testing.T) {
		term, leader := readLeadershipTerm(func() uint64 { return 5 }, func() bool { return true })
		require.Equal(t, uint64(5), term)
		require.True(t, leader)
	})

	t.Run("election between reads", func(t *testing.T) {
		// A follower in term 5 is elected leader of term 6 between reading the term and reading its
		// state. Reported as (5, true), it would claim a term another controller led.
		terms := []uint64{5, 6, 6, 6}
		reads := 0
		currentTerm := func() uint64 {
			term := terms[min(reads, len(terms)-1)]
			reads++
			return term
		}
		isLeader := func() bool {
			return reads >= 1
		}

		term, leader := readLeadershipTerm(currentTerm, isLeader)
		require.True(t, leader)
		require.Equal(t, uint64(6), term, "leadership must be paired with the term it was read in")
	})

	t.Run("elections never settle", func(t *testing.T) {
		var term uint64
		currentTerm := func() uint64 {
			term++
			return term
		}

		_, leader := readLeadershipTerm(currentTerm, func() bool { return true })
		require.False(t, leader, "without a consistent read, leadership must not be claimed")
	})
}
