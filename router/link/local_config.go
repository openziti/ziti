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

package link

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/openziti/channel/v5"
)

// LocalYamlConfig is the slice of router env config we need to translate
// into a Config. The matching method here keeps this package decoupled
// from router/env (which would create a circular import via router/link's
// other consumers).
type LocalYamlConfig struct {
	Listeners              []map[interface{}]interface{}
	Dialers                []map[interface{}]interface{}
	Heartbeats             channel.HeartbeatOptions
	PayloadSenderQueueSize int
	AckSenderQueueSize     int
}

// ConfigFromLocalYaml converts the YAML-decoded router config slice into a
// typed Config and serializes it to JSON, ready for
// managedconfig.Registry.ApplyLocal. Returns ("", nil) when no listeners
// or dialers are configured locally — caller treats that as "no local
// config; controller may manage."
//
// Heartbeats and queue sizes alone don't count as local content: the
// router env loader auto-fills heartbeat defaults regardless of whether
// the YAML has a `link:` section, so treating those as "operator set
// local config" would suppress controller management for every router.
// Operator intent for local-wins requires explicit listeners or dialers.
func ConfigFromLocalYaml(in LocalYamlConfig) (string, error) {
	hasContent := len(in.Listeners) > 0 || len(in.Dialers) > 0
	if !hasContent {
		return "", nil
	}

	cfg := Config{
		PayloadSenderQueueSize: in.PayloadSenderQueueSize,
		AckSenderQueueSize:     in.AckSenderQueueSize,
	}

	for i, raw := range in.Listeners {
		l, err := listenerFromYamlMap(raw)
		if err != nil {
			return "", fmt.Errorf("listener[%d]: %w", i, err)
		}
		cfg.Listeners = append(cfg.Listeners, l)
	}
	for i, raw := range in.Dialers {
		d, err := dialerFromYamlMap(raw)
		if err != nil {
			return "", fmt.Errorf("dialer[%d]: %w", i, err)
		}
		cfg.Dialers = append(cfg.Dialers, d)
	}
	if h := heartbeatsFromYamlOptions(in.Heartbeats); h != nil {
		cfg.Heartbeats = h
	}

	js, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal local config: %w", err)
	}
	return string(js), nil
}

// listenerFromYamlMap extracts a typed ListenerConfig from a raw YAML map.
// Unknown keys are ignored (the underlying factory.CreateListener will
// re-validate against its own expectations).
func listenerFromYamlMap(m map[interface{}]interface{}) (ListenerConfig, error) {
	var out ListenerConfig
	var err error
	if out.Binding, err = yamlString(m, "binding"); err != nil {
		return out, err
	}
	if out.Bind, err = yamlString(m, "bind"); err != nil {
		return out, err
	}
	if out.Advertise, err = yamlString(m, "advertise"); err != nil {
		return out, err
	}
	if out.BindInterface, err = yamlString(m, "bindInterface"); err != nil {
		return out, err
	}
	if out.Groups, err = yamlGroups(m, "groups"); err != nil {
		return out, err
	}
	if out.Options, err = yamlChannelOptions(m, "options"); err != nil {
		return out, err
	}
	return out, nil
}

func dialerFromYamlMap(m map[interface{}]interface{}) (DialerConfig, error) {
	var out DialerConfig
	var err error
	if out.Binding, err = yamlString(m, "binding"); err != nil {
		return out, err
	}
	if out.MaxDefaultConnections, err = yamlPositiveInt(m, "maxDefaultConnections"); err != nil {
		return out, err
	}
	if out.MaxAckConnections, err = yamlIntPtr(m, "maxAckConnections"); err != nil {
		return out, err
	}
	if out.Split, err = yamlBoolPtr(m, "split"); err != nil {
		return out, err
	}
	if out.StartupDelay, err = yamlString(m, "startupDelay"); err != nil {
		return out, err
	}
	if out.BindInterface, err = yamlString(m, "bindInterface"); err != nil {
		return out, err
	}
	if out.BindInterface == "" {
		if out.BindInterface, err = yamlString(m, "bind"); err != nil {
			return out, err
		}
	}
	if out.Groups, err = yamlGroups(m, "groups"); err != nil {
		return out, err
	}
	if out.HealthyDialBackoff, err = yamlBackoff(m, "healthyDialBackoff"); err != nil {
		return out, err
	}
	if out.UnhealthyDialBackoff, err = yamlBackoff(m, "unhealthyDialBackoff"); err != nil {
		return out, err
	}
	if out.Options, err = yamlChannelOptions(m, "options"); err != nil {
		return out, err
	}
	return out, nil
}

func yamlChannelOptions(m map[interface{}]interface{}, key string) (*ChannelOptions, error) {
	sub, found, err := yamlSubmap(m, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	out := &ChannelOptions{}
	if out.OutQueueSize, err = yamlNonNegativeIntPtr(sub, "outQueueSize"); err != nil {
		return nil, err
	}
	if out.MaxQueuedConnects, err = yamlNonNegativeIntPtr(sub, "maxQueuedConnects"); err != nil {
		return nil, err
	}
	if out.MaxOutstandingConnects, err = yamlPositiveInt(sub, "maxOutstandingConnects"); err != nil {
		return nil, err
	}
	if out.ConnectTimeout, err = yamlString(sub, "connectTimeout"); err != nil {
		return nil, err
	}
	if out.ConnectTimeout == "" {
		// Legacy key: connectTimeoutMs is the integer-millisecond form the channel
		// loader historically accepted. Read it only when the connectTimeout
		// duration string is absent, so existing local configs keep their timeout.
		ms, err := yamlIntPtr(sub, "connectTimeoutMs")
		if err != nil {
			return nil, err
		}
		if ms != nil {
			out.ConnectTimeout = (time.Duration(*ms) * time.Millisecond).String()
		}
	}
	if out.WriteTimeout, err = yamlString(sub, "writeTimeout"); err != nil {
		return nil, err
	}
	return out, nil
}

func yamlBackoff(m map[interface{}]interface{}, key string) (*BackoffConfig, error) {
	sub, found, err := yamlSubmap(m, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	out := &BackoffConfig{}
	if out.RetryBackoffFactor, err = yamlPositiveFloat(sub, "retryBackoffFactor"); err != nil {
		return nil, err
	}
	if out.MinRetryInterval, err = yamlString(sub, "minRetryInterval"); err != nil {
		return nil, err
	}
	if out.MaxRetryInterval, err = yamlString(sub, "maxRetryInterval"); err != nil {
		return nil, err
	}
	return out, nil
}

// heartbeatsFromYamlOptions converts the channel-package HeartbeatOptions
// (already parsed by the router env loader) into the typed
// HeartbeatsConfig in duration-string form for round-trip through the
// router.link.v1 JSON shape.
func heartbeatsFromYamlOptions(h channel.HeartbeatOptions) *HeartbeatsConfig {
	if !heartbeatsConfigured(h) {
		return nil
	}
	return &HeartbeatsConfig{
		SendInterval:             durationString(h.SendInterval),
		CheckInterval:            durationString(h.CheckInterval),
		CloseUnresponsiveTimeout: durationString(h.CloseUnresponsiveTimeout),
	}
}

func heartbeatsConfigured(h channel.HeartbeatOptions) bool {
	return h.SendInterval != 0 || h.CheckInterval != 0 || h.CloseUnresponsiveTimeout != 0
}

func durationString(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

// yamlString reads m[key] as string; returns "" if absent.
func yamlString(m map[interface{}]interface{}, key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q: expected string, got %T", key, v)
	}
	return s, nil
}

// yamlInt reads m[key] as int; returns 0 if absent. Accepts the integer
// types YAML's go-yaml might produce.
func yamlInt(m map[interface{}]interface{}, key string) (int, error) {
	v, ok := m[key]
	if !ok {
		return 0, nil
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		// YAML may decode an integer literal as float64; accept it only when it
		// is exactly integral and in range, so fractional (0.5) or overflowing
		// values are rejected rather than silently truncated.
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("field %q: expected integer, got fractional %v", key, n)
		}
		if n > math.MaxInt64 || n < math.MinInt64 {
			return 0, fmt.Errorf("field %q: integer value %v out of range", key, n)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("field %q: expected integer, got %T", key, v)
	}
}

// yamlPositiveInt reads m[key] as an int and rejects anything below 1.
//
// Fields read through this helper are dropped from the translated config when
// they are not positive, because the config-to-map translation only emits
// values greater than zero. Without this check an explicit 0 or negative would
// be silently replaced by the subsystem default instead of reported, so the
// error has to be raised here where key presence is still known.
func yamlPositiveInt(m map[interface{}]interface{}, key string) (int, error) {
	n, err := yamlInt(m, key)
	if err != nil {
		return 0, err
	}
	if _, present := m[key]; present && n < 1 {
		return 0, fmt.Errorf("field %q: must be at least 1, got %d", key, n)
	}
	return n, nil
}

// yamlNonNegativeIntPtr reads m[key] as *int, returning nil when the key is
// absent so an explicit 0 stays distinguishable from unset. Only negative
// values are rejected.
//
// Used for the fields where zero is meaningful rather than invalid: it sizes a
// buffered channel, and zero requests an unbuffered one. Callers must emit the
// value whenever it is non-nil, or the round-trip loses the explicit zero.
func yamlNonNegativeIntPtr(m map[interface{}]interface{}, key string) (*int, error) {
	p, err := yamlIntPtr(m, key)
	if err != nil || p == nil {
		return nil, err
	}
	if *p < 0 {
		return nil, fmt.Errorf("field %q: must not be negative, got %d", key, *p)
	}
	return p, nil
}

// yamlPositiveFloat reads m[key] as a float64 and rejects anything not greater
// than zero. Same rationale as yamlPositiveInt: non-positive values would be
// dropped during translation and silently defaulted.
func yamlPositiveFloat(m map[interface{}]interface{}, key string) (float64, error) {
	f, err := yamlFloat(m, key)
	if err != nil {
		return 0, err
	}
	if _, present := m[key]; present && f <= 0 {
		return 0, fmt.Errorf("field %q: must be greater than 0, got %v", key, f)
	}
	return f, nil
}

// yamlIntPtr reads m[key] as *int, returning nil when the key is absent so an
// explicitly-set value (including 0) is distinguishable from unset.
func yamlIntPtr(m map[interface{}]interface{}, key string) (*int, error) {
	if _, ok := m[key]; !ok {
		return nil, nil
	}
	n, err := yamlInt(m, key)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// yamlBoolPtr reads m[key] as *bool, returning nil when the key is absent so an
// explicitly-set value (including false) is distinguishable from unset.
func yamlBoolPtr(m map[interface{}]interface{}, key string) (*bool, error) {
	v, ok := m[key]
	if !ok {
		return nil, nil
	}
	b, ok := v.(bool)
	if !ok {
		return nil, fmt.Errorf("field %q: expected bool, got %T", key, v)
	}
	return &b, nil
}

func yamlFloat(m map[interface{}]interface{}, key string) (float64, error) {
	v, ok := m[key]
	if !ok {
		return 0, nil
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("field %q: expected number, got %T", key, v)
	}
}

func yamlGroups(m map[interface{}]interface{}, key string) (Groups, error) {
	v, ok := m[key]
	if !ok {
		return nil, nil
	}
	switch g := v.(type) {
	case string:
		return Groups{g}, nil
	case []interface{}:
		out := make(Groups, 0, len(g))
		for i, item := range g {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("field %q[%d]: expected string, got %T", key, i, item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("field %q: expected string or array, got %T", key, v)
	}
}

// yamlSubmap returns the nested map at key. found is false only when the key is
// absent; err is non-nil when the key is present but not a map. Distinguishing
// the two lets callers fail fast on a malformed value instead of silently
// treating it as absent and falling back to defaults.
func yamlSubmap(m map[interface{}]interface{}, key string) (sub map[interface{}]interface{}, found bool, err error) {
	v, ok := m[key]
	if !ok {
		return nil, false, nil
	}
	sub, ok = v.(map[interface{}]interface{})
	if !ok {
		return nil, true, fmt.Errorf("field %q: expected map, got %T", key, v)
	}
	return sub, true, nil
}
