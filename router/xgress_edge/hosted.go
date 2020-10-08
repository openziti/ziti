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

package xgress_edge

import (
	"sync"
)

type hostedServiceRegistry struct {
	services sync.Map
}

func (registry *hostedServiceRegistry) Put(hostId string, conn *localListener) {
	registry.services.Store(hostId, conn)
}

func (registry *hostedServiceRegistry) Get(hostId string) (*localListener, bool) {
	val, ok := registry.services.Load(hostId)
	if !ok {
		return nil, false
	}
	ch, ok := val.(*localListener)
	return ch, ok
}

func (registry *hostedServiceRegistry) Delete(hostId string) {
	registry.services.Delete(hostId)
}

func (registry *hostedServiceRegistry) cleanupServices(proxy *ingressProxy) (listeners []*localListener) {
	registry.services.Range(func(key, value interface{}) bool {
		listener := value.(*localListener)
		if listener.parent == proxy {
			listener.close(true, "underlying channel closing")
			registry.services.Delete(key)
			listeners = append(listeners, listener)
		}
		return true
	})
	return
}
