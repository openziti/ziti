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

package metrics

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/foundation/util/goroutines"
	"time"
)

func ConfigureGoroutinesPoolMetrics(registry metrics.Registry, poolType string) func(config *goroutines.PoolConfig) {
	return func(config *goroutines.PoolConfig) {
		config.OnCreate = func(pool goroutines.Pool) {
			registry.FuncGauge(poolType+".queue_size", func() int64 {
				return int64(pool.GetQueueSize())
			})

			registry.FuncGauge(poolType+".worker_count", func() int64 {
				return int64(pool.GetWorkerCount())
			})

			registry.FuncGauge(poolType+".busy_workers", func() int64 {
				return int64(pool.GetBusyWorkers())
			})
			pfxlog.Logger().
				WithField("poolType", poolType).
				WithField("minWorkers", config.MinWorkers).
				WithField("maxWorkers", config.MaxWorkers).
				WithField("idleTime", config.IdleTime).
				WithField("maxQueueSize", config.QueueSize).
				Info("starting goroutine pool")
		}

		timer := registry.Timer(poolType + ".work_timer")
		config.OnWorkCallback = func(workTime time.Duration) {
			timer.Update(workTime)
		}
	}
}
