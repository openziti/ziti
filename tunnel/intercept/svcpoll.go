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
	log "github.com/sirupsen/logrus"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
		log.Debugf("caught signal %v", s)
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

	switch eventType {
	case ziti.ServiceAdded:
		self.addService(tunnelerService)
	case ziti.ServiceRemoved:
		self.removeService(tunnelerService)
	case ziti.ServiceChanged:
		self.removeService(tunnelerService)
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

			log.Infof("starting tunnel for newly available service %s", svc.Name)
			if err := self.interceptor.Intercept(svc, self.resolver); err != nil {
				log.Errorf("failed to intercept service: %v", err)
			}
		} else if !found {
			pfxlog.Logger().Debugf("no service config of type %v for service %v", entities.ClientConfigV1, svc.Name)
		} else if err != nil {
			pfxlog.Logger().WithError(err).Errorf("error decoding service config of type %v for service %v", entities.ClientConfigV1, svc.Name)
		}
	}

	if stringz.Contains(svc.Permissions, "Bind") {
		serverConfig := &entities.ServiceConfig{}
		found, err := svc.GetConfigOfType(entities.ServerConfigV1, serverConfig)

		if found && err == nil {
			svc.ServerConfig = serverConfig
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

	options := ziti.DefaultListenOptions()
	options.ManualStart = true
	options.Precedence = ziti.GetPrecedenceForLabel(currentIdentity.DefaultHostingPrecedence)
	options.Cost = currentIdentity.DefaultHostingCost

	context := &hostingContext{
		service: svc,
		options: options,
		onClose: func() {
			self.healthCheckMgr.UnregisterServiceChecks(svc.Id)
		},
	}

	hostControl, err := self.provider.HostService(context)
	if err != nil {
		logger.WithError(err).WithField("service", svc.Name).Errorf("error listening for service")
		return
	}

	stopHook := func() {
		_ = hostControl.Close()
		self.healthCheckMgr.UnregisterServiceChecks(svc.Id)
	}

	svc.StopHostHook = stopHook

	if err := self.setupHealthChecks(hostControl, svc, currentIdentity); err != nil {
		logger.WithError(err).Error("error setting up health checks")
		stopHook()
		return
	}
}

func (self *ServiceListener) setupHealthChecks(serviceUpdater health.ServiceUpdater, service *entities.Service, identity *edge.CurrentIdentity) error {
	precedence := ziti.GetPrecedenceForLabel(identity.DefaultHostingPrecedence)
	serviceState := health.NewServiceState(service.Name, precedence, identity.DefaultHostingCost, serviceUpdater)

	var checkDefinitions []health.CheckDefinition

	for _, checkDef := range service.ServerConfig.PortChecks {
		checkDefinitions = append(checkDefinitions, checkDef)
	}

	for _, checkDef := range service.ServerConfig.HttpChecks {
		checkDefinitions = append(checkDefinitions, checkDef)
	}

	if len(checkDefinitions) > 0 {
		return self.healthCheckMgr.RegisterServiceChecks(serviceState, checkDefinitions)
	}

	return nil
}

type hostingContext struct {
	service *entities.Service
	options *ziti.ListenOptions
	onClose func()
}

func (self *hostingContext) ServiceName() string {
	return self.service.Name
}

func (self *hostingContext) ListenOptions() *ziti.ListenOptions {
	return self.options
}

func (self *hostingContext) Dial() (net.Conn, error) {
	config := self.service.ServerConfig
	return net.Dial(config.Protocol, config.Hostname+":"+strconv.Itoa(config.Port))
}

func (self *hostingContext) SupportHalfClose() bool {
	return !strings.Contains(self.service.ServerConfig.Protocol, "udp")
}

func (self *hostingContext) OnClose() {
	self.onClose()
}
