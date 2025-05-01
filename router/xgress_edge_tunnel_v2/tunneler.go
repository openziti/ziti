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
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/intercept"
	"github.com/openziti/ziti/tunnel/intercept/host"
	"github.com/openziti/ziti/tunnel/intercept/proxy"
	"github.com/openziti/ziti/tunnel/intercept/tproxy"
	"github.com/pkg/errors"
	"net"
	"strings"
	"sync/atomic"
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
	hostedServices  *hostedServiceRegistry
	notifyReconnect chan struct{}

	createTime  time.Time
	initialized atomic.Bool
}

func newTunneler(factory *Factory) *tunneler {
	result := &tunneler{
		env:             factory.env,
		hostedServices:  factory.hostedServices,
		notifyReconnect: make(chan struct{}, 1),
		createTime:      time.Now(),
	}

	result.fabricProvider = newProvider(factory, result)

	return result
}

func (self *tunneler) Start() error {
	defer self.initialized.Store(true)

	var err error
	var resolver dns.Resolver

	log := pfxlog.Logger()
	if strings.HasPrefix(self.listenOptions.mode, "tproxy") {
		log.WithField("mode", self.listenOptions.mode).Info("creating interceptor")
		resolver, err = dns.NewResolver(self.listenOptions.resolver)
		if err != nil {
			pfxlog.Logger().WithError(err).Error("failed to start DNS resolver. using dummy resolver")
			resolver = dns.NewDummyResolver()
		}

		if err = intercept.SetDnsInterceptIpRange(self.listenOptions.dnsSvcIpRange); err != nil {
			pfxlog.Logger().Errorf("invalid dns service IP range %s: %v", self.listenOptions.dnsSvcIpRange, err)
			return err
		}

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

	if err = self.env.GetRouterDataModel().SubscribeToIdentityChanges(self.env.GetRouterId().Token, self, true); err != nil {
		return err
	}

	return nil
}

func (self *tunneler) WaitForInitialized() {
	if self.initialized.Load() {
		return
	}

	for !self.initialized.Load() && time.Since(self.createTime) < (2*time.Minute) {
		time.Sleep(100 * time.Millisecond)
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

func (self *tunneler) NotifyIdentityEvent(state *common.IdentityState, eventType common.IdentityEventType) {
	if eventType == common.EventIdentityDeleted || state.Identity.Disabled {
		pfxlog.Logger().Infof("identity deleted or disabled %s, eventType: %s", state.Identity.Id, eventType)
		self.fabricProvider.updateIdentity(self.mapRdmIdentityToRest(state.Identity))
		self.serviceListener.Reset()
	} else if eventType == common.EventFullState || eventType == common.EventIdentityUpdated {
		pfxlog.Logger().Infof("identity updated %s, eventType: %s", state.Identity.Id, eventType)
		self.fabricProvider.updateIdentity(self.mapRdmIdentityToRest(state.Identity))
		self.serviceListener.Reset()
		for _, svc := range state.Services {
			self.NotifyServiceChange(state, svc, common.EventAccessGained)
		}
	}
}

func (self *tunneler) NotifyServiceChange(state *common.IdentityState, service *common.IdentityService, eventType common.ServiceEventType) {
	pfxlog.Logger().Infof("service changed for %s. service %s was %s", state.Identity.Name, service.Service.Name, eventType)
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
	if len(i.AppDataJson) > 0 {
		if err := json.Unmarshal(i.AppDataJson, &appData); err != nil {
			pfxlog.Logger().WithError(err).WithField("identity", id).Error("failed to unmarshal app data")
		}
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
