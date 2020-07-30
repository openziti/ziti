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

package handler_ctrl

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/forwarder"
	"sync"
)

type handlerPool struct {
	options   forwarder.WorkerPoolOptions
	startOnce sync.Once
	queue     chan func()
}

func (pool *handlerPool) Start() {
	pool.startOnce.Do(func() {
		pool.queue = make(chan func(), pool.options.QueueLength)
		for i := uint16(0); i < pool.options.WorkerCount; i++ {
			go func() {
				for handler := range pool.queue {
					if handler != nil {
						func() {
							defer func() {
								if err := recover(); err != nil {
									pfxlog.Logger().Errorf("worker error: %v", err)
								}
							}()
							handler()
						}()
					}
				}
			}()
		}
	})
}

func (pool *handlerPool) Queue(handler func()) {
	pool.queue <- handler
}
