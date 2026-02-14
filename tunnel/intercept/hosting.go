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
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/proxies"
	"github.com/openziti/ziti/v2/tunnel"
	"github.com/openziti/ziti/v2/tunnel/entities"
	"github.com/openziti/ziti/v2/tunnel/health"
	"github.com/openziti/ziti/v2/tunnel/router"
	"github.com/openziti/ziti/v2/tunnel/utils"
	"github.com/pkg/errors"
)

type healthChecksProvider interface {
	GetPortChecks() []*health.PortCheckDefinition
	GetHttpChecks() []*health.HttpCheckDefinition
}

func createHostingContexts(service *entities.Service, identity *rest_model.IdentityDetail, tracker AddressTracker) []tunnel.HostingContext {
	var result []tunnel.HostingContext
	pfxlog.Logger().WithField("service", service.Name).WithField("terminatorCount", len(service.HostV2Config.Terminators)).Info("creating hosting contexts")
	for idx, t := range service.HostV2Config.Terminators {
		context := newDefaultHostingContext(identity, service, t, tracker, uint32(idx))
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

func newDefaultHostingContext(identity *rest_model.IdentityDetail, service *entities.Service, config *entities.HostV1Config, tracker AddressTracker, index uint32) *hostingContext {
	log := pfxlog.Logger().WithField("service", service.Name)

	if config.ForwardProtocol && len(config.AllowedProtocols) < 1 {
		log.Error("configuration specifies 'ForwardProtocol` with zero-lengh 'AllowedProtocols'")
		return nil
	}

	var addrTranslations []addrTranslation
	if config.ForwardAddress {
		if len(config.AllowedAddresses) < 1 {
			log.Error("configuration specifies 'ForwardAddress` with zero-lengh 'AllowedAddresses'")
			return nil
		}
		if len(config.ForwardAddressTranslations) > 0 {
			slices.SortFunc(config.ForwardAddressTranslations, func(a, b entities.AddressTranslation) int {
				// sort by prefix length so first matching address is the best match
				return int(b.PrefixLength) - int(a.PrefixLength)
			})

			addrTranslations = make([]addrTranslation, len(config.ForwardAddressTranslations))
			for i, cfgX := range config.ForwardAddressTranslations {
				fromAddr, err := netip.ParseAddr(cfgX.From)
				if err != nil {
					log.Errorf("failed to parse 'From' address translation '%s'", cfgX.From)
					return nil
				}
				fromPrefix := netip.PrefixFrom(fromAddr, int(cfgX.PrefixLength))
				toAddr, err := netip.ParseAddr(cfgX.To)
				if err != nil {
					log.Errorf("failed to parse 'To' address translation '%s'", cfgX.To)
					return nil
				}
				toPrefix := netip.PrefixFrom(toAddr, int(cfgX.PrefixLength))
				addrTranslations[i] = addrTranslation{
					fromPrefix: fromPrefix,
					toPrefix:   toPrefix,
				}
			}
		}
	}
	if config.ForwardPort && len(config.AllowedPortRanges) < 1 {
		log.Error("configuration specifies 'ForwardPort` with zero-length 'AllowedPortRanges'")
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
		service:          service,
		terminatorIndex:  index,
		options:          listenOptions,
		proxyConf:        proxyConf,
		dialTimeout:      config.GetDialTimeout(5 * time.Second),
		config:           config,
		addrTracker:      tracker,
		addrTranslations: addrTranslations,
	}

}

// map input IP/cidr to output IP/cidr
type addrTranslation struct {
	fromPrefix netip.Prefix
	toPrefix   netip.Prefix
}

type hostingContext struct {
	service          *entities.Service
	terminatorIndex  uint32
	options          *ziti.ListenOptions
	proxyConf        *transport.ProxyConfiguration
	config           *entities.HostV1Config
	dialTimeout      time.Duration
	onClose          func()
	addrTracker      AddressTracker
	addrTranslations []addrTranslation
	dialWrapper      tunnel.DialWrapper
}

func (self *hostingContext) GetTerminatorIdCacheKey() string {
	return fmt.Sprintf("%s.%d", *self.service.ID, self.terminatorIndex)
}

func (self *hostingContext) SetDialWrapper(dialWrapper tunnel.DialWrapper) {
	self.dialWrapper = dialWrapper
}

func (self *hostingContext) Service() tunnel.HostedService {
	return self.service
}

func (self *hostingContext) ServiceName() string {
	return *self.service.Name
}

func (self *hostingContext) ServiceId() string {
	return *self.service.ID
}

func (self *hostingContext) ListenOptions() *ziti.ListenOptions {
	return self.options
}

func (self *hostingContext) GetAllowConfig() tunnel.AllowConfig {
	return self.config.GetAllowConfig()
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

	var dialer tunnel.Dialer

	var srcAddrPort *netip.AddrPort

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
			addr := &net.UDPAddr{IP: net.ParseIP(sourceIp), Port: sourcePort}
			localAddr = addr
			addrPort := addr.AddrPort()
			srcAddrPort = &addrPort
		} else if isTcp {
			addr := &net.TCPAddr{IP: net.ParseIP(sourceIp), Port: sourcePort}
			localAddr = addr
			addrPort := addr.AddrPort()
			srcAddrPort = &addrPort
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

	if self.dialWrapper != nil {
		conn, err = self.dialWrapper.Dial(self, srcAddrPort, dialer, protocol, address)
	} else {
		conn, err = dialer.Dial(protocol, address)
	}

	return conn, enableHalfClose, err
}

func (self *hostingContext) SetCloseCallback(f func()) {
	self.onClose = f
}

func (self *hostingContext) OnClose() {
	log := pfxlog.Logger().WithField("service", self.service.Name)
	for _, addr := range self.config.AllowedSourceAddresses {
		ipNet, err := utils.GetCidr(addr)
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

func translateIP(ip netip.Addr, fromPrefix, toPrefix netip.Prefix) (netip.Addr, error) {
	if ip.BitLen() != fromPrefix.Addr().BitLen() {
		return netip.Addr{}, fmt.Errorf("from and to addresses must be the same IP version")
	}
	if fromPrefix.Bits() != toPrefix.Bits() {
		return netip.Addr{}, fmt.Errorf("from and to addresses must have the same prefix length")
	}
	if !fromPrefix.Contains(ip) {
		return netip.Addr{}, fmt.Errorf("IP %s not in 'from' subnet %s", ip.String(), fromPrefix.String())
	}

	// Convert IPs to byte slices (IPv4 or IPv6)
	var ipBytes, tgtNet []byte
	if ip.Is4() {
		ipBytes = ip.AsSlice()[:4]
		tgtNet = toPrefix.Masked().Addr().AsSlice()[:4]
	} else {
		ipBytes = ip.AsSlice()[:16]
		tgtNet = toPrefix.Masked().Addr().AsSlice()[:16]
	}

	// Calculate host bits and apply to target network
	result := make([]byte, len(ipBytes))
	for i := range ipBytes {
		maskByte := byte(0xFF)
		if i*8 < fromPrefix.Bits() {
			bitsLeft := fromPrefix.Bits() - i*8
			if bitsLeft < 8 {
				maskByte = ^(0xFF >> bitsLeft)
			}
		} else {
			maskByte = 0
		}

		hostPart := ipBytes[i] &^ maskByte
		result[i] = tgtNet[i] | hostPart
	}

	// Convert back to netip.Addr
	var finalAddr netip.Addr
	if ip.Is4() {
		var out [4]byte
		copy(out[:], result)
		finalAddr = netip.AddrFrom4(out)
	} else {
		var out [16]byte
		copy(out[:], result)
		finalAddr = netip.AddrFrom16(out)
	}

	return finalAddr, nil
}

// apply host portion of input address to "to" address of translation with best-matching "from" address. e.g.:
// translations:
// - { "from": "192.168.0.0", "to": "10.0.1.0", "prefixLen": 24 }
// - { "from": "192.168.1.4", "to": "1.2.3.4",  "prefixLen": 32 }
// 192.168.10.3 --> 10.0.1.3
// 192.168.1.4  --> 1.2.3.4
func (self *hostingContext) translateAddress(addr string) (string, error) {
	a, err := netip.ParseAddr(addr)
	if err != nil {
		return addr, nil
	}

	to := addr
	// translations have been sorted by prefix length (highest first), so the first matching "from" will be the best match.
	for _, x := range self.addrTranslations {
		if x.fromPrefix.Contains(a) {
			t, err := translateIP(a, x.fromPrefix, x.toPrefix)
			if err != nil {
				pfxlog.Logger().WithError(err).Errorf("failed to translate address %s", addr)
				continue
			}
			to = t.String()
			break
		}
	}
	// return "to" from first translation "from" that matches addr
	return to, nil
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
	xAddress, err := self.translateAddress(address)
	if err != nil {
		return nil, false, err
	}

	port, err := self.config.GetPort(options)
	if err != nil {
		return nil, false, err
	}

	return self.dialAddress(options, protocol, xAddress+":"+port)
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
		if config.ListenOptions.MaxConnections > 0 {
			options.MaxTerminators = config.ListenOptions.MaxConnections
		}
	}

	if val, ok := identity.ServiceHostingPrecedences[*service.ID]; ok {
		options.Precedence = ziti.GetPrecedenceForLabel(string(val))
	}

	if val, ok := identity.ServiceHostingCosts[*service.ID]; ok {
		options.Cost = uint16(*val)
	}

	if config.ListenOptions != nil {
		if config.ListenOptions.BindUsingEdgeIdentityWildcard {
			options.Identity = "*:" + *identity.Name
		} else if config.ListenOptions.BindUsingEdgeIdentity {
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
