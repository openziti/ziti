package entities

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/health"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/utils"
	"github.com/openziti/foundation/util/stringz"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ClientConfigV1 = "ziti-tunneler-client.v1"
	ServerConfigV1 = "ziti-tunneler-server.v1"
	HostConfigV1   = "host.v1"
	HostConfigV2   = "host.v2"
	InterceptV1    = "intercept.v1"
)

type ServiceConfig struct {
	Protocol   string
	Hostname   string
	Port       int
	PortChecks []*health.PortCheckDefinition
	HttpChecks []*health.HttpCheckDefinition
}

func (self *ServiceConfig) GetPortChecks() []*health.PortCheckDefinition {
	return self.PortChecks
}

func (self *ServiceConfig) GetHttpChecks() []*health.HttpCheckDefinition {
	return self.HttpChecks
}

func (s *ServiceConfig) String() string {
	return fmt.Sprintf("%v:%v:%v", s.Protocol, s.Hostname, s.Port)
}

func (self *ServiceConfig) ToInterceptV1Config() *InterceptV1Config {
	return &InterceptV1Config{
		Protocols:  []string{"tcp", "udp"},
		Addresses:  []string{self.Hostname},
		PortRanges: []*PortRange{{Low: uint16(self.Port), High: uint16(self.Port)}},
	}
}

func (self *ServiceConfig) ToHostV2Config() *HostV2Config {
	terminator := &HostV2Terminator{
		Protocol:   self.Protocol,
		Address:    self.Hostname,
		Port:       self.Port,
		PortChecks: self.PortChecks,
		HttpChecks: self.HttpChecks,
	}

	return &HostV2Config{
		Terminators: []*HostV2Terminator{
			terminator,
		},
	}
}

type HostV1ListenOptions struct {
	BindUsingEdgeIdentity bool
	ConnectTimeoutSeconds *int
	Cost                  *uint16
	Identity              string
	MaxConnections        int
	Precedence            *string
}

type HostV1Config struct {
	Protocol               string
	ForwardProtocol        bool
	AllowedProtocols       []string
	Address                string
	ForwardAddress         bool
	AllowedAddresses       []string
	Port                   int
	ForwardPort            bool
	AllowedPortRanges      []*PortRange
	AllowedSourceAddresses []string

	PortChecks []*health.PortCheckDefinition
	HttpChecks []*health.HttpCheckDefinition

	ListenOptions *HostV1ListenOptions
}

func (self *HostV1Config) ToHostV2Config() *HostV2Config {
	terminator := &HostV2Terminator{
		Protocol:               self.Protocol,
		ForwardProtocol:        self.ForwardProtocol,
		AllowedProtocols:       self.AllowedProtocols,
		Address:                self.Address,
		ForwardAddress:         self.ForwardAddress,
		AllowedAddresses:       self.AllowedAddresses,
		Port:                   self.Port,
		ForwardPort:            self.ForwardPort,
		AllowedPortRanges:      self.AllowedPortRanges,
		AllowedSourceAddresses: self.AllowedSourceAddresses,
		PortChecks:             self.PortChecks,
		HttpChecks:             self.HttpChecks,
	}

	if self.ListenOptions != nil {
		var timeout *time.Duration
		if self.ListenOptions.ConnectTimeoutSeconds != nil {
			val := time.Duration(*self.ListenOptions.ConnectTimeoutSeconds) * time.Second
			timeout = &val
		}
		terminator.ListenOptions = &HostV2ListenOptions{
			BindUsingEdgeIdentity: self.ListenOptions.BindUsingEdgeIdentity,
			ConnectTimeout:        timeout,
			Cost:                  self.ListenOptions.Cost,
			Identity:              self.ListenOptions.Identity,
			MaxConnections:        self.ListenOptions.MaxConnections,
			Precedence:            self.ListenOptions.Precedence,
		}
	}

	return &HostV2Config{
		Terminators: []*HostV2Terminator{
			terminator,
		},
	}
}

type HostV2ListenOptions struct {
	BindUsingEdgeIdentity bool
	ConnectTimeout        *time.Duration
	Cost                  *uint16
	Identity              string
	MaxConnections        int
	Precedence            *string
}

type allowedAddress interface {
	Allows(addr interface{}) bool
}

type cidrAddress struct {
	cidr net.IPNet
}

func (self *cidrAddress) Allows(addr interface{}) bool {
	if ip, ok := addr.(net.IP); ok {
		return self.cidr.Contains(ip)
	}
	return false
}

type hostnameAddress struct {
	hostname string
}

func (self *hostnameAddress) Allows(addr interface{}) bool {
	host, ok := addr.(string)
	return ok && strings.ToLower(host) == self.hostname
}

type domainAddress struct {
	domain string
}

func (self *domainAddress) Allows(addr interface{}) bool {
	host, ok := addr.(string)
	host = strings.ToLower(host)
	return ok && (strings.HasSuffix(host, self.domain[1:]) || host == self.domain[2:])
}

func makeAllowedAddress(addr string) (allowedAddress, error) {
	if addr[0] == '*' {
		if len(addr) < 3 || addr[1] != '.' {
			return nil, errors.Errorf("invalid domain[%s]", addr)
		}
		return &domainAddress{domain: strings.ToLower(addr)}, nil
	}

	if _, cidr, err := utils.GetDialIP(addr); err == nil {
		return &cidrAddress{cidr: *cidr}, nil
	}

	return &hostnameAddress{hostname: strings.ToLower(addr)}, nil
}

type HostV2Terminator struct {
	Protocol               string
	ForwardProtocol        bool
	AllowedProtocols       []string
	Address                string
	ForwardAddress         bool
	AllowedAddresses       []string
	Port                   int
	ForwardPort            bool
	AllowedPortRanges      []*PortRange
	AllowedSourceAddresses []string

	PortChecks []*health.PortCheckDefinition
	HttpChecks []*health.HttpCheckDefinition

	ListenOptions *HostV2ListenOptions

	allowedAddrs []allowedAddress
}

func (self *HostV2Terminator) GetDialTimeout(defaultTimeout time.Duration) time.Duration {
	if self.ListenOptions != nil && self.ListenOptions.ConnectTimeout != nil {
		return *self.ListenOptions.ConnectTimeout
	}
	return defaultTimeout
}

func (self *HostV2Terminator) GetAllowedAddresses() []allowedAddress {
	log := pfxlog.Logger()
	if self.allowedAddrs != nil {
		return self.allowedAddrs
	}

	for _, addrStr := range self.AllowedAddresses {
		if address, err := makeAllowedAddress(addrStr); err == nil {
			self.allowedAddrs = append(self.allowedAddrs, address)
		} else {
			log.WithError(err).Warn("failed to parse allowed address")
		}
	}

	return self.allowedAddrs
}

func (self *HostV2Terminator) GetPortChecks() []*health.PortCheckDefinition {
	return self.PortChecks
}

func (self *HostV2Terminator) GetHttpChecks() []*health.HttpCheckDefinition {
	return self.HttpChecks
}

func (self *HostV2Terminator) getValue(options map[string]interface{}, key string) (string, error) {
	val, ok := options[key]
	if !ok {
		return "", errors.Errorf("%v required but not provided", key)
	}
	result, ok := val.(string)
	if !ok {
		return "", errors.Errorf("%v required and present but not a string. val: %v, type: %v", key, val, reflect.TypeOf(val))
	}
	return result, nil
}

func (self *HostV2Terminator) GetProtocol(options map[string]interface{}) (string, error) {
	if self.ForwardProtocol {
		protocol, err := self.getValue(options, tunnel.DestinationProtocolKey)
		if err != nil {
			return protocol, err
		}
		if stringz.Contains(self.AllowedProtocols, protocol) {
			return protocol, nil
		}
		return "", errors.Errorf("protocol '%s' is not in allowed protocols", protocol)
	}
	return self.Protocol, nil
}

func (self *HostV2Terminator) GetAddress(options map[string]interface{}) (string, error) {
	if self.ForwardAddress {
		allowedAddresses := self.GetAllowedAddresses()

		address, err := self.getValue(options, tunnel.DestinationHostname)
		if err == nil {
			for _, addr := range allowedAddresses {
				if addr.Allows(address) {
					return address, nil
				}
			}
		} else {
			address, err = self.getValue(options, tunnel.DestinationIpKey)
			if err != nil {
				return address, err
			}
			ip := net.ParseIP(address)
			for _, addr := range allowedAddresses {
				if addr.Allows(ip) {
					return address, nil
				}
			}
		}
		return "", errors.Errorf("address '%s' is not in allowed addresses", address)
	}
	return self.Address, nil
}

func (self *HostV2Terminator) GetPort(options map[string]interface{}) (string, error) {
	if self.ForwardPort {
		portStr, err := self.getValue(options, tunnel.DestinationPortKey)
		if err != nil {
			return portStr, err
		}
		port, err := strconv.Atoi(portStr)
		for _, portRange := range self.AllowedPortRanges {
			if uint16(port) >= portRange.Low && uint16(port) <= portRange.High {
				return portStr, nil
			}
		}
		return "", errors.Errorf("port %d is not in allowed port ranges", port)
	}
	return strconv.Itoa(self.Port), nil
}

func (self *HostV2Terminator) GetAllowedSourceAddressRoutes() ([]*net.IPNet, error) {
	var routes []*net.IPNet
	for _, addr := range self.AllowedSourceAddresses {
		// need to get CIDR from address - iputils.getInterceptIp?
		_, ipNet, err := utils.GetDialIP(addr)
		if err != nil {
			return nil, errors.Errorf("failed to parse allowed source address '%s': %v", addr, err)
		}
		routes = append(routes, ipNet)
	}
	return routes, nil
}

type HostV2Config struct {
	Terminators []*HostV2Terminator
}

type DialOptions struct {
	ConnectTimeoutSeconds *int
	Identity              *string
}

type PortRange struct {
	Low  uint16
	High uint16
}

type InterceptV1Config struct {
	Addresses   []string
	PortRanges  []*PortRange
	Protocols   []string
	SourceIp    *string
	DialOptions *DialOptions
}

type TemplateFunc func(sourceAddr net.Addr, destAddr net.Addr) string

type Service struct {
	FabricProvider tunnel.FabricProvider
	edge.Service
	InterceptV1Config *InterceptV1Config
	DialTimeout       time.Duration

	HostV2Config         *HostV2Config
	DialIdentityProvider TemplateFunc
	SourceAddrProvider   TemplateFunc
	cleanupActions       []func()
	reconnectAction      func()
	lock                 sync.Mutex
}

func (self *Service) GetFabricProvider() tunnel.FabricProvider {
	return self.FabricProvider
}

func (self *Service) AddCleanupAction(f func()) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.cleanupActions = append(self.cleanupActions, f)
}

func (self *Service) SetReconnectAction(f func()) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.reconnectAction = f
}

func (self *Service) RunCleanupActions() {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, action := range self.cleanupActions {
		action()
	}

	self.cleanupActions = nil
	self.reconnectAction = nil
}

func (self *Service) RunReconnectAction() {
	self.lock.Lock()
	reconnectAction := self.reconnectAction
	self.lock.Unlock()
	if reconnectAction != nil {
		reconnectAction()
	}
}

func (self *Service) GetSourceAddr(sourceAddr net.Addr, destAddr net.Addr) string {
	if self.SourceAddrProvider == nil {
		return ""
	}
	return self.SourceAddrProvider(sourceAddr, destAddr)
}

func (self *Service) GetName() string {
	return self.Name
}

func (self *Service) GetDialTimeout() time.Duration {
	return self.DialTimeout
}

func (self *Service) GetDialIdentity(sourceAddr net.Addr, destAddr net.Addr) string {
	if self.DialIdentityProvider == nil {
		return ""
	}
	return self.DialIdentityProvider(sourceAddr, destAddr)
}

func (self *Service) GetSourceIpTemplate() string {
	if self.InterceptV1Config == nil {
		return ""
	}
	if self.InterceptV1Config.SourceIp == nil {
		return ""
	}
	return *self.InterceptV1Config.SourceIp
}

func (self *Service) GetDialIdentityTemplate() string {
	if self.InterceptV1Config == nil {
		return ""
	}
	if self.InterceptV1Config.DialOptions == nil {
		return ""
	}
	if self.InterceptV1Config.DialOptions.Identity == nil {
		return ""
	}
	return *self.InterceptV1Config.DialOptions.Identity
}
