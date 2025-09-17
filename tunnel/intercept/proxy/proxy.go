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

package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/foundation/v2/mempool"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/tunnel"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/intercept"
	"github.com/openziti/ziti/tunnel/udp_vconn"
)

type Service struct {
	Name      string
	Port      int
	Protocols []intercept.Protocol
	Binding   string
	Closers   map[string]io.Closer
	sync.Mutex
	TunnelService *entities.Service
}

func (self *Service) addCloser(closerType string, c io.Closer) {
	self.Lock()
	defer self.Unlock()
	self.Closers[closerType] = c
}

func (self *Service) Stop() error {
	self.Lock()
	defer self.Unlock()

	logger := pfxlog.Logger().WithField("service", self.Name)

	var errList []error
	for listenerType, closer := range self.Closers {
		logger.Infof("closing %s listener for service of type", listenerType)
		if err := closer.Close(); err != nil {
			errList = append(errList, err)
		}
	}
	clear(self.Closers)
	return errors.Join(errList...)
}

type interceptor struct {
	interceptIP net.IP
	services    map[string]*Service
	lock        sync.Mutex
}

func NewDelegate() intercept.Interceptor {
	return &interceptor{
		interceptIP: net.IPv4(127, 0, 0, 1),
		services:    map[string]*Service{},
	}
}

func New(ip net.IP, serviceList []string) (intercept.Interceptor, error) {
	services := make(map[string]*Service, len(serviceList))

	for _, arg := range serviceList {
		parts := strings.Split(arg, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, fmt.Errorf("invalid argument '%s'", arg)
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid port specified in '%s'", arg)
		}

		service := &Service{
			Name:    parts[0],
			Port:    port,
			Closers: map[string]io.Closer{},
		}

		if len(parts) == 3 {
			protocol := parts[2]
			if protocol == "tcp" {
				service.Protocols = append(service.Protocols, intercept.TCP)
			} else if protocol == "udp" {
				service.Protocols = append(service.Protocols, intercept.UDP)
			} else {
				return nil, fmt.Errorf("invalid protocol specified in '%s', must be tcp or udp", arg)
			}
		}
		services[parts[0]] = service
	}

	p := interceptor{
		interceptIP: ip,
		services:    services,
	}
	return &p, nil
}

func (p *interceptor) Intercept(service *entities.Service, _ dns.Resolver, _ intercept.AddressTracker) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	log := pfxlog.Logger().WithField("service", service.Name)

	proxiedService, hasStaticProxyConfig := p.services[*service.Name]

	if proxiedService != nil {
		if err := proxiedService.Stop(); err != nil {
			log.WithError(err).Error("error stopping existing proxied service listeners")
		}
	}

	proxyConfig := &entities.ProxyV1Config{}
	hasProxyConfig, err := service.GetConfigOfType(entities.ProxyV1, proxyConfig)
	if err != nil {
		if !hasStaticProxyConfig {
			return err
		} else {
			log.WithError(err).Error("error loading dynamic service proxy configuration")
		}
	} else if hasProxyConfig {
		proxiedService = &Service{
			Name:    *service.Name,
			Port:    int(proxyConfig.Port),
			Binding: proxyConfig.Binding,
			Closers: map[string]io.Closer{},
		}

		for _, protocol := range proxyConfig.Protocols {
			if protocol == "tcp" {
				proxiedService.Protocols = append(proxiedService.Protocols, intercept.TCP)
			} else if protocol == "udp" {
				proxiedService.Protocols = append(proxiedService.Protocols, intercept.UDP)
			} else {
				return fmt.Errorf("unexpected proxy protocol '%s' for service '%s'", protocol, proxiedService.Name)
			}
		}

		p.services[proxiedService.Name] = proxiedService
	}

	if proxiedService == nil {
		log.Debugf("service %v was not specified at initialization and has no proxy config, no proxy listener being created", service.Name)
		return nil
	}

	proxiedService.TunnelService = service

	// pre-fetch network session todo move this to service poller?
	service.FabricProvider.PrepForUse(*service.ID)

	return p.runServiceListener(proxiedService)
}

func (p *interceptor) runServiceListener(service *Service) error {
	var errList []error
	for _, protocol := range service.Protocols {
		if protocol == intercept.TCP {
			if err := p.handleTCP(service); err != nil {
				errList = append(errList, err)
			}
		} else if protocol == intercept.UDP {
			if err := p.handleUDP(service); err != nil {
				errList = append(errList, err)
			}
		}
	}
	return errors.Join(errList...)
}

func (p *interceptor) handleTCP(service *Service) error {
	log := pfxlog.Logger().WithField("service", service.Name)

	serviceBinding := p.interceptIP
	if service.Binding != "" {
		binding, err := transport.ResolveLocalBinding(service.Binding)
		if err != nil {
			return fmt.Errorf("unable to resolve service binding '%s' (%w)", service.Name, err)
		}
		serviceBinding = binding
	}

	listenAddr := net.TCPAddr{IP: serviceBinding, Port: service.Port}
	server, err := net.Listen("tcp4", listenAddr.String())
	if err != nil {
		return err
	}
	service.addCloser("tcp", server)

	log = log.WithField("addr", server.Addr().String())

	log.Info("service is listening")

	go func() {
		defer log.Info("service stopped")

		for {
			conn, err := server.Accept()
			if err != nil {
				log.WithError(err).Error("accept failed")
				return
			}
			sourceAddr := service.TunnelService.GetSourceAddr(conn.RemoteAddr(), conn.LocalAddr())
			appInfo := tunnel.GetAppInfo("tcp", "", p.interceptIP.String(), strconv.Itoa(service.Port), sourceAddr)
			identity := service.TunnelService.GetDialIdentity(conn.RemoteAddr(), conn.LocalAddr())
			go tunnel.DialAndRun(service.TunnelService.FabricProvider, service.TunnelService, identity, conn, appInfo, true)
		}
	}()

	return nil
}

func (p *interceptor) handleUDP(service *Service) error {
	log := pfxlog.Logger().WithField("service", service.Name)

	serviceBinding := p.interceptIP
	if service.Binding != "" {
		binding, err := transport.ResolveLocalBinding(service.Binding)
		if err != nil {
			return fmt.Errorf("unable to resolve service binding '%s' (%w)", service.Name, err)
		}
		serviceBinding = binding
	}

	listenAddr := &net.UDPAddr{IP: serviceBinding, Port: service.Port}
	udpPacketConn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		return err
	}

	service.addCloser("udp", udpPacketConn)

	log = log.WithField("addr", udpPacketConn.LocalAddr().String())

	log.Infof("service %v is listening", service.Name)
	reader := &udpReader{
		service: service.TunnelService,
		conn:    udpPacketConn,
	}
	vconnManager := udp_vconn.NewManager(service.TunnelService.FabricProvider, udp_vconn.NewUnlimitedConnectionPolicy(), udp_vconn.NewDefaultExpirationPolicy())
	go reader.generateReadEvents(vconnManager)
	return nil
}

func (p *interceptor) Stop() {
	pfxlog.Logger().Info("stopping proxy interceptor")

	for _, service := range p.services {
		_ = service.Stop()
	}
}

func (p *interceptor) StopIntercepting(serviceName string, _ intercept.AddressTracker) error {
	p.lock.Lock()
	defer p.lock.Unlock()

	pfxlog.Logger().WithField("service", serviceName).Info("stopping proxy interceptor for service")
	if service, ok := p.services[serviceName]; ok {
		return service.Stop()
	}
	return nil
}

type udpReader struct {
	service *entities.Service
	conn    *net.UDPConn
}

func (reader *udpReader) generateReadEvents(manager udp_vconn.Manager) {
	log := pfxlog.Logger().WithField("service", reader.service.Name)
	bufPool := mempool.NewPool(16, info.MaxUdpPacketSize)
	for {
		buf := bufPool.AcquireBuffer()
		n, srcAddr, err := reader.conn.ReadFromUDP(buf.Buf)
		if err != nil {
			log.WithError(err).Error("failure while reading udp message. stopping UDP read loop")
			manager.QueueError(err)
			return
		}

		log.Debugf("read %v bytes from udp, queuing", len(buf.GetPayload()))
		buf.Buf = buf.Buf[:n]
		event := &udpReadEvent{
			reader:  reader,
			buf:     buf,
			srcAddr: srcAddr,
		}
		manager.QueueEvent(event)
	}
}

type udpReadEvent struct {
	reader  *udpReader
	buf     *mempool.DefaultPooledBuffer
	srcAddr net.Addr
}

func (event *udpReadEvent) Handle(manager udp_vconn.Manager) error {
	log := pfxlog.Logger()

	writeQueue := manager.GetWriteQueue(event.srcAddr)

	if writeQueue == nil {
		log.Infof("received connection for %v --> %v, which maps to intercepted service %v",
			event.srcAddr, event.reader.conn.LocalAddr(), event.reader.service)
		var err error
		writeQueue, err = manager.CreateWriteQueue(event.srcAddr.(*net.UDPAddr), event.srcAddr, event.reader.service, event.reader.conn)
		if err != nil {
			return err
		}
	}

	log.Infof("received %v bytes for conn %v -> %v", len(event.buf.Buf), writeQueue.LocalAddr(), writeQueue.Service())
	writeQueue.Accept(event.buf)

	return nil
}
