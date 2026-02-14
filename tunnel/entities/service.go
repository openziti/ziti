package entities

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/genext"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/tunnel"
	"github.com/openziti/ziti/v2/tunnel/health"
	"github.com/openziti/ziti/v2/tunnel/utils"
	"github.com/pkg/errors"
)

const (
	ClientConfigV1 = "ziti-tunneler-client.v1"
	ServerConfigV1 = "ziti-tunneler-server.v1"
	HostConfigV1   = "host.v1"
	HostConfigV2   = "host.v2"
	InterceptV1    = "intercept.v1"
	InterfacesV1   = "interfaces.v1"
	ProxyV1        = "proxy.v1"
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
	terminator := &HostV1Config{
		Protocol:   self.Protocol,
		Address:    self.Hostname,
		Port:       self.Port,
		PortChecks: self.PortChecks,
		HttpChecks: self.HttpChecks,
	}

	return &HostV2Config{
		Terminators: []*HostV1Config{
			terminator,
		},
	}
}

type HostV1ListenOptions struct {
	BindUsingEdgeIdentity         bool
	BindUsingEdgeIdentityWildcard bool
	ConnectTimeoutSeconds         *int
	ConnectTimeout                *time.Duration
	Cost                          *uint16
	Identity                      string
	MaxConnections                int
	Precedence                    *string
}

type AddressTranslation struct {
	From         string
	To           string
	PrefixLength uint8
}

type allowConfig struct {
	AllowedProtocols  []string
	AllowedAddresses  []string
	AllowedPortRanges []tunnel.PortRange
}

func (self *allowConfig) GetAllowedProtocols() []string {
	return self.AllowedProtocols
}

func (self *allowConfig) GetAllowedPortRanges() []tunnel.PortRange {
	return self.AllowedPortRanges
}

func (self *allowConfig) GetAllowedAddresses() []string {
	return self.AllowedAddresses
}

type HostV1Config struct {
	Protocol                   string
	ForwardProtocol            bool
	AllowedProtocols           []string
	Address                    string
	ForwardAddress             bool
	ForwardAddressTranslations []AddressTranslation
	AllowedAddresses           []string
	Port                       int
	ForwardPort                bool
	AllowedPortRanges          []*PortRange
	AllowedSourceAddresses     []string

	PortChecks []*health.PortCheckDefinition
	HttpChecks []*health.HttpCheckDefinition

	ListenOptions *HostV1ListenOptions
	Proxy         *ProxyConfiguration

	allowedAddrs []allowedAddress
}

func (self *HostV1Config) GetAllowConfig() tunnel.AllowConfig {
	var result []tunnel.PortRange
	for _, r := range self.AllowedPortRanges {
		result = append(result, tunnel.PortRange{
			Low:  r.Low,
			High: r.High,
		})
	}
	return &allowConfig{
		AllowedProtocols:  self.AllowedProtocols,
		AllowedAddresses:  self.AllowedAddresses,
		AllowedPortRanges: result,
	}
}

func (self *HostV1Config) ToHostV2Config() *HostV2Config {
	return &HostV2Config{
		Terminators: []*HostV1Config{
			self,
		},
	}
}

type HostV2ListenOptions struct {
	BindUsingEdgeIdentity         bool
	BindUsingEdgeIdentityWildcard bool
	ConnectTimeoutSeconds         *int
	ConnectTimeout                *time.Duration
	Cost                          *uint16
	Identity                      string
	MaxConnections                int
	Precedence                    *string
	Proxy                         *transport.ProxyConfiguration
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
	return ok && (self.domain == "*" || (strings.HasSuffix(host, self.domain[1:]) || host == self.domain[2:]))
}

func makeAllowedAddress(addr string) (allowedAddress, error) {
	if addr[0] == '*' {
		if len(addr) != 1 && (len(addr) < 3 || addr[1] != '.') {
			return nil, errors.Errorf("invalid domain[%s]", addr)
		}
		return &domainAddress{domain: strings.ToLower(addr)}, nil
	}

	if cidr, err := utils.GetCidr(addr); err == nil {
		return &cidrAddress{cidr: *cidr}, nil
	}

	return &hostnameAddress{hostname: strings.ToLower(addr)}, nil
}

type ProxyConfiguration struct {
	Address string
	Type    string
}

func (self *HostV1Config) GetDialTimeout(defaultTimeout time.Duration) time.Duration {
	if self.ListenOptions != nil {
		if self.ListenOptions.ConnectTimeout != nil {
			return *self.ListenOptions.ConnectTimeout
		}
		if self.ListenOptions.ConnectTimeoutSeconds != nil {
			return time.Second * time.Duration(*self.ListenOptions.ConnectTimeoutSeconds)
		}
	}
	return defaultTimeout
}

func (self *HostV1Config) GetAllowedAddresses() []allowedAddress {
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

func (self *HostV1Config) GetPortChecks() []*health.PortCheckDefinition {
	return self.PortChecks
}

func (self *HostV1Config) GetHttpChecks() []*health.HttpCheckDefinition {
	return self.HttpChecks
}

func (self *HostV1Config) getValue(options map[string]interface{}, key string) (string, error) {
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

func (self *HostV1Config) GetProtocol(options map[string]interface{}) (string, error) {
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

func (self *HostV1Config) GetAddress(options map[string]interface{}) (string, error) {
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

func (self *HostV1Config) GetPort(options map[string]interface{}) (string, error) {
	if self.ForwardPort {
		portStr, err := self.getValue(options, tunnel.DestinationPortKey)
		if err != nil {
			return portStr, err
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return "", errors.Wrapf(err, "invalid destination port %v", portStr)
		}
		for _, portRange := range self.AllowedPortRanges {
			if uint16(port) >= portRange.Low && uint16(port) <= portRange.High {
				return portStr, nil
			}
		}
		return "", errors.Errorf("port %d is not in allowed port ranges", port)
	}
	return strconv.Itoa(self.Port), nil
}

func (self *HostV1Config) GetAllowedSourceAddressRoutes() ([]*net.IPNet, error) {
	var routes []*net.IPNet
	for _, addr := range self.AllowedSourceAddresses {
		// need to get CIDR from address - iputils.getInterceptIp?
		ipNet, err := utils.GetCidr(addr)
		if err != nil {
			return nil, errors.Errorf("failed to parse allowed source address '%s': %v", addr, err)
		}
		routes = append(routes, ipNet)
	}
	return routes, nil
}

type HostV2Config struct {
	Terminators []*HostV1Config
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
	Addresses              []string
	PortRanges             []*PortRange
	Protocols              []string
	SourceIp               *string
	DialOptions            *DialOptions
	AllowedSourceAddresses []string // white list for source IPs/CIDRs that will be intercepted
}

type TemplateFunc func(sourceAddr net.Addr, destAddr net.Addr) string

type Service struct {
	FabricProvider tunnel.FabricProvider
	rest_model.ServiceDetail
	InterceptV1Config *InterceptV1Config
	DialTimeout       time.Duration

	HostV2Config         *HostV2Config
	DialIdentityProvider TemplateFunc
	SourceAddrProvider   TemplateFunc
	cleanupActions       []func()
	lock                 sync.Mutex
}

func (self *Service) GetConfigOfType(configType string, target interface{}) (bool, error) {
	configMap, found := self.Config[configType]
	if !found {
		pfxlog.Logger().Debugf("no service config of type %v defined for service %v", configType, *self.Name)
		return false, nil
	}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.StringToTimeDurationHookFunc(),
		Result:     target,
	})
	if err != nil {
		return false, fmt.Errorf("unable construct decoder (%w)", err)
	}
	if err := decoder.Decode(configMap); err != nil {
		pfxlog.Logger().WithError(err).Debugf("unable to decode service configuration of type %v defined for service %v", configType, *self.Name)
		return true, fmt.Errorf("unable to decode service config structure: %w", err)
	}
	return true, nil
}

func (self *Service) GetFabricProvider() tunnel.FabricProvider {
	return self.FabricProvider
}

func (self *Service) AddCleanupAction(f func()) {
	self.lock.Lock()
	defer self.lock.Unlock()
	self.cleanupActions = append(self.cleanupActions, f)
}

func (self *Service) RunCleanupActions() {
	self.lock.Lock()
	defer self.lock.Unlock()

	for _, action := range self.cleanupActions {
		action()
	}

	self.cleanupActions = nil
}

func (self *Service) GetSourceAddr(sourceAddr net.Addr, destAddr net.Addr) string {
	if self.SourceAddrProvider == nil {
		return ""
	}
	return self.SourceAddrProvider(sourceAddr, destAddr)
}

func (self *Service) GetName() string {
	return *self.Name
}

func (self *Service) GetId() string {
	return *self.ID
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

func (self *Service) IsEncryptionRequired() bool {
	return genext.OrDefault(self.EncryptionRequired)
}

type InterfacesV1Config struct {
	Interfaces []string `json:"interfaces"`
}

type ProxyV1Config struct {
	Port      uint16   `json:"port"`
	Protocols []string `json:"protocols"`
	Binding   string   `json:"binding"`
}
