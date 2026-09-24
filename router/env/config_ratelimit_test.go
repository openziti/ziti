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
	cfg := loadCtrlRateLimiter(t, map[interface{}]interface{}{
		"rateLimiter": map[interface{}]interface{}{
			"timeout": "5s",
			"minSize": 5,
		},
	})

	require.Less(t, cfg.Ctrl.RateLimit.Timeout, xgress_common.EstablishmentTimeout,
		"the configured value must be applied, not clamped")
}
