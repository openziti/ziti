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

package boltz

import (
	"sync"
	"sync/atomic"
)

type EntityChangeHandler func(ctx MutateContext, entityId string) error

type EntityChangeHandlers struct {
	atomic.Value
	sync.Mutex
}

func (handlers *EntityChangeHandlers) Add(handler EntityChangeHandler) {
	handlers.Lock()
	defer handlers.Unlock()
	current := handlers.Get()

	updated := make([]EntityChangeHandler, len(current)+1)
	copy(updated, current)
	updated[len(updated)-1] = handler
	handlers.Store(updated)
}

// Figure out Remove (by adding comparable key) if/when we actually need it

func (handlers *EntityChangeHandlers) Get() []EntityChangeHandler {
	result := handlers.Load()
	if result == nil {
		return nil
	}
	return result.([]EntityChangeHandler)
}

func (handlers *EntityChangeHandlers) Handle(ctx MutateContext, entityId string) error {
	for _, callback := range handlers.Get() {
		err := callback(ctx, entityId)
		if err != nil {
			return err
		}
	}
	return nil
}
