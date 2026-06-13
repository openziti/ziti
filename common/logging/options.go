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
	"time"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
)

// Default AsyncOptions values. See doc/design/logging-refactor.md.
const (
	DefaultQueueSize       = 4096
	DefaultBlockThreshold  = slog.LevelWarn
	DefaultSummaryInterval = 5 * time.Second
)

// Flag names used by AddFlags / OptionsFromFlags.
const (
	FlagQueueSize       = "log-queue-size"
	FlagBlockThreshold  = "log-block-threshold"
	FlagSummaryInterval = "log-summary-interval"
)

// AsyncOptions configures the AsyncHandler queue, block-threshold, and summary
// cadence. Zero values are not valid; use DefaultOptions as a starting point.
type AsyncOptions struct {
	// QueueSize is the bounded capacity of the records queue between the
	// handler's caller and the drain goroutine.
	QueueSize int

	// BlockThreshold is the lowest level at which Handle blocks when the queue
	// is full. Records strictly below this level are dropped under saturation
	// and counted toward the next summary line.
	BlockThreshold slog.Level

	// SummaryInterval is the cadence at which the drain emits a drop-summary
	// record when any per-level drop counter is non-zero.
	SummaryInterval time.Duration
}

// DefaultOptions returns AsyncOptions with the defaults documented in
// doc/design/logging-refactor.md.
func DefaultOptions() AsyncOptions {
	return AsyncOptions{
		QueueSize:       DefaultQueueSize,
		BlockThreshold:  DefaultBlockThreshold,
		SummaryInterval: DefaultSummaryInterval,
	}
}

// Validate returns an error if any field is outside its valid range.
func (o AsyncOptions) Validate() error {
	if o.QueueSize < 1 {
		return errors.Errorf("QueueSize must be >= 1, got %d", o.QueueSize)
	}
	if o.BlockThreshold < LevelTrace || o.BlockThreshold > LevelPanic {
		return errors.Errorf("BlockThreshold %v is outside the canonical level range (%v..%v)", o.BlockThreshold, LevelTrace, LevelPanic)
	}
	if o.SummaryInterval <= 0 {
		return errors.Errorf("SummaryInterval must be > 0, got %v", o.SummaryInterval)
	}
	return nil
}

// AddFlags binds AsyncOptions to a pflag.FlagSet. The flag values default to
// DefaultOptions; OptionsFromFlags reads them back after the flag set has
// been parsed.
func AddFlags(fs *pflag.FlagSet) {
	d := DefaultOptions()
	fs.Int(FlagQueueSize, d.QueueSize, "bounded capacity of the async log queue")
	fs.String(FlagBlockThreshold, LevelName(d.BlockThreshold), "lowest log level that blocks under queue saturation (panic, fatal, error, warn, info, debug, trace)")
	fs.Duration(FlagSummaryInterval, d.SummaryInterval, "cadence of the drop-summary log line emitted when records have been dropped")
}

// OptionsFromFlags reads AsyncOptions from a parsed pflag.FlagSet. Any flag
// that wasn't added via AddFlags falls back to the DefaultOptions value, so
// the caller can mix AddFlags-managed flags with a hand-rolled FlagSet
// without surprises. The returned options are Validate-checked.
func OptionsFromFlags(fs *pflag.FlagSet) (AsyncOptions, error) {
	opts := DefaultOptions()

	if q, err := fs.GetInt(FlagQueueSize); err == nil {
		opts.QueueSize = q
	}
	if s, err := fs.GetString(FlagBlockThreshold); err == nil && s != "" {
		lvl, err := ParseLevel(s)
		if err != nil {
			return opts, errors.Wrapf(err, "--%s", FlagBlockThreshold)
		}
		opts.BlockThreshold = lvl
	}
	if d, err := fs.GetDuration(FlagSummaryInterval); err == nil {
		opts.SummaryInterval = d
	}

	if err := opts.Validate(); err != nil {
		return opts, err
	}
	return opts, nil
}
