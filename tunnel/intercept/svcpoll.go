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

package intercept

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/health"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	logrus "github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
)

func NewServiceListener(interceptor Interceptor, resolver dns.Resolver) *ServiceListener {
	return &ServiceListener{
		interceptor:    interceptor,
		resolver:       resolver,
		healthCheckMgr: health.NewManager(),
		addresses:      map[string]int{},
		services:       map[string]*entities.Service{},
	}
}

type ServiceListener struct {
	provider       tunnel.FabricProvider
	interceptor    Interceptor
	resolver       dns.Resolver
	healthCheckMgr health.Manager
	addresses      map[string]int
	services       map[string]*entities.Service
	sync.Mutex
}

func (self *ServiceListener) WaitForShutdown() {
	sig := make(chan os.Signal, 1) //signal.Notify expects a buffered chan of at least 1
	signal.Notify(sig, syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM)

	for s := range sig {
		logrus.Debugf("caught signal %v", s)
		break
	}

	self.Lock()
	defer self.Unlock()

	for _, svc := range self.services {
		self.removeService(svc)
	}
	self.interceptor.Stop()
}

func (self *ServiceListener) HandleProviderReady(provider tunnel.FabricProvider) {
	self.provider = provider
}

func (self *ServiceListener) HandleServicesChange(eventType ziti.ServiceEventType, service *edge.Service) {
	tunnelerService := &entities.Service{Service: *service}

	log := logrus.WithField("service", service.Name)

	switch eventType {
	case ziti.ServiceAdded:
		log.Info("adding service")
		self.addService(tunnelerService)
	case ziti.ServiceRemoved:
		log.Info("removing service")
		self.removeService(tunnelerService)
	case ziti.ServiceChanged:
		log.Info("updating service: removing old service")
		self.removeService(tunnelerService)

		log.Info("updating service: adding new service")
		self.addService(tunnelerService)
	default:
		pfxlog.Logger().Errorf("unhandled service change event type: %v", eventType)
	}
}

func (self *ServiceListener) addService(svc *entities.Service) {
	log := pfxlog.Logger()

	if stringz.Contains(svc.Permissions, "Dial") {
		clientConfig := &entities.ServiceConfig{}
		found, err := svc.GetConfigOfType(entities.ClientConfigV1, clientConfig)

		if found && err == nil {
			svc.ClientConfig = clientConfig
		} else if !found {
			pfxlog.Logger().Debugf("no service config of type %v for service %v", entities.ClientConfigV1, svc.Name)
		} else if err != nil {
			pfxlog.Logger().WithError(err).Errorf("error decoding service config of type %v for service %v", entities.ClientConfigV1, svc.Name)
		}

		// not all interceptors need a config, specifically proxy doesn't need one
		log.Infof("starting tunnel for newly available service %s", svc.Name)
		if err := self.interceptor.Intercept(svc, self.resolver); err != nil {
			log.Errorf("failed to intercept service: %v", err)
		}
	}

	if stringz.Contains(svc.Permissions, "Bind") {
		hostV2config := &entities.HostV2Config{}
		found, err := svc.GetConfigOfType(entities.HostConfigV2, hostV2config)
		if found {
			svc.HostV2Config = hostV2config
		}

		if !found {
			hostV1config := &entities.HostV1Config{}
			if found, err = svc.GetConfigOfType(entities.HostConfigV1, hostV1config); found {
				svc.HostV2Config = hostV1config.ToHostV2Config()
			}
		}

		if !found {
			serverConfig := &entities.ServiceConfig{}
			if found, err = svc.GetConfigOfType(entities.ServerConfigV1, serverConfig); found {
				svc.ServerConfig = serverConfig
			}
		}

		if found && err == nil {
			log.Infof("Hosting newly available service %s", svc.Name)
			go self.host(svc)
		} else if !found {
			log.WithError(err).Warnf("service %v is hostable but no server config of type %v is available", svc.Name, entities.ServerConfigV1)
		} else if err != nil {
			log.WithError(err).Errorf("service %v is hostable but unable to decode server config of type %v", svc.Name, entities.ServerConfigV1)
		}
	}

	if svc.ClientConfig != nil {
		addr := svc.ClientConfig.Hostname
		self.addresses[addr] += 1
	}

	if svc.ClientConfig != nil || svc.ServerConfig != nil {
		self.services[svc.Id] = svc
	}
}

func (self *ServiceListener) removeService(svc *entities.Service) {
	log := pfxlog.Logger()

	previousService := self.services[svc.Id]
	if previousService != nil {
		if previousService.ClientConfig != nil {
			log.Infof("stopping tunnel for unavailable service: %s", previousService.Name)
			useCnt := self.addresses[previousService.ClientConfig.Hostname]
			err := self.interceptor.StopIntercepting(previousService.Name, useCnt == 1)
			if err != nil {
				log.Errorf("failed to stop intercepting: %v", err)
			}
			if useCnt == 1 {
				delete(self.addresses, previousService.ClientConfig.Hostname)
			} else {
				self.addresses[previousService.ClientConfig.Hostname] -= 1
			}
		}

		if previousService.StopHostHook != nil {
			previousService.StopHostHook()
		}

		delete(self.services, svc.Id)
	}
}

func (self *ServiceListener) host(svc *entities.Service) {
	logger := pfxlog.Logger().WithField("service", svc.Name)

	currentIdentity, err := self.provider.GetCurrentIdentity()
	if err != nil {
		logger.WithError(err).Error("error getting current identity information")
		return
	}

	hostContexts := createHostingContexts(svc, currentIdentity)

	var hostControls []tunnel.HostControl

	stopHook := func() {
		for _, hostControl := range hostControls {
			_ = hostControl.Close()
		}
		self.healthCheckMgr.UnregisterServiceChecks(svc.Id)
	}
	svc.StopHostHook = stopHook

	for idx, hostContext := range hostContexts {
		hostControl, err := self.provider.HostService(hostContext)
		if err != nil {
			logger.WithError(err).WithField("service", svc.Name).Errorf("error listening for service")
			return
		}

		context := strconv.Itoa(idx)

		hostContext.SetCloseCallback(func() {
			self.healthCheckMgr.UnregisterServiceContextChecks(svc.Name, context)
		})

		hostControls = append(hostControls, hostControl)

		precedence, cost := hostContext.GetInitialHealthState()
		serviceState := health.NewServiceStateWithContext(svc.Name, context, precedence, cost, hostControl)

		if err := self.healthCheckMgr.RegisterServiceChecks(serviceState, hostContext.GetHealthChecks()); err != nil {
			logger.WithError(err).Error("error setting up health checks")
			hostContext.OnClose()
			return
		}
	}
}
