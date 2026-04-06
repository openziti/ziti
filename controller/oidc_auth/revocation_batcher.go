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

package oidc_auth

import (
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/controller/change"
	"github.com/openziti/ziti/controller/model"
)

// RevocationBatcher collects refresh-token revocations and flushes them in
// batches through raft. All operations are best-effort: Add never blocks or
// returns an error, and flush failures are logged but not propagated. This
// avoids making the DB and raft a bottleneck on token refresh.
type RevocationBatcher struct {
	env           model.Env
	maxBatchSize  int
	maxQueued     int
	flushInterval time.Duration
	pending       []*model.Revocation
	mu            sync.Mutex
	closeNotify   <-chan struct{}
}

// NewRevocationBatcher creates a batcher that flushes pending revocations on a
// timer. It does not start the background goroutine; call Start for that.
func NewRevocationBatcher(env model.Env, cfg *Config) *RevocationBatcher {
	return &RevocationBatcher{
		env:           env,
		maxBatchSize:  cfg.RevocationBucketMaxSize,
		maxQueued:     cfg.RevocationMaxQueued,
		flushInterval: cfg.RevocationBucketInterval,
		closeNotify:   env.GetCloseNotifyChannel(),
	}
}

// Add queues a revocation for eventual batch creation. If the queue is full the
// revocation is dropped and a warning is logged. This method never blocks on
// raft or database I/O.
func (b *RevocationBatcher) Add(rev *model.Revocation) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.pending) >= b.maxQueued {
		pfxlog.Logger().WithField("tokenId", rev.Id).Warn("revocation batcher queue full, dropping revocation")
		return
	}
	b.pending = append(b.pending, rev)
}

// Start begins the background flush goroutine.
func (b *RevocationBatcher) Start() {
	go b.run()
}

func (b *RevocationBatcher) run() {
	log := pfxlog.Logger()
	timer := time.NewTimer(b.flushInterval)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			b.drainLoop()
			timer.Reset(b.flushInterval)
		case <-b.closeNotify:
			log.Info("revocation batcher shutting down, flushing remaining entries")
			b.drainLoop()
			return
		}
	}
}

// drainLoop flushes one batch at a time until fewer than maxBatchSize entries
// remain. Each batch is dispatched through raft without holding the lock.
func (b *RevocationBatcher) drainLoop() {
	for {
		batch := b.takeBatch()
		if len(batch) == 0 {
			return
		}

		b.flushBatch(batch)

		b.mu.Lock()
		remaining := len(b.pending)
		b.mu.Unlock()

		if remaining < b.maxBatchSize {
			return
		}
	}
}

// takeBatch removes up to maxBatchSize entries from the front of pending.
func (b *RevocationBatcher) takeBatch() []*model.Revocation {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.pending) == 0 {
		return nil
	}

	n := b.maxBatchSize
	if n > len(b.pending) {
		n = len(b.pending)
	}

	batch := make([]*model.Revocation, n)
	copy(batch, b.pending[:n])
	b.pending = b.pending[n:]
	return batch
}

func (b *RevocationBatcher) flushBatch(batch []*model.Revocation) {
	ctx := change.New().SetSourceType("revocation.batcher").SetChangeAuthorType(change.AuthorTypeController)
	if err := b.env.GetManagers().Revocation.CreateBatch(batch, ctx); err != nil {
		pfxlog.Logger().WithError(err).Errorf("failed to flush %d batched revocations", len(batch))
	}
}

// Flush synchronously drains all pending revocations. Exposed for testing.
func (b *RevocationBatcher) Flush() {
	b.drainLoop()
}
