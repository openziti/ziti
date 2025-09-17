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

package intercept

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/tunnel"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/health"
	"github.com/pkg/errors"
	logrus "github.com/sirupsen/logrus"
)

// variables for substitutions in intercept.v1 sourceIp property
var sourceIpVar = "$" + tunnel.SourceIpKey
var sourcePortVar = "$" + tunnel.SourcePortKey
var dstIpVar = "$" + tunnel.DestinationIpKey
var destPortVar = "$" + tunnel.DestinationPortKey
var destHostnameVar = "$" + tunnel.DestinationHostname

func NewServiceListenerGroup(interceptor Interceptor, resolver dns.Resolver) *ServiceListenerGroup {
	return &ServiceListenerGroup{
		interceptor:    interceptor,
		resolver:       resolver,
		healthCheckMgr: health.NewManager(),
		addrTracker:    addrTracker{},
	}
}

type ServiceListenerGroup struct {
	interceptor    Interceptor
	resolver       dns.Resolver
	healthCheckMgr health.Manager
	addrTracker    AddressTracker
	listener       []*ServiceListener
	sync.Mutex
}

func (self *ServiceListenerGroup) NewServiceListener() *ServiceListener {
	result := &ServiceListener{
		interceptor:    self.interceptor,
		resolver:       self.resolver,
		healthCheckMgr: self.healthCheckMgr,
		addrTracker:    self.addrTracker,
		services:       map[string]*entities.Service{},
		Mutex:          &self.Mutex,
	}
	self.listener = append(self.listener, result)
	return result
}

func (self *ServiceListenerGroup) WaitForShutdown() {
	sig := make(chan os.Signal, 1) //signal.Notify expects a buffered chan of at least 1
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for s := range sig {
		logrus.Debugf("caught signal %v", s)
		break
	}

	for _, listener := range self.listener {
		listener.stop()
	}

	self.interceptor.Stop()
}

func NewServiceListener(interceptor Interceptor, resolver dns.Resolver) *ServiceListener {
	return &ServiceListener{
		interceptor:    interceptor,
		resolver:       resolver,
		healthCheckMgr: health.NewManager(),
		addrTracker:    addrTracker{},
		services:       map[string]*entities.Service{},
		Mutex:          &sync.Mutex{},
	}
}

type ServiceListener struct {
	provider       tunnel.FabricProvider
	interceptor    Interceptor
	resolver       dns.Resolver
	healthCheckMgr health.Manager
	services       map[string]*entities.Service
	addrTracker    AddressTracker
	*sync.Mutex
}

func (self *ServiceListener) WaitForShutdown() {
	sig := make(chan os.Signal, 1) //signal.Notify expects a buffered chan of at least 1
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for s := range sig {
		logrus.Debugf("caught signal %v", s)
		break
	}

	self.stop()
	self.interceptor.Stop()
}

func (self *ServiceListener) stop() {
	self.Lock()
	defer self.Unlock()

	for _, svc := range self.services {
		self.removeService(svc)
	}
}

func (self *ServiceListener) HandleProviderReady(provider tunnel.FabricProvider) {
	self.provider = provider
}

func (self *ServiceListener) HandleServicesChange(eventType ziti.ServiceEventType, service *rest_model.ServiceDetail) {
	self.Lock()
	defer self.Unlock()

	tunnelerService := &entities.Service{
		FabricProvider: self.provider,
		ServiceDetail:  *service,
	}

	log := logrus.WithField("service", *service.Name)

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

func (self *ServiceListener) Reset() {
	self.Lock()
	defer self.Unlock()

	for _, svc := range self.services {
		self.removeService(svc)
	}
}

func (self *ServiceListener) addService(svc *entities.Service) {
	log := pfxlog.Logger().WithField("serviceId", *svc.ID).WithField("serviceName", *svc.Name)

	svc.DialTimeout = 5 * time.Second

	var perms []string

	for _, perm := range svc.Permissions {
		perms = append(perms, string(perm))
	}

	if stringz.Contains(perms, "Dial") {
		interceptV1Config := &entities.InterceptV1Config{}
		found, err := svc.GetConfigOfType(entities.InterceptV1, interceptV1Config)
		if found {
			svc.InterceptV1Config = interceptV1Config
			if interceptV1Config.DialOptions != nil && interceptV1Config.DialOptions.ConnectTimeoutSeconds != nil {
				svc.DialTimeout = time.Duration(*interceptV1Config.DialOptions.ConnectTimeoutSeconds) * time.Second
			}
		}

		if err != nil {
			logrus.WithError(err).Errorf("error decoding service config of type %v for service %v", entities.InterceptV1, *svc.Name)
		}

		if !found {
			clientConfig := &entities.ServiceConfig{}
			found, err = svc.GetConfigOfType(entities.ClientConfigV1, clientConfig)

			if found {
				svc.InterceptV1Config = clientConfig.ToInterceptV1Config()
			}

			if err != nil {
				logrus.WithError(err).Errorf("error decoding service config of type %v for service %v", entities.ClientConfigV1, *svc.Name)
			}
		}

		if err = self.configureSourceAddrProvider(svc); err != nil {
			log.WithError(err).Error("failed interpreting source ip")
		}
		if err = self.configureDialIdentityProvider(svc); err != nil {
			log.WithError(err).Error("error interpreting dialOptions.identity")
		}

		// not all interceptors need a config, specifically proxy doesn't need one
		log.Infof("checking if we can intercept newly available service %s", *svc.Name)
		if err = self.interceptor.Intercept(svc, self.resolver, self.addrTracker); err != nil {
			log.Errorf("failed to intercept service: %v", err)
		}
	}

	if stringz.Contains(perms, "Bind") {
		configType := entities.HostConfigV2
		hostV2config := &entities.HostV2Config{}
		found, err := svc.GetConfigOfType(configType, hostV2config)
		if found {
			svc.HostV2Config = hostV2config
		}

		if !found {
			configType = entities.HostConfigV1
			hostV1config := &entities.HostV1Config{}
			if found, err = svc.GetConfigOfType(configType, hostV1config); found {
				svc.HostV2Config = hostV1config.ToHostV2Config()
			}
		}

		if !found {
			configType = entities.ServerConfigV1
			serverConfig := &entities.ServiceConfig{}
			if found, err = svc.GetConfigOfType(configType, serverConfig); found {
				svc.HostV2Config = serverConfig.ToHostV2Config()
			}
		}

		if found && err == nil {
			log.Info("Hosting newly available service")
			self.host(svc, self.addrTracker)
		} else if !found {
			log.WithError(err).Warnf("service is hostable but no compatible host config found. supported types: [%v, %v, %v]",
				entities.HostConfigV2, entities.HostConfigV1, entities.ServerConfigV1)
		} else {
			log.WithError(err).Errorf("service is hostable but unable to decode server config of type %v", configType)
		}
	}

	if svc.InterceptV1Config != nil || svc.HostV2Config != nil {
		self.services[*svc.ID] = svc
	}
}

func (self *ServiceListener) removeService(svc *entities.Service) {
	log := pfxlog.Logger()

	log.Infof("stopping tunnel for unavailable service: %s", *svc.Name)
	err := self.interceptor.StopIntercepting(*svc.Name, self.addrTracker)
	if err != nil {
		log.WithError(err).Errorf("failed to stop intercepting service: %v", *svc.Name)
	}

	if previousService := self.services[*svc.ID]; previousService != nil {
		previousService.RunCleanupActions()
		delete(self.services, *svc.ID)
	}
}

func (self *ServiceListener) host(svc *entities.Service, tracker AddressTracker) {
	logger := pfxlog.Logger().WithField("service", *svc.Name)

	currentIdentity, err := self.provider.GetCurrentIdentityWithBackoff()
	if err != nil {
		logger.WithError(err).Error("error getting current identity information")
		return
	}

	hostContexts := createHostingContexts(svc, currentIdentity, tracker)

	var hostControls []tunnel.HostControl

	stopHook := func() {
		for _, hostControl := range hostControls {
			_ = hostControl.Close()
		}
		self.healthCheckMgr.UnregisterServiceChecks(*svc.ID)
	}
	svc.AddCleanupAction(stopHook)

	for idx, hostContext := range hostContexts {
		hostControl, err := self.provider.HostService(hostContext)
		if err != nil {
			logger.WithError(err).WithField("service", *svc.Name).Errorf("error listening for service")
			return
		}

		context := strconv.Itoa(idx)

		hostContext.SetCloseCallback(func() {
			self.healthCheckMgr.UnregisterServiceContextChecks(*svc.Name, context)
		})

		hostControls = append(hostControls, hostControl)

		precedence, cost := hostContext.GetInitialHealthState()
		serviceState := health.NewServiceStateWithContext(*svc.Name, context, precedence, cost, hostControl)

		if err := self.healthCheckMgr.RegisterServiceChecks(serviceState, hostContext.GetHealthChecks()); err != nil {
			logger.WithError(err).Error("error setting up health checks")
			hostContext.OnClose()
			return
		}
	}
}

func (self *ServiceListener) configureSourceAddrProvider(svc *entities.Service) error {
	var err error
	if template := svc.GetSourceIpTemplate(); template != "" {
		svc.SourceAddrProvider, err = self.getTemplatingProvider(template)
	}
	return err
}

func (self *ServiceListener) configureDialIdentityProvider(svc *entities.Service) error {
	var err error
	if template := svc.GetDialIdentityTemplate(); template != "" {
		svc.DialIdentityProvider, err = self.getTemplatingProvider(template)
	}
	return err
}

func (self *ServiceListener) getTemplatingProvider(template string) (entities.TemplateFunc, error) {
	if template == sourceIpVar+":"+sourcePortVar {
		return func(sourceAddr, _ net.Addr) string {
			return sourceAddr.String()
		}, nil
	}

	currentIdentity, err := self.provider.GetCurrentIdentity()
	if err != nil {
		return nil, err
	}

	if template, err = replaceTemplatized(template, currentIdentity); err != nil {
		return nil, err
	}

	if strings.IndexByte(template, '$') < 0 {
		return func(_, _ net.Addr) string {
			return template
		}, nil
	}

	return func(sourceAddr, destAddr net.Addr) string {
		sourceAddrIp, sourceAddrPort := tunnel.GetIpAndPort(sourceAddr)
		destAddrIp, destAddrPort := tunnel.GetIpAndPort(destAddr)
		result := strings.ReplaceAll(template, sourceIpVar, sourceAddrIp)
		result = strings.ReplaceAll(result, sourcePortVar, sourceAddrPort)
		result = strings.ReplaceAll(result, dstIpVar, destAddrIp)
		result = strings.ReplaceAll(result, destPortVar, destAddrPort)
		if self.resolver != nil {
			destHostname, err := self.resolver.Lookup(net.ParseIP(destAddrIp))
			if err == nil {
				result = strings.ReplaceAll(result, destHostnameVar, destHostname)
			}
		}
		return result
	}, nil
}

func replaceTemplatized(input string, currentIdentity *rest_model.IdentityDetail) (string, error) {
	input = strings.ReplaceAll(input, "$tunneler_id.name", *currentIdentity.Name)
	start := "$tunneler_id.appData["
	for {
		index := strings.Index(input, start)
		if index < 0 {
			return input, nil
		}
		postStr := input[index+len(start):]
		closeIdx := strings.IndexByte(postStr, ']')
		if closeIdx == -1 {
			return "", errors.New("input contains unclosed $tunneler_id.appData[")
		}
		tagName := postStr[0:closeIdx]
		logrus.Infof("looking up tagname: %v", tagName)
		tagValue := ""
		logrus.Infof("appData: %v", currentIdentity.AppData)
		if currentIdentity.AppData != nil {
			val, found := currentIdentity.AppData.SubTags[tagName]
			if found {
				tagValue = fmt.Sprintf("%v", val)
			}
		}
		logrus.Infof("value: %v", tagValue)

		fullTag := start + tagName + "]"
		input = strings.ReplaceAll(input, fullTag, tagValue)
	}
}

type AddressTracker interface {
	AddAddress(addr string)
	RemoveAddress(addr string) bool
}

type addrTracker map[string]int

func (self addrTracker) AddAddress(addr string) {
	logrus.Debugf("adding %v from address tracker: %+v", addr, self)
	useCnt := self[addr]
	self[addr] = useCnt + 1
}

func (self addrTracker) RemoveAddress(addr string) bool {
	logrus.Debugf("trying to remove %v from address tracker: %+v", addr, self)
	useCnt := self[addr]
	if useCnt <= 1 {
		delete(self, addr)
		return true
	}

	self[addr] = useCnt - 1
	return false
}
