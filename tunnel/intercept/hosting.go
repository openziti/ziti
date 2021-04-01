package intercept

import (
	"fmt"
	"github.com/openziti/edge/health"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/entities"
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

type serviceConfiguration interface {
	healthChecksProvider
	SetListenOptions(options *ziti.ListenOptions)
	GetDialTimeout(defaultTimeout time.Duration) time.Duration
	GetProtocol(options map[string]interface{}) (string, error)
	GetAddress(options map[string]interface{}) (string, error)
	GetPort(options map[string]interface{}) (string, error)
}

func createHostingContexts(service *entities.Service, identity *edge.CurrentIdentity) []tunnel.HostingContext {
	if service.HostV2Config != nil {
		var result []tunnel.HostingContext
		for _, t := range service.HostV2Config.Terminators {
			context := newDefaultHostingContext(identity, service, t)
			result = append(result, context)
		}
		return result
	}

	context := &legacyHostingContext{
		baseHostingContext{
			service:     service,
			options:     getDefaultOptions(identity),
			dialTimeout: defaultDialTimeout,
		},
	}
	return []tunnel.HostingContext{context}
}

type baseHostingContext struct {
	service     *entities.Service
	options     *ziti.ListenOptions
	dialTimeout time.Duration
	onClose     func()
}

func (self *baseHostingContext) ServiceName() string {
	return self.service.Name
}

func (self *baseHostingContext) ListenOptions() *ziti.ListenOptions {
	return self.options
}

func (self *baseHostingContext) dialAddress(options map[string]interface{}, protocol string, address string) (net.Conn, error) {
	var sourceAddr string
	if val, ok := options[tunnel.SourceAddrKey]; ok {
		sourceAddr = val.(string)
	}

	if sourceAddr != "" {
		sourceIp := sourceAddr
		sourcePort := 0
		s := strings.Split(sourceAddr, ":")
		if len(s) == 2 {
			var e error
			sourceIp = s[0]
			sourcePort, e = strconv.Atoi(s[1])
			if e != nil {
				return nil, fmt.Errorf("failed to parse port '%s': %v", s[1], e)
			}
		}
		dialer := net.Dialer{
			LocalAddr: &net.TCPAddr{IP: net.ParseIP(sourceIp), Port: sourcePort},
			Timeout:   self.dialTimeout,
		}
		return dialer.Dial(protocol, address)
	}

	return net.DialTimeout(protocol, address, self.dialTimeout)
}

func (self *baseHostingContext) SupportHalfClose() bool {
	return !strings.Contains(self.service.ServerConfig.Protocol, "udp")
}

func (self *baseHostingContext) SetCloseCallback(f func()) {
	self.onClose = f
}

func (self *baseHostingContext) OnClose() {
	if self.onClose != nil {
		self.onClose()
	}
}

func (self *baseHostingContext) getHealthChecks(provider healthChecksProvider) []health.CheckDefinition {
	var checkDefinitions []health.CheckDefinition

	for _, checkDef := range provider.GetPortChecks() {
		checkDefinitions = append(checkDefinitions, checkDef)
	}

	for _, checkDef := range provider.GetHttpChecks() {
		checkDefinitions = append(checkDefinitions, checkDef)
	}

	return checkDefinitions
}

func (self *baseHostingContext) GetInitialHealthState() (ziti.Precedence, uint16) {
	return self.options.Precedence, self.options.Cost
}

type legacyHostingContext struct {
	baseHostingContext
}

func (self *legacyHostingContext) GetHealthChecks() []health.CheckDefinition {
	return self.getHealthChecks(self.service.ServerConfig)
}

func (self *legacyHostingContext) Dial(options map[string]interface{}) (net.Conn, error) {
	config := self.service.ServerConfig
	return self.dialAddress(options, config.Protocol, config.Hostname+":"+strconv.Itoa(config.Port))
}

func newDefaultHostingContext(identity *edge.CurrentIdentity, service *entities.Service, config serviceConfiguration) *hostV2HostingContext {
	options := getDefaultOptions(identity)
	config.SetListenOptions(options)

	return &hostV2HostingContext{
		baseHostingContext: baseHostingContext{
			service:     service,
			options:     options,
			dialTimeout: config.GetDialTimeout(5 * time.Second),
		},
		config: config,
	}
}

type hostV2HostingContext struct {
	baseHostingContext
	config serviceConfiguration
}

func (self *hostV2HostingContext) GetHealthChecks() []health.CheckDefinition {
	return self.getHealthChecks(self.config)
}

func (self *hostV2HostingContext) Dial(options map[string]interface{}) (net.Conn, error) {
	protocol, err := self.config.GetProtocol(options)
	if err != nil {
		return nil, err
	}

	address, err := self.config.GetAddress(options)
	if err != nil {
		return nil, err
	}

	port, err := self.config.GetPort(options)
	if err != nil {
		return nil, err
	}

	return self.dialAddress(options, protocol, address+":"+port)
}

func getDefaultOptions(identity *edge.CurrentIdentity) *ziti.ListenOptions {
	options := ziti.DefaultListenOptions()
	options.ManualStart = true
	options.Precedence = ziti.GetPrecedenceForLabel(identity.DefaultHostingPrecedence)
	options.Cost = identity.DefaultHostingCost
	return options
}
