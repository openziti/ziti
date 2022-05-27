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
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

func newServicePoller(fabricProvider *fabricProvider) *servicePoller {
	result := &servicePoller{
		services:       cmap.New[*edge.Service](),
		fabricProvider: fabricProvider,
	}

	return result
}

type servicePoller struct {
	services                cmap.ConcurrentMap[*edge.Service]
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

	var toRemove []string

	// process Deletes
	self.services.IterCb(func(k string, svc *edge.Service) {
		if _, found := idMap[svc.Id]; !found {
			toRemove = append(toRemove, k)
			self.serviceListener.HandleServicesChange(ziti.ServiceRemoved, svc)
		}
	})

	for _, key := range toRemove {
		self.services.Remove(key)
	}

	// Adds and Updates
	for _, s := range services {
		self.services.Upsert(s.Id, s, func(exist bool, valueInMap *edge.Service, newValue *edge.Service) *edge.Service {
			if !exist {
				self.serviceListener.HandleServicesChange(ziti.ServiceAdded, s)
				return s
			}
			if !reflect.DeepEqual(valueInMap, s) {
				self.serviceListener.HandleServicesChange(ziti.ServiceChanged, s)
				return s
			} else {
				logrus.WithField("service", s.Name).Debug("no change detected in service definition")
			}
			return valueInMap
		})
	}
}

// TODO: just push updates down the control channel when necessary
func (self *servicePoller) pollServices(pollInterval time.Duration, notifyClose <-chan struct{}) {
	if err := self.fabricProvider.authenticate(); err != nil {
		logrus.WithError(err).Fatal("xgress_edge_tunnel unable to authenticate to controller. " +
			"ensure tunneler mode is enabled for this router or disable tunnel listener. exiting ")
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	self.requestServiceListUpdate()

	for {
		select {
		case <-ticker.C:
			self.requestServiceListUpdate()
		case <-notifyClose:
			return
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
