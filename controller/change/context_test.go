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

package change

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnixNanosRoundTrip(t *testing.T) {
	t.Run("zero time maps to 0 and back", func(t *testing.T) {
		require.Equal(t, int64(0), TimeToUnixNanos(time.Time{}))
		require.True(t, UnixNanosToTime(0).IsZero())
	})

	t.Run("a real time survives the round trip to the nanosecond", func(t *testing.T) {
		orig := time.Unix(1783090265, 36219890)
		require.Equal(t, orig.UnixNano(), UnixNanosToTime(TimeToUnixNanos(orig)).UnixNano())
	})
}

func TestContextTimestampProtoRoundTrip(t *testing.T) {
	t.Run("timestamp survives ToProtoBuf/FromProtoBuf", func(t *testing.T) {
		ctx := New()
		ctx.Timestamp = time.Unix(1783090265, 36219890)

		restored := FromProtoBuf(ctx.ToProtoBuf())
		require.Equal(t, ctx.Timestamp.UnixNano(), restored.Timestamp.UnixNano())
	})

	t.Run("an unset timestamp stays zero", func(t *testing.T) {
		restored := FromProtoBuf(New().ToProtoBuf())
		require.True(t, restored.Timestamp.IsZero())
	})
}
