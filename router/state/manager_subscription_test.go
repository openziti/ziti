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

package state

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_SubscriptionNeedsRenewal(t *testing.T) {
	now := time.Date(2026, time.November, 24, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{name: "two minutes remaining", expiresAt: now.Add(2 * time.Minute), want: false},
		{name: "exactly one minute remaining", expiresAt: now.Add(time.Minute), want: true},
		{name: "one millisecond remaining", expiresAt: now.Add(time.Millisecond), want: true},
		{name: "expired", expiresAt: now.Add(-time.Second), want: true},
		{name: "zero expiration", expiresAt: time.Time{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, subscriptionNeedsRenewal(now, tt.expiresAt))
		})
	}
}
