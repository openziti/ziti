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

package handler_ctrl

import (
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testAddresses = []string{"tls:127.0.0.1:6262"}

func testConfig() *config.CtrlDialerConfig {
	return &config.CtrlDialerConfig{
		MinRetryInterval:   time.Second,
		MaxRetryInterval:   5 * time.Minute,
		RetryBackoffFactor: 1.5,
		FastFailureWindow:  5 * time.Second,
	}
}

func TestDialState_DialFailed_BackoffProgression(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:  "test-router",
		addresses: testAddresses,
	}

	// first failure should set retryDelay to at least MinRetryInterval
	state.dialFailed(cfg)
	assert.Equal(t, statusNeedsDial, state.status)
	assert.GreaterOrEqual(t, state.retryDelay, cfg.MinRetryInterval)
	assert.LessOrEqual(t, state.retryDelay, cfg.MaxRetryInterval)
	assert.Equal(t, uint32(1), state.dialAttempts)

	prevDelay := state.retryDelay

	// subsequent failures should increase the delay
	for i := 0; i < 5; i++ {
		state.dialFailed(cfg)
		assert.GreaterOrEqual(t, state.retryDelay, prevDelay,
			"delay should not decrease on iteration %d", i)
		prevDelay = state.retryDelay
	}
}

func TestDialState_DialFailed_BackoffClamping(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:   "test-router",
		addresses:  testAddresses,
		retryDelay: 4 * time.Minute, // close to max
	}

	// after many failures, should never exceed MaxRetryInterval
	for i := 0; i < 20; i++ {
		state.dialFailed(cfg)
		assert.LessOrEqual(t, state.retryDelay, cfg.MaxRetryInterval,
			"delay should not exceed max on iteration %d", i)
	}
}

func TestDialState_DialFailed_MinRetryFloor(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:   "test-router",
		addresses:  testAddresses,
		retryDelay: 0, // starting from zero
	}

	state.dialFailed(cfg)
	assert.GreaterOrEqual(t, state.retryDelay, cfg.MinRetryInterval)
}

func TestDialState_DialSucceeded(t *testing.T) {
	state := &routerDialState{
		routerId:     "test-router",
		addresses:    testAddresses,
		status:       statusDialing,
		retryDelay:   30 * time.Second,
		dialAttempts: 5,
	}

	state.dialSucceeded()

	assert.Equal(t, statusConnected, state.status)
	assert.False(t, state.connectedAt.IsZero())
	// retryDelay is intentionally preserved for fast failure detection
	assert.Equal(t, 30*time.Second, state.retryDelay)
}

func TestDialState_ConnectionLost_NormalDisconnect(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:    "test-router",
		addresses:   testAddresses,
		status:      statusConnected,
		connectedAt: time.Now().Add(-time.Minute), // connected a minute ago
		retryDelay:  30 * time.Second,             // had some previous backoff
	}

	state.connectionLost(cfg)

	assert.Equal(t, statusNeedsDial, state.status)
	assert.Equal(t, time.Duration(0), state.retryDelay, "normal disconnect should reset backoff")

	// nextDial should be approximately now
	require.WithinDuration(t, time.Now(), state.nextDial, time.Second)
}

func TestDialState_ConnectionLost_FastFailure(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:    "test-router",
		addresses:   testAddresses,
		status:      statusConnected,
		connectedAt: time.Now().Add(-time.Second), // connected just 1 second ago
		retryDelay:  2 * time.Second,              // some existing backoff
	}

	state.connectionLost(cfg)

	assert.Equal(t, statusNeedsDial, state.status)
	// fast failure should apply backoff, not reset
	assert.Greater(t, state.retryDelay, time.Duration(0),
		"fast failure should apply backoff")
	assert.Greater(t, state.nextDial, time.Now(),
		"fast failure should schedule future redial")
}

func TestDialState_ConnectionLost_NotConnected(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:   "test-router",
		addresses:  testAddresses,
		status:     statusNeedsDial,
		retryDelay: 30 * time.Second,
	}

	state.connectionLost(cfg)

	// when not in connected state, should reset and retry immediately
	assert.Equal(t, statusNeedsDial, state.status)
	assert.Equal(t, time.Duration(0), state.retryDelay)
	require.WithinDuration(t, time.Now(), state.nextDial, time.Second)
}

func TestDialState_FastFailure_BackoffAccumulation(t *testing.T) {
	cfg := testConfig()
	state := &routerDialState{
		routerId:  "test-router",
		addresses: testAddresses,
	}

	// simulate repeated fast failures: connect then disconnect quickly
	for i := 0; i < 5; i++ {
		state.dialSucceeded()
		state.connectedAt = time.Now().Add(-100 * time.Millisecond) // simulate very short connection
		state.connectionLost(cfg)
	}

	// backoff should have accumulated
	assert.Greater(t, state.retryDelay, cfg.MinRetryInterval,
		"repeated fast failures should accumulate backoff")
}
