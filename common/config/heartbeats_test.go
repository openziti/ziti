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

package config

import (
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/stretchr/testify/require"
)

func TestValidateHeartbeatOptions(t *testing.T) {
	options := func(send, check, closeTimeout time.Duration) *channel.HeartbeatOptions {
		return &channel.HeartbeatOptions{
			SendInterval:             send,
			CheckInterval:            check,
			CloseUnresponsiveTimeout: closeTimeout,
		}
	}

	tests := []struct {
		name    string
		options *channel.HeartbeatOptions
		errIs   string
	}{
		{
			name:    "the library defaults are accepted",
			options: channel.DefaultHeartbeatOptions(),
		},
		{
			name:    "nil is accepted, since an absent config is the caller's default",
			options: nil,
		},
		{
			name:    "a cadence well inside the timeout is accepted",
			options: options(10*time.Second, time.Second, 30*time.Second),
		},
		{
			name:    "a zero check interval is rejected, since the pulse ticker panics on it",
			options: options(10*time.Second, 0, 30*time.Second),
			errIs:   "checkInterval",
		},
		{
			name:    "a negative check interval is rejected",
			options: options(10*time.Second, -time.Second, 30*time.Second),
			errIs:   "checkInterval",
		},
		{
			name:    "a check interval above the timeout is rejected",
			options: options(10*time.Second, time.Minute, 30*time.Second),
			errIs:   "checkInterval",
		},
		{
			name:    "a check interval equal to the timeout is rejected",
			options: options(10*time.Second, 30*time.Second, 30*time.Second),
			errIs:   "checkInterval",
		},
		{
			name:    "a send interval above the timeout is rejected",
			options: options(time.Minute, time.Second, 30*time.Second),
			errIs:   "sendInterval",
		},
		{
			name:    "a send interval equal to the timeout is rejected",
			options: options(30*time.Second, time.Second, 30*time.Second),
			errIs:   "sendInterval",
		},
		{
			// A zero timeout disables the teardown, so there is no deadline to outpace and only the
			// ticker's own requirement is left to enforce.
			name:    "a disabled teardown leaves the intervals unconstrained",
			options: options(time.Hour, time.Minute, 0),
		},
		{
			name:    "a disabled teardown still rejects a zero check interval",
			options: options(10*time.Second, 0, 0),
			errIs:   "checkInterval",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateHeartbeatOptions(test.options)
			if test.errIs == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), test.errIs)
		})
	}
}
