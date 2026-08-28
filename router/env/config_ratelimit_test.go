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

	"github.com/openziti/ziti/v2/router/xgress_common"
	"github.com/stretchr/testify/require"
)

func loadCtrlRateLimiter(t *testing.T, src map[interface{}]interface{}) *Config {
	t.Helper()
	cfg := &Config{}
	require.NoError(t, cfg.loadCtrlRateLimiterConfig(src))
	return cfg
}

func rateLimiterStanza(entries map[interface{}]interface{}) map[interface{}]interface{} {
	return map[interface{}]interface{}{"rateLimiter": entries}
}

// The router floors minSize at 1 so the window can shrink all the way down under load. Validating
// that default against a higher floor made every rateLimiter stanza that omitted minSize fail to
// load, which takes the router down at startup rather than degrading it.
func TestCtrlRateLimiterStanzaLoadsWithoutMinSize(t *testing.T) {
	for name, entries := range map[string]map[interface{}]interface{}{
		"empty stanza":  {},
		"timeout only":  {"timeout": "45s"},
		"max size only": {"maxSize": 250},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := loadCtrlRateLimiter(t, rateLimiterStanza(entries))
			require.EqualValues(t, 1, cfg.Ctrl.RateLimit.MinSize,
				"the router's floor of 1 must survive a stanza that does not set minSize")
		})
	}
}

// Dropping the floor must not drop the bounds that still matter.
func TestCtrlRateLimiterStanzaRejectsOutOfRange(t *testing.T) {
	for name, entries := range map[string]map[interface{}]interface{}{
		"minSize below 1":       {"minSize": 0},
		"minSize above maxSize": {"minSize": 40, "maxSize": 20},
		"maxSize below floor":   {"maxSize": 2},
		"maxSize above ceiling": {"maxSize": 5000},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := &Config{}
			require.Error(t, cfg.loadCtrlRateLimiterConfig(rateLimiterStanza(entries)))
		})
	}
}

// A minSize between the router's floor and the old rejected floor is legitimate: it is strictly
// more conservative than the default the router uses when the operator says nothing.
func TestCtrlRateLimiterStanzaAcceptsSmallMinSize(t *testing.T) {
	cfg := loadCtrlRateLimiter(t, rateLimiterStanza(map[interface{}]interface{}{"minSize": 2}))

	require.EqualValues(t, 2, cfg.Ctrl.RateLimit.MinSize)
}

// The router classifies a terminator operation as congestion at EstablishmentTimeout, but the
// limiter independently expires outstanding work as a backoff at its own timeout. If the limiter's
// timeout is the smaller of the two, it resolves the work first and the router's classification is
// never the one that lands. The defaults must not put us there.
func TestCtrlRateLimiterDefaultTimeoutOutlivesEstablishment(t *testing.T) {
	cfg := loadCtrlRateLimiter(t, map[interface{}]interface{}{})

	require.GreaterOrEqual(t, cfg.Ctrl.RateLimit.Timeout, xgress_common.EstablishmentTimeout,
		"the default limiter timeout must not expire work before the router can classify it")
}

// A too-short timeout is a warning, not a rejection: the router still functions, it just stops
// being the thing that classifies slow operations.
func TestCtrlRateLimiterAcceptsTimeoutBelowEstablishment(t *testing.T) {
	cfg := loadCtrlRateLimiter(t, rateLimiterStanza(map[interface{}]interface{}{"timeout": "5s"}))

	require.Less(t, cfg.Ctrl.RateLimit.Timeout, xgress_common.EstablishmentTimeout,
		"the configured value must be applied, not clamped")
}
