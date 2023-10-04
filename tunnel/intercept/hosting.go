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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/proxies"
	"github.com/openziti/ziti/tunnel"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/health"
	"github.com/openziti/ziti/tunnel/router"
	"github.com/openziti/ziti/tunnel/utils"
	"github.com/pkg/errors"
	"golang.org/x/net/proxy"
	"net"
	"strconv"
	"strings"
	"time"
)

type healthChecksProvider interface {
	GetPortChecks() []*health.PortCheckDefinition
	GetHttpChecks() []*health.HttpCheckDefinition
}

func createHostingContexts(service *entities.Service, identity *rest_model.IdentityDetail, tracker AddressTracker) []tunnel.HostingContext {
	var result []tunnel.HostingContext
	for _, t := range service.HostV2Config.Terminators {
		context := newDefaultHostingContext(identity, service, t, tracker)
		if context == nil {
			for _, c := range result {
				c.OnClose()
			}
			return nil
		}
		result = append(result, context)
	}
	return result
}

func newDefaultHostingContext(identity *rest_model.IdentityDetail, service *entities.Service, config *entities.HostV1Config, tracker AddressTracker) *hostingContext {
	log := pfxlog.Logger().WithField("service", service.Name)

	if config.ForwardProtocol && len(config.AllowedProtocols) < 1 {
		log.Error("configuration specifies 'ForwardProtocol` with zero-lengh 'AllowedProtocols'")
		return nil
	}
	if config.ForwardAddress && len(config.AllowedAddresses) < 1 {
		log.Error("configuration specifies 'ForwardAddress` with zero-lengh 'AllowedAddresses'")
		return nil
	}
	if config.ForwardPort && len(config.AllowedPortRanges) < 1 {
		log.Error("configuration specifies 'ForwardPort` with zero-lengh 'AllowedPortRanges'")
		return nil
	}

	// establish routes for allowedSourceAddresses
	routes, err := config.GetAllowedSourceAddressRoutes()
	if err != nil {
		log.Errorf("failed adding routes for allowed source addresses: %v", err)
		return nil
	}

	for _, ipNet := range routes {
		log.Infof("adding local route for allowed source address '%s'", ipNet.String())
		err = router.AddLocalAddress(ipNet, "lo")
		if err != nil {
			log.Errorf("failed to add local route for allowed source address '%s': %v", ipNet.String(), err)
			return nil
		}
		tracker.AddAddress(ipNet.String())
	}

	listenOptions, err := getDefaultOptions(service, identity, config)
	if err != nil {
		log.WithError(err).Error("failed to setup options")
		return nil
	}

	var proxyConf *transport.ProxyConfiguration
	if config.Proxy != nil {
		proxyConf = &transport.ProxyConfiguration{
			Address: config.Proxy.Address,
			Type:    transport.ProxyType(config.Proxy.Type),
		}
	}

	return &hostingContext{
		service:     service,
		options:     listenOptions,
		proxyConf:   proxyConf,
		dialTimeout: config.GetDialTimeout(5 * time.Second),
		config:      config,
		addrTracker: tracker,
	}

}

type hostingContext struct {
	service     *entities.Service
	options     *ziti.ListenOptions
	proxyConf   *transport.ProxyConfiguration
	config      *entities.HostV1Config
	dialTimeout time.Duration
	onClose     func()
	addrTracker AddressTracker
}

func (self *hostingContext) ServiceName() string {
	return *self.service.Name
}

func (self *hostingContext) ListenOptions() *ziti.ListenOptions {
	return self.options
}

func (self *hostingContext) dialAddress(options map[string]interface{}, protocol string, address string) (net.Conn, bool, error) {
	var sourceAddr string
	if val, ok := options[tunnel.SourceAddrKey]; ok {
		sourceAddr = val.(string)
	}

	isUdp := protocol == "udp"
	isTcp := protocol == "tcp"
	enableHalfClose := isTcp
	var conn net.Conn
	var err error

	var dialer proxy.Dialer

	if sourceAddr != "" {
		sourceIp := sourceAddr
		sourcePort := 0
		s := strings.Split(sourceAddr, ":")
		if len(s) == 2 {
			var e error
			sourceIp = s[0]
			sourcePort, e = strconv.Atoi(s[1])
			if e != nil {
				return nil, false, errors.Wrapf(err, "failed to parse port '%v'", s[1])
			}
		}

		var localAddr net.Addr

		if isUdp {
			localAddr = &net.UDPAddr{IP: net.ParseIP(sourceIp), Port: sourcePort}
		} else if isTcp {
			localAddr = &net.TCPAddr{IP: net.ParseIP(sourceIp), Port: sourcePort}
		} else {
			return nil, false, errors.Errorf("unsupported protocol for source address '%v'", protocol)
		}

		dialer = &net.Dialer{LocalAddr: localAddr, Timeout: self.dialTimeout}
	} else {
		dialer = &net.Dialer{Timeout: self.dialTimeout}
	}

	if self.proxyConf != nil && self.proxyConf.Type != transport.ProxyTypeNone {
		if self.proxyConf.Type == transport.ProxyTypeHttpConnect {
			dialer = proxies.NewHttpConnectProxyDialer(dialer, self.proxyConf.Address, self.proxyConf.Auth, self.dialTimeout)
		} else {
			return nil, false, errors.Errorf("unsupported proxy type %s", string(self.proxyConf.Type))
		}
	}

	conn, err = dialer.Dial(protocol, address)

	return conn, enableHalfClose, err
}

func (self *hostingContext) SetCloseCallback(f func()) {
	self.onClose = f
}

func (self *hostingContext) OnClose() {
	log := pfxlog.Logger().WithField("service", self.service.Name)
	for _, addr := range self.config.AllowedSourceAddresses {
		_, ipNet, err := utils.GetDialIP(addr)
		if err != nil {
			log.WithError(err).Error("failed to get dial IP")
		} else if self.addrTracker.RemoveAddress(ipNet.String()) {
			if err = router.RemoveLocalAddress(ipNet, "lo"); err != nil {
				log.WithError(err).Error("failed to remove local address")
			}
		}
	}

	if self.onClose != nil {
		self.onClose()
	}
}

func (self *hostingContext) getHealthChecks(provider healthChecksProvider) []health.CheckDefinition {
	var checkDefinitions []health.CheckDefinition

	for _, checkDef := range provider.GetPortChecks() {
		checkDefinitions = append(checkDefinitions, checkDef)
	}

	for _, checkDef := range provider.GetHttpChecks() {
		checkDefinitions = append(checkDefinitions, checkDef)
	}

	return checkDefinitions
}

func (self *hostingContext) GetInitialHealthState() (ziti.Precedence, uint16) {
	return self.options.Precedence, self.options.Cost
}

func (self *hostingContext) GetHealthChecks() []health.CheckDefinition {
	return self.getHealthChecks(self.config)
}

func (self *hostingContext) Dial(options map[string]interface{}) (net.Conn, bool, error) {
	if connType, found := options["connType"]; found && connType == "resolver" {
		return newResolvConn(self)
	}

	protocol, err := self.config.GetProtocol(options)
	if err != nil {
		return nil, false, err
	}

	address, err := self.config.GetAddress(options)
	if err != nil {
		return nil, false, err
	}

	port, err := self.config.GetPort(options)
	if err != nil {
		return nil, false, err
	}

	return self.dialAddress(options, protocol, address+":"+port)
}

func getDefaultOptions(service *entities.Service, identity *rest_model.IdentityDetail, config *entities.HostV1Config) (*ziti.ListenOptions, error) {
	options := ziti.DefaultListenOptions()
	options.ManualStart = true
	options.Precedence = ziti.GetPrecedenceForLabel(string(identity.DefaultHostingPrecedence))
	options.Cost = uint16(*identity.DefaultHostingCost)

	if config.ListenOptions != nil {
		if config.ListenOptions.Cost != nil {
			options.Cost = *config.ListenOptions.Cost
		}
		if config.ListenOptions.Precedence != nil {
			options.Precedence = ziti.GetPrecedenceForLabel(*config.ListenOptions.Precedence)
		}
	}

	if val, ok := identity.ServiceHostingPrecedences[*service.ID]; ok {
		options.Precedence = ziti.GetPrecedenceForLabel(string(val))
	}

	if val, ok := identity.ServiceHostingCosts[*service.ID]; ok {
		options.Cost = uint16(*val)
	}

	if config.ListenOptions != nil {
		if config.ListenOptions.BindUsingEdgeIdentity {
			options.Identity = *identity.Name
		} else if config.ListenOptions.Identity != "" {
			result, err := replaceTemplatized(config.ListenOptions.Identity, identity)
			if err != nil {
				return nil, err
			}
			options.Identity = result
		}
	}

	return options, nil
}
