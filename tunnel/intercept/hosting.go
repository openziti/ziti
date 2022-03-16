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
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/router"
	"github.com/openziti/edge/tunnel/utils"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"net"
	"strconv"
	"strings"
	"time"
)

type healthChecksProvider interface {
	GetPortChecks() []*health.PortCheckDefinition
	GetHttpChecks() []*health.HttpCheckDefinition
}

func createHostingContexts(service *entities.Service, identity *edge.CurrentIdentity, tracker AddressTracker) []tunnel.HostingContext {
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

func newDefaultHostingContext(identity *edge.CurrentIdentity, service *entities.Service, config *entities.HostV2Terminator, tracker AddressTracker) *hostingContext {
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

	return &hostingContext{
		service:     service,
		options:     listenOptions,
		dialTimeout: config.GetDialTimeout(5 * time.Second),
		config:      config,
		addrTracker: tracker,
	}

}

type hostingContext struct {
	service     *entities.Service
	options     *ziti.ListenOptions
	config      *entities.HostV2Terminator
	dialTimeout time.Duration
	onClose     func()
	addrTracker AddressTracker
}

func (self *hostingContext) ServiceName() string {
	return self.service.Name
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

		dialer := net.Dialer{LocalAddr: localAddr, Timeout: self.dialTimeout}
		conn, err = dialer.Dial(protocol, address)
	} else {
		conn, err = net.DialTimeout(protocol, address, self.dialTimeout)
	}

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
			err = router.RemoveLocalAddress(ipNet, "lo")
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

func getDefaultOptions(service *entities.Service, identity *edge.CurrentIdentity, config *entities.HostV2Terminator) (*ziti.ListenOptions, error) {
	options := ziti.DefaultListenOptions()
	options.ManualStart = true
	options.Precedence = ziti.GetPrecedenceForLabel(identity.DefaultHostingPrecedence)
	options.Cost = identity.DefaultHostingCost

	if config.ListenOptions != nil {
		if config.ListenOptions.Cost != nil {
			options.Cost = *config.ListenOptions.Cost
		}
		if config.ListenOptions.Precedence != nil {
			options.Precedence = ziti.GetPrecedenceForLabel(*config.ListenOptions.Precedence)
		}
	}

	if val, ok := identity.ServiceHostingPrecedences[service.Id]; ok {
		if strVal, ok := val.(string); ok {
			options.Precedence = ziti.GetPrecedenceForLabel(strVal)
		}
	}

	if val, ok := identity.ServiceHostingCosts[service.Id]; ok {
		if floatVal, ok := val.(float64); ok {
			options.Cost = uint16(floatVal)
		}
	}

	if config.ListenOptions != nil {
		if config.ListenOptions.BindUsingEdgeIdentity {
			options.Identity = identity.Name
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
