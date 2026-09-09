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

package env

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_LoadForwarderOptions_EndpointFaultRetention(t *testing.T) {
	t.Run("an unset retention takes the default", func(t *testing.T) {
		req := require.New(t)

		// act
		options, err := LoadForwarderOptions(map[interface{}]interface{}{})

		// assert
		req.NoError(err)
		req.Equal(DefaultEndpointFaultRetention, options.EndpointFaultRetention)
	})

	t.Run("a duration string sets the retention", func(t *testing.T) {
		req := require.New(t)

		// arrange
		src := map[interface{}]interface{}{"endpointFaultRetention": "90s"}

		// act
		options, err := LoadForwarderOptions(src)

		// assert
		req.NoError(err)
		req.Equal(90*time.Second, options.EndpointFaultRetention)
	})

	t.Run("a value that is not a duration is rejected", func(t *testing.T) {
		req := require.New(t)

		// arrange
		src := map[interface{}]interface{}{"endpointFaultRetention": "not-a-duration"}

		// act
		_, err := LoadForwarderOptions(src)

		// assert
		req.ErrorContains(err, "endpointFaultRetention", "the error must name the setting")
	})

	t.Run("a zero retention is rejected", func(t *testing.T) {
		req := require.New(t)

		// arrange
		src := map[interface{}]interface{}{"endpointFaultRetention": "0s"}

		// act
		_, err := LoadForwarderOptions(src)

		// assert
		req.ErrorContains(err, "endpointFaultRetention", "a zero retention would give up on every fault at once")
	})

	t.Run("a negative retention is rejected", func(t *testing.T) {
		req := require.New(t)

		// arrange
		src := map[interface{}]interface{}{"endpointFaultRetention": "-1m"}

		// act
		_, err := LoadForwarderOptions(src)

		// assert
		req.ErrorContains(err, "endpointFaultRetention", "a negative retention must be rejected")
	})

	// Durations are configured as strings. A bare number is rejected rather than read in some unit,
	// so a millisecond value copied from a neighbouring setting can't be taken as nanoseconds.
	t.Run("a bare number is rejected", func(t *testing.T) {
		req := require.New(t)

		// arrange
		src := map[interface{}]interface{}{"endpointFaultRetention": 300000}

		// act
		_, err := LoadForwarderOptions(src)

		// assert
		req.ErrorContains(err, "duration", "the error must say a duration is expected")
	})
}
