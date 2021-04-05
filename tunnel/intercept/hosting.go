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
	var result []tunnel.HostingContext
	for _, t := range service.HostV2Config.Terminators {
		context := newDefaultHostingContext(identity, service, t)
		result = append(result, context)
	}
	return result
}

func newDefaultHostingContext(identity *edge.CurrentIdentity, service *entities.Service, config serviceConfiguration) *hostingContext {
	options := getDefaultOptions(identity)
	config.SetListenOptions(options)

	return &hostingContext{
		service:     service,
		options:     options,
		dialTimeout: config.GetDialTimeout(5 * time.Second),
		config:      config,
	}

}

type hostingContext struct {
	service     *entities.Service
	options     *ziti.ListenOptions
	config      serviceConfiguration
	dialTimeout time.Duration
	onClose     func()
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

func getDefaultOptions(identity *edge.CurrentIdentity) *ziti.ListenOptions {
	options := ziti.DefaultListenOptions()
	options.ManualStart = true
	options.Precedence = ziti.GetPrecedenceForLabel(identity.DefaultHostingPrecedence)
	options.Cost = identity.DefaultHostingCost
	return options
}
