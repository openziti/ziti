/*
	Copyright NetFoundry, Inc.

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

package model

import (
	"github.com/openziti/edge/controller/persistence"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func mustParseDuration(s string) time.Duration {
	result, err := time.ParseDuration(s)

	if err != nil {
		panic(err)
	}

	return result
}

func TestPostureCheckResponseHandlers_shouldPostureCheckTimeoutBeAltered(t *testing.T) {

	t.Run("returns false if the check is null", func(t *testing.T) {
		result := shouldPostureCheckTimeoutBeAltered(nil, mustParseDuration("10m"), mustParseDuration("5m"), true, true)

		require.New(t).False(result)
	})

	t.Run("returns false if no prompts (all false), timeout remaining is greater than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          false,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("3m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, true, true)

		require.New(t).False(result)
	})

	// promptOnWake = true, wake = true
	t.Run("returns true if promptOnWake=true, wake=true, timeout remaining is greater than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          true,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("3m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, true, false)

		require.New(t).True(result)
	})

	t.Run("returns false if promptOnWake=true, wake=true, timeout remaining is less than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          true,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("6m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, true, false)

		require.New(t).False(result)
	})

	t.Run("returns false if promptOnWake=true, wake=true, timeout remaining equals grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          true,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("5m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, true, false)

		require.New(t).False(result)
	})

	// promptOnWake = true, wake = false
	t.Run("returns false if promptOnWake=true, wake=false, timeout remaining is greater than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          true,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("3m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, false)

		require.New(t).False(result)
	})

	t.Run("returns false if promptOnWake=true, wake=false, timeout remaining is less than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          true,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("6m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, false)

		require.New(t).False(result)
	})

	t.Run("returns false if promptOnWake=true, wake=false, timeout remaining equals grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnWake:          true,
			PromptOnUnlock:        false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("5m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, false)

		require.New(t).False(result)
	})

	// promptOnUnlock = true, unlock = true
	t.Run("returns true if promptOnUnlock=true, unlock=true, timeout remaining is greater than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnUnlock:        true,
			PromptOnWake:          false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("3m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, true)

		require.New(t).True(result)
	})

	t.Run("returns false if promptOnUnlock=true, unlock=true, timeout remaining is less than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnUnlock:        true,
			PromptOnWake:          false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("6m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, true)

		require.New(t).False(result)
	})

	t.Run("returns false if promptOnUnlock=true, unlock=true, timeout remaining equals grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnUnlock:        true,
			PromptOnWake:          false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("5m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, true)

		require.New(t).False(result)
	})

	// promptOnUnlock = true, unlock = false
	t.Run("returns false if promptOnUnlock=true, unlock=false, timeout remaining is greater than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnUnlock:        true,
			PromptOnWake:          false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("3m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, false)

		require.New(t).False(result)
	})

	t.Run("returns false if promptOnUnlock=true, unlock=false, timeout remaining is less than grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnUnlock:        true,
			PromptOnWake:          false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("6m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, false)

		require.New(t).False(result)
	})

	t.Run("returns false if promptOnUnlock=true, unlock=false, timeout remaining equals grace", func(t *testing.T) {
		mfaCheck := &persistence.PostureCheckMfa{
			TimeoutSeconds:        int64(mustParseDuration("10m").Seconds()),
			PromptOnUnlock:        true,
			PromptOnWake:          false,
			IgnoreLegacyEndpoints: false,
		}

		timeSinceLastMfa := mustParseDuration("5m")
		gracePeriod := mustParseDuration("5m")

		result := shouldPostureCheckTimeoutBeAltered(mfaCheck, timeSinceLastMfa, gracePeriod, false, false)

		require.New(t).False(result)
	})
}