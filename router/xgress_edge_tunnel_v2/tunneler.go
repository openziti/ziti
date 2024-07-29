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

package xgress_edge_tunnel_v2

import (
	"encoding/json"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	routerEnv "github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/intercept"
	"github.com/openziti/ziti/tunnel/intercept/host"
	"github.com/openziti/ziti/tunnel/intercept/proxy"
	"github.com/openziti/ziti/tunnel/intercept/tproxy"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"math"
	"net"
	"strings"
	"time"
)

type tunneler struct {
	env           routerEnv.RouterEnv
	dialOptions   *Options
	listenOptions *Options
	bindHandler   xgress.BindHandler

	interceptor     intercept.Interceptor
	serviceListener *intercept.ServiceListener
	fabricProvider  *fabricProvider
	terminators     cmap.ConcurrentMap[string, *tunnelTerminator]
	notifyReconnect chan struct{}
}

func newTunneler(factory *Factory) *tunneler {
	result := &tunneler{
		env:             factory.env,
		terminators:     cmap.New[*tunnelTerminator](),
		notifyReconnect: make(chan struct{}, 1),
	}

	result.fabricProvider = newProvider(factory, result)

	go result.ReestablishmentRunner()

	return result
}

func (self *tunneler) Start(notifyClose <-chan struct{}) error {
	var err error

	log := pfxlog.Logger()
	log.WithField("mode", self.listenOptions.mode).Info("creating interceptor")

	resolver, err := dns.NewResolver(self.listenOptions.resolver)
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to start DNS resolver. using dummy resolver")
		resolver = dns.NewDummyResolver()
	}

	if err = intercept.SetDnsInterceptIpRange(self.listenOptions.dnsSvcIpRange); err != nil {
		pfxlog.Logger().Errorf("invalid dns service IP range %s: %v", self.listenOptions.dnsSvcIpRange, err)
		return err
	}

	if strings.HasPrefix(self.listenOptions.mode, "tproxy") {
		tproxyConfig := tproxy.Config{
			LanIf:            self.listenOptions.lanIf,
			UDPIdleTimeout:   self.listenOptions.udpIdleTimeout,
			UDPCheckInterval: self.listenOptions.udpCheckInterval,
		}

		if strings.HasPrefix(self.listenOptions.mode, "tproxy:") {
			tproxyConfig.Diverter = strings.TrimPrefix(self.listenOptions.mode, "tproxy:")
		}

		if self.interceptor, err = tproxy.New(tproxyConfig); err != nil {
			return errors.Wrap(err, "failed to initialize tproxy interceptor")
		}
	} else if self.listenOptions.mode == "host" {
		self.listenOptions.resolver = ""
		self.interceptor = host.New()
	} else if self.listenOptions.mode == "proxy" {
		self.listenOptions.resolver = ""
		if self.interceptor, err = proxy.New(net.IPv4zero, self.listenOptions.services); err != nil {
			return errors.Wrap(err, "failed to initialize tproxy interceptor")
		}
	} else {
		return errors.Errorf("unsupported tunnel mode '%v'", self.listenOptions.mode)
	}

	self.serviceListener = intercept.NewServiceListener(self.interceptor, resolver)
	self.serviceListener.HandleProviderReady(self.fabricProvider)

	go self.removeStaleConnections(notifyClose)

	if err = self.env.GetRouterDataModel().SubscribeToIdentityChanges(self.env.GetRouterId().Token, self, true); err != nil {
		return err
	}

	return nil
}

func (self *tunneler) removeStaleConnections(notifyClose <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var toRemove []string
			self.terminators.IterCb(func(key string, t *tunnelTerminator) {

				if t.closed.Load() {
					toRemove = append(toRemove, key)
				}

			})

			for _, key := range toRemove {
				if t, found := self.terminators.Get(key); found {
					self.terminators.Remove(key)
					pfxlog.Logger().Debugf("removed closed tunnel terminator %v for service %v", key, t.context.ServiceName())
				}
			}
		case <-notifyClose:
			return
		}
	}
}

func (self *tunneler) Listen(_ string, bindHandler xgress.BindHandler) error {
	self.bindHandler = bindHandler
	return nil
}

func (self *tunneler) Close() error {
	self.interceptor.Stop()
	return nil
}

func (self *tunneler) HandleReconnect() {
	terminators := self.terminators.Items()
	for _, terminator := range terminators {
		terminator.created.Store(false)
	}

	select {
	case self.notifyReconnect <- struct{}{}:
	default:
	}
}

func (self *tunneler) ReestablishmentRunner() {
	for {
		<-self.notifyReconnect
		self.ReestablishTerminators()
	}
}

func (self *tunneler) ReestablishTerminators() {
	log := pfxlog.Logger()
	terminators := self.terminators.Items()

	time.Sleep(10 * time.Second) // wait for validate terminator messages to come in first

	if len(terminators) > 0 {
		pfxlog.Logger().Debugf("reestablishing %v terminators", len(terminators))
	}

	count := 1
	for _, terminator := range terminators {
		t := terminator
		if !terminator.created.Load() && !terminator.closed.Load() {
			err := self.fabricProvider.factory.env.GetRateLimiterPool().QueueWithTimeout(func() {
				self.fabricProvider.establishTerminatorWithRetry(t)
			}, math.MaxInt64)

			if err != nil {
				// should only happen if router is shutting down
				log.WithField("terminatorId", terminator.id).WithError(err).
					Error("unable to queue terminator reestablishment")
			}
		} else if terminator.created.Load() {
			log.Infof("terminator %v already verified", terminator.id)
		} else if terminator.closed.Load() {
			log.Infof("terminator %v closed, can't reestablish", terminator.id)
		}
		count++
	}

	if len(terminators) > 0 {
		log.Debug("finished queueing terminator reestablishment")
	}
}

func (self *tunneler) NotifyIdentityEvent(state *common.IdentityState, eventType common.IdentityEventType) {
	if eventType == common.EventIdentityDeleted || state.Identity.Disabled {
		self.fabricProvider.updateIdentity(self.mapRdmIdentityToRest(state.Identity))
		self.serviceListener.Reset()
	} else if eventType == common.EventFullState || eventType == common.EventIdentityUpdated {
		self.fabricProvider.updateIdentity(self.mapRdmIdentityToRest(state.Identity))
		self.serviceListener.Reset()
		for _, svc := range state.Services {
			self.NotifyServiceChange(state, svc, common.EventAccessGained)
		}
	}
}

func (self *tunneler) NotifyServiceChange(_ *common.IdentityState, service *common.IdentityService, eventType common.ServiceEventType) {
	tunSvc := self.mapRdmServiceToRest(service)
	switch eventType {
	case common.EventAccessGained:
		self.serviceListener.HandleServicesChange(ziti.ServiceAdded, tunSvc)
	case common.EventUpdated:
		self.serviceListener.HandleServicesChange(ziti.ServiceChanged, tunSvc)
	case common.EventAccessRemoved:
		self.serviceListener.HandleServicesChange(ziti.ServiceRemoved, tunSvc)
	}
}

func (self *tunneler) mapRdmServiceToRest(svc *common.IdentityService) *rest_model.ServiceDetail {
	id := svc.Service.Id
	name := svc.Service.Name
	encyrptionRequired := svc.Service.EncryptionRequired
	result := &rest_model.ServiceDetail{
		BaseEntity: rest_model.BaseEntity{
			ID: &id,
		},
		EncryptionRequired: &encyrptionRequired,
		Name:               &name,
		Config:             map[string]map[string]interface{}{},
	}

	for cfgId, cfg := range svc.Configs {
		cfgData := map[string]interface{}{}
		if err := json.Unmarshal([]byte(cfg.Config.DataJson), &cfgData); err != nil {
			pfxlog.Logger().WithError(err).WithField("configId", cfgId).Error("failed to unmarshal config data")
		} else {
			result.Configs = append(result.Configs, cfgId)
			result.Config[cfg.ConfigType.Name] = cfgData
		}
	}

	if svc.DialAllowed {
		result.Permissions = append(result.Permissions, rest_model.DialBindDial)
	}

	if svc.BindAllowed {
		result.Permissions = append(result.Permissions, rest_model.DialBindBind)
	}

	return result
}

func (self *tunneler) mapRdmIdentityToRest(i *common.Identity) *rest_model.IdentityDetail {
	id := i.Id
	name := i.Name
	disabled := i.Disabled
	defaultHostingCost := int64(i.DefaultHostingCost)
	defaultHostingPrecedence := rest_model.TerminatorPrecedenceDefault

	if i.DefaultHostingPrecedence == edge_ctrl_pb.TerminatorPrecedence_Required {
		defaultHostingPrecedence = rest_model.TerminatorPrecedenceRequired
	} else if i.DefaultHostingPrecedence == edge_ctrl_pb.TerminatorPrecedence_Failed {
		defaultHostingPrecedence = rest_model.TerminatorPrecedenceFailed
	}

	appData := map[string]interface{}{}
	if err := json.Unmarshal(i.AppDataJson, &appData); err != nil {
		pfxlog.Logger().WithError(err).WithField("identity", id).Error("failed to unmarshal app data")
	}

	hostingCosts := map[string]*rest_model.TerminatorCost{}
	for k, v := range i.ServiceHostingCosts {
		cost := int64(v)
		hostingCosts[k] = (*rest_model.TerminatorCost)(&cost)
	}

	hostingPrecedences := map[string]rest_model.TerminatorPrecedence{}
	for k, v := range i.ServiceHostingPrecedences {
		hostingPrecedences[k] = rest_model.TerminatorPrecedence(v.String())
	}

	result := &rest_model.IdentityDetail{
		BaseEntity: rest_model.BaseEntity{
			ID: &id,
		},
		AppData:                   &rest_model.Tags{SubTags: appData},
		DefaultHostingCost:        (*rest_model.TerminatorCost)(&defaultHostingCost),
		DefaultHostingPrecedence:  defaultHostingPrecedence,
		Disabled:                  &disabled,
		Name:                      &name,
		ServiceHostingCosts:       hostingCosts,
		ServiceHostingPrecedences: hostingPrecedences,
	}

	return result
}
