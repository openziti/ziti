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
package xgress_edge_tunnel

import (
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/sirupsen/logrus"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

func newServicePoller(fabricProvider *fabricProvider) *servicePoller {
	result := &servicePoller{
		services:       cmap.New(),
		fabricProvider: fabricProvider,
	}

	return result
}

type servicePoller struct {
	services                cmap.ConcurrentMap
	serviceListener         *intercept.ServiceListener
	servicesLastUpdateToken atomic.Value
	serviceListenerLock     sync.Mutex

	fabricProvider *fabricProvider
}

func (self *servicePoller) handleServiceListUpdate(lastUpdateToken []byte, services []*edge.Service) {
	self.serviceListenerLock.Lock()
	defer self.serviceListenerLock.Unlock()

	if self.serviceListener == nil {
		logrus.Error("GOT SERVICE LIST BEFORE INITIALIZATION COMPLETE. SHOULD NOT HAPPEN!")
		return
	}

	logrus.Debugf("procesing service updates with %v services", len(services))

	self.servicesLastUpdateToken.Store(lastUpdateToken)

	idMap := make(map[string]*edge.Service)
	for _, s := range services {
		idMap[s.Id] = s
	}

	// process Deletes
	self.services.IterCb(func(k string, value interface{}) {
		svc := value.(*edge.Service)
		if _, found := idMap[svc.Id]; !found {
			self.services.Remove(k)
			self.serviceListener.HandleServicesChange(ziti.ServiceRemoved, svc)
		}
	})

	// Adds and Updates
	for _, s := range services {
		self.services.Upsert(s.Name, s, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
			if !exist {
				self.serviceListener.HandleServicesChange(ziti.ServiceAdded, s)
				return s
			}
			if !reflect.DeepEqual(valueInMap, s) {
				self.serviceListener.HandleServicesChange(ziti.ServiceChanged, s)
				return s
			}
			return valueInMap
		})
	}
}

// TODO: just push updates down the control channel when necessary
func (self *servicePoller) pollServices(pollInterval time.Duration) {
	if err := self.fabricProvider.authenticate(); err != nil {
		logrus.Panic(err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	self.requestServiceListUpdate()

	for {
		select {
		case <-ticker.C:
			self.requestServiceListUpdate()
		}
	}
}

func (self *servicePoller) requestServiceListUpdate() {
	lastUpdateTokenIntf := self.servicesLastUpdateToken.Load()
	var lastUpdateToken []byte
	if lastUpdateTokenIntf != nil {
		lastUpdateToken = lastUpdateTokenIntf.([]byte)
	}
	self.fabricProvider.requestServiceList(lastUpdateToken)
}
