package intercept

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/health"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/router"
	"github.com/openziti/edge/tunnel/utils"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDialTimeout = 5 * time.Second
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

	return &hostingContext{
		service:     service,
		options:     getDefaultOptions(service, identity),
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

	halfClose := protocol != "udp"
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
				return nil, halfClose, fmt.Errorf("failed to parse port '%s': %v", s[1], e)
			}
		}

		dialer := net.Dialer{
			LocalAddr: &net.TCPAddr{IP: net.ParseIP(sourceIp), Port: sourcePort},
			Timeout:   self.dialTimeout,
		}

		conn, err = dialer.Dial(protocol, address)
	} else {
		conn, err = net.DialTimeout(protocol, address, self.dialTimeout)
	}

	return conn, halfClose, err
}

func (self *hostingContext) SetCloseCallback(f func()) {
	self.onClose = f
}

func (self *hostingContext) OnClose() {
	log := pfxlog.Logger().WithField("service", self.service.Name)
	for _, addr := range self.config.AllowedSourceAddresses {
		_, ipNet, err := utils.GetDialIP(addr)
		if err != nil {
			log.Errorf("failed to")
		}
		if self.addrTracker.RemoveAddress(ipNet.String()) {
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

func getDefaultOptions(service *entities.Service, identity *edge.CurrentIdentity) *ziti.ListenOptions {
	options := ziti.DefaultListenOptions()
	options.ManualStart = true
	options.Precedence = ziti.GetPrecedenceForLabel(identity.DefaultHostingPrecedence)
	options.Cost = identity.DefaultHostingCost

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

	return options
}
