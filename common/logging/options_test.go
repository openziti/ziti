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

package logging

import (
	"log/slog"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func TestDefaultOptionsValid(t *testing.T) {
	require.NoError(t, DefaultOptions().Validate())
}

func TestValidateRejectsBadValues(t *testing.T) {
	tests := map[string]AsyncOptions{
		"zero queue size":           {QueueSize: 0, BlockThreshold: slog.LevelWarn, SummaryInterval: time.Second},
		"negative queue size":       {QueueSize: -1, BlockThreshold: slog.LevelWarn, SummaryInterval: time.Second},
		"threshold below trace":     {QueueSize: 1, BlockThreshold: LevelTrace - 1, SummaryInterval: time.Second},
		"threshold above panic":     {QueueSize: 1, BlockThreshold: LevelPanic + 1, SummaryInterval: time.Second},
		"zero summary interval":     {QueueSize: 1, BlockThreshold: slog.LevelWarn, SummaryInterval: 0},
		"negative summary interval": {QueueSize: 1, BlockThreshold: slog.LevelWarn, SummaryInterval: -time.Second},
	}
	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			require.Error(t, opts.Validate())
		})
	}
}

func TestValidateAcceptsBoundaryValues(t *testing.T) {
	for _, lvl := range []slog.Level{LevelTrace, LevelPanic} {
		opts := AsyncOptions{QueueSize: 1, BlockThreshold: lvl, SummaryInterval: time.Nanosecond}
		require.NoError(t, opts.Validate(), "boundary value %v should be accepted", lvl)
	}
}

// TestOptionsFromFlagsDefaults proves that an unparsed FlagSet with our flags
// bound yields exactly DefaultOptions.
func TestOptionsFromFlagsDefaults(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	AddFlags(fs)
	require.NoError(t, fs.Parse(nil))

	got, err := OptionsFromFlags(fs)
	require.NoError(t, err)
	require.Equal(t, DefaultOptions(), got)
}

func TestOptionsFromFlagsRoundTrip(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	AddFlags(fs)
	require.NoError(t, fs.Parse([]string{
		"--" + FlagQueueSize, "16",
		"--" + FlagBlockThreshold, "error",
		"--" + FlagSummaryInterval, "10s",
	}))

	got, err := OptionsFromFlags(fs)
	require.NoError(t, err)
	require.Equal(t, AsyncOptions{
		QueueSize:       16,
		BlockThreshold:  slog.LevelError,
		SummaryInterval: 10 * time.Second,
	}, got)
}

// TestOptionsFromFlagsRejectsUnknownLevel proves that a bogus level name on
// the --log-block-threshold flag surfaces as an error from OptionsFromFlags.
func TestOptionsFromFlagsRejectsUnknownLevel(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	AddFlags(fs)
	require.NoError(t, fs.Parse([]string{"--" + FlagBlockThreshold, "bogus"}))

	_, err := OptionsFromFlags(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), FlagBlockThreshold)
}

// TestOptionsFromFlagsRejectsValidatedBadValues proves Validate runs after
// the flag binding.
func TestOptionsFromFlagsRejectsValidatedBadValues(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	AddFlags(fs)
	require.NoError(t, fs.Parse([]string{"--" + FlagQueueSize, "0"}))

	_, err := OptionsFromFlags(fs)
	require.Error(t, err)
	require.Contains(t, err.Error(), "QueueSize")
}

// TestOptionsFromFlagsMissingFlagsFallBackToDefaults proves that a FlagSet
// missing our flags entirely yields DefaultOptions, so callers can mix-and-match.
func TestOptionsFromFlagsMissingFlagsFallBackToDefaults(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	require.NoError(t, fs.Parse(nil))

	got, err := OptionsFromFlags(fs)
	require.NoError(t, err)
	require.Equal(t, DefaultOptions(), got)
}
