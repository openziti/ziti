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
)

// ConfigBaseType is the un-versioned config family this package owns.
const ConfigBaseType = "router.link"

// ConfigTypeV1 is the controller-defined ConfigType.Name for version 1.
const ConfigTypeV1 = "router.link.v1"

// Config is the typed view of router.link.v1 JSON. Field tags match the
// schema property names. Strings rather than time.Duration are used for
// duration fields so they round-trip cleanly into the existing
// channel.LoadOptions parser, which already understands "30s" / "100ms".
type Config struct {
	Listeners              []ListenerConfig  `json:"listeners,omitempty"`
	Dialers                []DialerConfig    `json:"dialers,omitempty"`
	Heartbeats             *HeartbeatsConfig `json:"heartbeats,omitempty"`
	PayloadSenderQueueSize int               `json:"payloadSenderQueueSize,omitempty"`
	AckSenderQueueSize     int               `json:"ackSenderQueueSize,omitempty"`
	// GcMode is the auto-GC policy for stale links: "preserve" (default,
	// never act), "orphaned" (close links whose supporting
	// listener/dialer is entirely gone), or "changed" (close links
	// whose details have shifted). Empty string is treated as
	// "preserve".
	GcMode string `json:"gcMode,omitempty"`
}

// GcMode names the auto-GC policy applied by the router after each
// successful link-config Apply. Mirrors the CLI `--mode` for
// `ziti ops verify stale-links`, plus a `Preserve` value that means
// "never act."
type GcMode int

const (
	GcModePreserve GcMode = iota
	GcModeOrphaned
	GcModeChanged
)

func (m GcMode) String() string {
	switch m {
	case GcModeOrphaned:
		return "orphaned"
	case GcModeChanged:
		return "changed"
	default:
		return "preserve"
	}
}

// ParseGcMode normalizes the string form (as it appears in the JSON
// config or local YAML) into the enum. Unknown values return
// GcModePreserve and an error; the caller decides whether to fall back
// or reject the config.
func ParseGcMode(s string) (GcMode, error) {
	switch s {
	case "", "preserve":
		return GcModePreserve, nil
	case "orphaned":
		return GcModeOrphaned, nil
	case "changed":
		return GcModeChanged, nil
	default:
		return GcModePreserve, fmt.Errorf("unknown gcMode %q (expected preserve|orphaned|changed)", s)
	}
}

// ListenerConfig matches the schema's listener entry. Bind is the only
// required field.
type ListenerConfig struct {
	Binding       string          `json:"binding,omitempty"`
	Bind          string          `json:"bind"`
	Advertise     string          `json:"advertise,omitempty"`
	BindInterface string          `json:"bindInterface,omitempty"`
	Groups        Groups          `json:"groups,omitempty"`
	Options       *ChannelOptions `json:"options,omitempty"`
}

// DialerConfig matches the schema's dialer entry.
type DialerConfig struct {
	Binding               string          `json:"binding,omitempty"`
	MaxDefaultConnections int             `json:"maxDefaultConnections,omitempty"`
	MaxAckConnections     int             `json:"maxAckConnections,omitempty"`
	StartupDelay          string          `json:"startupDelay,omitempty"`
	BindInterface         string          `json:"bindInterface,omitempty"`
	Groups                Groups          `json:"groups,omitempty"`
	HealthyDialBackoff    *BackoffConfig  `json:"healthyDialBackoff,omitempty"`
	UnhealthyDialBackoff  *BackoffConfig  `json:"unhealthyDialBackoff,omitempty"`
	Options               *ChannelOptions `json:"options,omitempty"`
}

// ChannelOptions matches the shared channelOptions definition.
type ChannelOptions struct {
	OutQueueSize           int    `json:"outQueueSize,omitempty"`
	MaxQueuedConnects      int    `json:"maxQueuedConnects,omitempty"`
	MaxOutstandingConnects int    `json:"maxOutstandingConnects,omitempty"`
	ConnectTimeout         string `json:"connectTimeout,omitempty"`
	WriteTimeout           string `json:"writeTimeout,omitempty"`
}

// BackoffConfig matches the shared backoff definition.
type BackoffConfig struct {
	RetryBackoffFactor float64 `json:"retryBackoffFactor,omitempty"`
	MinRetryInterval   string  `json:"minRetryInterval,omitempty"`
	MaxRetryInterval   string  `json:"maxRetryInterval,omitempty"`
}

// HeartbeatsConfig matches the heartbeats definition.
type HeartbeatsConfig struct {
	SendInterval             string `json:"sendInterval,omitempty"`
	CheckInterval            string `json:"checkInterval,omitempty"`
	CloseUnresponsiveTimeout string `json:"closeUnresponsiveTimeout,omitempty"`
}

// Groups is a list of group names. The schema allows the JSON value to be
// either a single string or an array of strings; UnmarshalJSON normalizes
// both to a slice. MarshalJSON always emits an array for downstream
// consumers that prefer one shape.
type Groups []string

// UnmarshalJSON accepts either a string or an array of strings per schema.
func (g *Groups) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*g = Groups{single}
		return nil
	}
	var slice []string
	if err := json.Unmarshal(data, &slice); err != nil {
		return err
	}
	*g = Groups(slice)
	return nil
}

// MarshalJSON always emits the array form for stable downstream parsing.
func (g Groups) MarshalJSON() ([]byte, error) {
	if g == nil {
		return []byte("null"), nil
	}
	return json.Marshal([]string(g))
}

// ParseConfig unmarshals raw JSON into a Config. Returns an error if the
// JSON is malformed; does not enforce schema semantics (the schema is
// enforced at the controller).
func ParseConfig(data string) (*Config, error) {
	var c Config
	if err := json.Unmarshal([]byte(data), &c); err != nil {
		return nil, err
	}
	return &c, nil
}
