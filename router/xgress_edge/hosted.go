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

package xgress_edge

import (
	"github.com/michaelquigley/pfxlog"
	cmap "github.com/orcaman/concurrent-map/v2"
	"sync"
)

func NewHostedServicesRegistry() *hostedServiceRegistry {
	return &hostedServiceRegistry{
		services: sync.Map{},
		ids:      cmap.New[string](),
	}
}

type hostedServiceRegistry struct {
	services sync.Map
	ids      cmap.ConcurrentMap[string, string]
}

func (registry *hostedServiceRegistry) Put(hostId string, conn *edgeTerminator) {
	registry.services.Store(hostId, conn)
}

func (registry *hostedServiceRegistry) Get(hostId string) (*edgeTerminator, bool) {
	val, ok := registry.services.Load(hostId)
	if !ok {
		return nil, false
	}
	ch, ok := val.(*edgeTerminator)
	return ch, ok
}

func (registry *hostedServiceRegistry) Delete(hostId string) {
	registry.services.Delete(hostId)
}

func (registry *hostedServiceRegistry) cleanupServices(proxy *edgeClientConn) {
	registry.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.edgeClientConn == proxy {
			terminator.close(false, "") // don't notify, channel is already closed, we can't send messages
			registry.services.Delete(key)
		}
		return true
	})
}

func (registry *hostedServiceRegistry) unbindSession(sessionToken string, proxy *edgeClientConn) bool {
	atLeastOneRemoved := false
	registry.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			terminator.close(true, "unbind successful") // don't notify, sdk asked us to unbind
			registry.services.Delete(key)
			pfxlog.Logger().WithField("routerId", terminator.edgeClientConn.listener.id.Token).
				WithField("sessionToken", sessionToken).
				WithField("terminatorId", terminator.terminatorId.Load()).
				Info("terminator removed")
			atLeastOneRemoved = true
		}
		return true
	})
	return atLeastOneRemoved
}

func (registry *hostedServiceRegistry) getRelatedTerminators(sessionToken string, proxy *edgeClientConn) []*edgeTerminator {
	var result []*edgeTerminator
	registry.services.Range(func(key, value interface{}) bool {
		terminator := value.(*edgeTerminator)
		if terminator.token == sessionToken && terminator.edgeClientConn == proxy {
			result = append(result, terminator)
		}
		return true
	})
	return result
}
