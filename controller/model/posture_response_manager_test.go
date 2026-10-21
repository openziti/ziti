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

package model

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/openziti/storage/boltz"
	"github.com/openziti/ziti/controller/db"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func mustParseDuration(s string) time.Duration {
	result, err := time.ParseDuration(s)

	if err != nil {
		panic(err)
	}

	return result
}

func TestPostureCheckResponseManager_shouldPostureCheckTimeoutBeAltered(t *testing.T) {

	t.Run("returns false if the check is null", func(t *testing.T) {
		result := shouldPostureCheckTimeoutBeAltered(nil, mustParseDuration("10m"), mustParseDuration("5m"), true, true)

		require.New(t).False(result)
	})

	t.Run("returns false if no prompts (all false), timeout remaining is greater than grace", func(t *testing.T) {
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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
		mfaCheck := &db.PostureCheckMfa{
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

func TestPostureResponseManager_postureDataUpdatedWithConcurrentDbGrowth(t *testing.T) {
	ctx := NewTestContext(t)

	ctx.requireNewEdgeRouter()
	identity := ctx.requireNewIdentity(false)
	service := ctx.requireNewService()
	ctx.requireNewServicePolicy(db.PolicyTypeDialName, ss("#all"), ss("#all"))
	ctx.requireNewEdgeRouterPolicy(ss("#all"), ss("#all"))
	ctx.requireNewServiceNewEdgeRouterPolicy(ss("#all"), ss("#all"))

	for i := 0; i < 20; i++ {
		apiSession := ctx.requireNewApiSession(identity)
		ctx.requireNewSession(apiSession, service.Id, db.SessionTypeDial)
	}

	// Only the updater's own View goes through signalDb; the managers it calls keep the real db.
	signalDb := &enteredViewDb{Db: ctx.GetDb(), entered: make(chan chan struct{})}
	updaterEnv := &dbOverrideEnv{TestContext: ctx, db: signalDb}
	updater := &PostureResponseManager{env: updaterEnv, postureCache: ctx.managers.PostureResponse.postureCache}

	stop := make(chan struct{})
	updaterDone := make(chan struct{})
	go func() {
		defer close(updaterDone)
		for {
			select {
			case <-stop:
				return
			default:
				updater.postureDataUpdated(updaterEnv, identity.Id)
			}
		}
	}()

	// Each write grows the file, so its commit must remap it. Every write starts with the updater
	// held inside its transaction, and releases it just before committing.
	writesDone := make(chan error, 1)
	go func() {
		val := make([]byte, 1<<20)
		for i := 0; i < 32; i++ {
			release := <-signalDb.entered
			err := ctx.GetDb().Update(nil, func(mutateCtx boltz.MutateContext) error {
				defer close(release)
				bucket, err := mutateCtx.Tx().CreateBucketIfNotExists([]byte("growth"))
				if err != nil {
					return err
				}
				return bucket.Put([]byte(fmt.Sprintf("k%d", i)), val)
			})
			if err != nil {
				writesDone <- err
				return
			}
		}
		writesDone <- nil
	}()

	select {
	case err := <-writesDone:
		close(stop)
		<-updaterDone
		ctx.Cleanup()
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		// The db is wedged, so closing it would hang; skip cleanup.
		buf := make([]byte, 1<<20)
		t.Fatalf("db writes stalled while posture data was updating\n%s", buf[:runtime.Stack(buf, true)])
	}
}

// enteredViewDb is a boltz.Db whose View, once inside the transaction, offers a release channel on
// entered to a waiting receiver and blocks until the receiver closes it. With no receiver waiting it
// proceeds without blocking.
type enteredViewDb struct {
	boltz.Db
	entered chan chan struct{}
}

func (self *enteredViewDb) View(fn func(tx *bbolt.Tx) error) error {
	return self.Db.View(func(tx *bbolt.Tx) error {
		release := make(chan struct{})
		select {
		case self.entered <- release:
			<-release
		default:
		}
		return fn(tx)
	})
}

// dbOverrideEnv is a TestContext whose GetDb returns db. Managers reached through GetManagers still
// use the TestContext's db.
type dbOverrideEnv struct {
	*TestContext
	db boltz.Db
}

func (self *dbOverrideEnv) GetDb() boltz.Db {
	return self.db
}
