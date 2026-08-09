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

package diagnostics

import (
	"time"

	"github.com/pkg/errors"
)

// Config keys, shared so a controller and a router are configured the same way.
const (
	MapKey                       = "diagnostics"
	SlowHandlersMapKey           = "slowHandlers"
	SlowHandlersEnabledKey       = "enabled"
	SlowHandlersWarnKey          = "warnThreshold"
	SlowHandlersDumpThresholdKey = "stackDumpThreshold"
	SlowHandlersDumpIntervalKey  = "stackDumpInterval"
	SlowHandlersMaxSignaturesKey = "maxStackDumps"
	SlowHandlersMinGroupKey      = "minGoroutineGroup"
	SlowHandlersMaxBytesKey      = "maxStackDumpBytes"
	SlowHandlersDirKey           = "stackDumpDir"
)

// LoadConfig reads the diagnostics section from a parsed config map, returning defaults when it is
// absent. Unset values keep their defaults, so a config need only name what it changes.
func LoadConfig(cfgmap map[interface{}]interface{}) (SlowHandlerConfig, error) {
	cfg := DefaultSlowHandlerConfig()

	section, found := submap(cfgmap, MapKey)
	if !found {
		return cfg, nil
	}
	slow, found := submap(section, SlowHandlersMapKey)
	if !found {
		return cfg, nil
	}

	var err error
	if cfg.Enabled, err = boolAt(slow, SlowHandlersEnabledKey, cfg.Enabled); err != nil {
		return cfg, err
	}
	if cfg.WarnThreshold, err = durationAt(slow, SlowHandlersWarnKey, cfg.WarnThreshold); err != nil {
		return cfg, err
	}
	if cfg.StackDumpThreshold, err = durationAt(slow, SlowHandlersDumpThresholdKey, cfg.StackDumpThreshold); err != nil {
		return cfg, err
	}
	if cfg.StackDumpInterval, err = durationAt(slow, SlowHandlersDumpIntervalKey, cfg.StackDumpInterval); err != nil {
		return cfg, err
	}
	if cfg.MaxSignatures, err = intAt(slow, SlowHandlersMaxSignaturesKey, cfg.MaxSignatures); err != nil {
		return cfg, err
	}
	if cfg.MinGroup, err = intAt(slow, SlowHandlersMinGroupKey, cfg.MinGroup); err != nil {
		return cfg, err
	}
	if cfg.MaxBytes, err = intAt(slow, SlowHandlersMaxBytesKey, cfg.MaxBytes); err != nil {
		return cfg, err
	}
	if value, found := slow[SlowHandlersDirKey]; found {
		str, ok := value.(string)
		if !ok {
			return cfg, errors.Errorf("[%v/%v/%v] must be a string", MapKey, SlowHandlersMapKey, SlowHandlersDirKey)
		}
		cfg.Dir = str
	}

	// The buffer grows from the initial size to the max, so a max below it would truncate every dump.
	if cfg.MaxBytes < cfg.InitialBytes {
		cfg.InitialBytes = cfg.MaxBytes
	}

	return cfg, cfg.Validate()
}

// Validate reports a configuration that would produce no useful output. It is called by LoadConfig, and
// is exported for callers building a config by hand.
func (self SlowHandlerConfig) Validate() error {
	if !self.Enabled {
		return nil
	}
	if self.StackDumpThreshold <= 0 {
		return errors.Errorf("[%v/%v/%v] must be positive", MapKey, SlowHandlersMapKey, SlowHandlersDumpThresholdKey)
	}
	if self.WarnThreshold <= 0 {
		return errors.Errorf("[%v/%v/%v] must be positive", MapKey, SlowHandlersMapKey, SlowHandlersWarnKey)
	}
	if self.MaxSignatures <= 0 {
		return errors.Errorf("[%v/%v/%v] must be positive, or no dump is ever written",
			MapKey, SlowHandlersMapKey, SlowHandlersMaxSignaturesKey)
	}
	// Nothing here rejects a snapshot before it is taken: the stack has to be captured to be identified,
	// so an interval that never rate limits means every slow handler pays for a snapshot of every
	// goroutine, whether or not the result is allowed to be written.
	if self.StackDumpInterval <= 0 {
		return errors.Errorf("[%v/%v/%v] must be positive, or every slow handler snapshots every goroutine",
			MapKey, SlowHandlersMapKey, SlowHandlersDumpIntervalKey)
	}
	if self.MinGroup <= 0 {
		return errors.Errorf("[%v/%v/%v] must be positive", MapKey, SlowHandlersMapKey, SlowHandlersMinGroupKey)
	}
	if self.MaxBytes <= 0 {
		return errors.Errorf("[%v/%v/%v] must be positive", MapKey, SlowHandlersMapKey, SlowHandlersMaxBytesKey)
	}
	return nil
}

func submap(m map[interface{}]interface{}, key string) (map[interface{}]interface{}, bool) {
	value, found := m[key]
	if !found {
		return nil, false
	}
	sub, ok := value.(map[interface{}]interface{})
	return sub, ok
}

func boolAt(m map[interface{}]interface{}, key string, def bool) (bool, error) {
	value, found := m[key]
	if !found {
		return def, nil
	}
	b, ok := value.(bool)
	if !ok {
		return def, errors.Errorf("[%v/%v/%v] must be true or false", MapKey, SlowHandlersMapKey, key)
	}
	return b, nil
}

func intAt(m map[interface{}]interface{}, key string, def int) (int, error) {
	value, found := m[key]
	if !found {
		return def, nil
	}
	i, ok := value.(int)
	if !ok {
		return def, errors.Errorf("[%v/%v/%v] must be a whole number", MapKey, SlowHandlersMapKey, key)
	}
	return i, nil
}

// durationAt accepts a Go duration string, so a threshold reads as 50ms rather than a bare number whose
// unit the reader has to know.
func durationAt(m map[interface{}]interface{}, key string, def time.Duration) (time.Duration, error) {
	value, found := m[key]
	if !found {
		return def, nil
	}
	str, ok := value.(string)
	if !ok {
		return def, errors.Errorf("[%v/%v/%v] must be a duration string, e.g. 50ms or 2m",
			MapKey, SlowHandlersMapKey, key)
	}
	d, err := time.ParseDuration(str)
	if err != nil {
		return def, errors.Wrapf(err, "cannot parse [%v/%v/%v]", MapKey, SlowHandlersMapKey, key)
	}
	return d, nil
}
