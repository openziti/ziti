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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/router/fabric"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/openziti/edge/tunnel/intercept/host"
	"github.com/openziti/edge/tunnel/intercept/proxy"
	"github.com/openziti/edge/tunnel/intercept/tproxy"
	"github.com/openziti/fabric/router/xgress"
	cmap "github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
	"net"
)

type tunneler struct {
	dialOptions   *Options
	listenOptions *Options
	stateManager  fabric.StateManager
	bindHandler   xgress.BindHandler

	interceptor    intercept.Interceptor
	servicePoller  *servicePoller
	fabricProvider *fabricProvider
	terminators    cmap.ConcurrentMap
}

func newTunneler(factory *Factory, stateManager fabric.StateManager) *tunneler {
	result := &tunneler{
		stateManager: stateManager,
		terminators:  cmap.New(),
	}

	result.fabricProvider = newProvider(factory, result)
	result.servicePoller = newServicePoller(result.fabricProvider)

	return result
}

func (self *tunneler) Start() error {
	self.servicePoller.serviceListenerLock.Lock()
	defer self.servicePoller.serviceListenerLock.Unlock()

	var err error

	if self.listenOptions.mode == "tproxy" {
		if self.interceptor, err = tproxy.NewWithLanIf(self.listenOptions.lanIf); err != nil {
			return errors.Wrap(err, "failed to initialize tproxy interceptor")
		}
	} else if self.listenOptions.mode == "host" {
		self.interceptor = host.New()
	} else if self.listenOptions.mode == "proxy" {
		self.listenOptions.resolver = ""
		if self.interceptor, err = proxy.New(net.IPv4zero, self.listenOptions.services); err != nil {
			return errors.Wrap(err, "failed to initialize tproxy interceptor")
		}
	} else {
		return errors.Errorf("unsupported tunnel mode '%v'", self.listenOptions.mode)
	}

	resolver := dns.NewResolver(self.listenOptions.resolver)
	if err = intercept.SetDnsInterceptIpRange(self.listenOptions.dnsSvcIpRange); err != nil {
		pfxlog.Logger().Errorf("invalid dns service IP range %s: %v", self.listenOptions.dnsSvcIpRange, err)
		return err
	}

	self.servicePoller.serviceListener = intercept.NewServiceListener(self.interceptor, resolver)
	self.servicePoller.serviceListener.HandleProviderReady(self.fabricProvider)
	self.interceptor.Start(self.fabricProvider)

	go self.servicePoller.pollServices(self.listenOptions.svcPollRate)

	return nil
}

func (self *tunneler) Listen(_ string, bindHandler xgress.BindHandler) error {
	self.bindHandler = bindHandler
	return nil
}

func (self *tunneler) Close() error {
	self.interceptor.Stop()
	return nil
}
