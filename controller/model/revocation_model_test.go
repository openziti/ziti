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

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRevocationRevokesSessionIssuedAt(t *testing.T) {
	cutoff := time.Unix(1783090265, 0)

	t.Run("a zero cutoff revokes every session", func(t *testing.T) {
		rev := &Revocation{}
		require.True(t, rev.RevokesSessionIssuedAt(cutoff.Add(-time.Hour)))
		require.True(t, rev.RevokesSessionIssuedAt(cutoff.Add(time.Hour)))
	})

	t.Run("a session issued before the cutoff is revoked", func(t *testing.T) {
		rev := &Revocation{IssuedBefore: cutoff}
		require.True(t, rev.RevokesSessionIssuedAt(cutoff.Add(-time.Second)))
	})

	t.Run("a session issued at or after the cutoff survives", func(t *testing.T) {
		rev := &Revocation{IssuedBefore: cutoff}
		require.False(t, rev.RevokesSessionIssuedAt(cutoff))
		require.False(t, rev.RevokesSessionIssuedAt(cutoff.Add(time.Second)))
	})
}
