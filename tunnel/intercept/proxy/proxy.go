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

package proxy

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/openziti/edge/tunnel/udp_vconn"
	"github.com/openziti/foundation/util/info"
	"github.com/openziti/foundation/util/mempool"
	"github.com/pkg/errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

type Service struct {
	Name     string
	Port     int
	Protocol intercept.Protocol
	Closer   io.Closer
	sync.Mutex
	TunnelService *entities.Service
}

func (self *Service) setCloser(c io.Closer) {
	self.Lock()
	defer self.Unlock()
	self.Closer = c
}

func (self *Service) Stop() error {
	self.Lock()
	defer self.Unlock()
	if self.Closer != nil {
		return self.Closer.Close()
	}
	return nil
}

type interceptor struct {
	interceptIP net.IP
	services    map[string]*Service
	closeCh     chan interface{}
	provider    tunnel.FabricProvider
}

func New(ip net.IP, serviceList []string) (intercept.Interceptor, error) {
	services := make(map[string]*Service, len(serviceList))

	for _, arg := range serviceList {
		parts := strings.Split(arg, ":")
		if len(parts) < 2 || len(parts) > 3 {
			return nil, errors.Errorf("invalid argument '%s'", arg)
		}

		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, errors.Errorf("invalid port specified in '%s'", arg)
		}

		service := &Service{
			Name:     parts[0],
			Port:     port,
			Protocol: intercept.TCP,
		}

		if len(parts) == 3 {
			protocol := parts[2]
			if protocol == "udp" {
				service.Protocol = intercept.UDP
			} else if protocol != "tcp" {
				return nil, errors.Errorf("invalid protocol specified in '%s', must be tcp or udp", arg)
			}
		}
		services[parts[0]] = service
	}

	p := interceptor{
		interceptIP: ip,
		services:    services,
		closeCh:     make(chan interface{}),
	}
	return &p, nil
}

func (p *interceptor) Start(provider tunnel.FabricProvider) {
	log := pfxlog.Logger()
	log.Info("starting proxy interceptor")

	// just stash the context
	p.provider = provider
}

func (p interceptor) Intercept(service *entities.Service, _ dns.Resolver, _ intercept.AddressTracker) error {
	log := pfxlog.Logger().WithField("service", service.Name)

	proxiedService, ok := p.services[service.Name]
	if !ok {
		log.Debugf("service %v was not specified at initialization. not intercepting", service.Name)
		return nil
	}

	proxiedService.TunnelService = service

	// pre-fetch network session todo move this to service poller?
	p.provider.PrepForUse(service.Id)

	go p.runServiceListener(proxiedService)
	return nil
}

func (p *interceptor) runServiceListener(service *Service) {
	if service.Protocol == intercept.TCP {
		p.handleTCP(service)
	} else {
		p.handleUDP(service)
	}
}

func (p *interceptor) handleTCP(service *Service) {
	log := pfxlog.Logger().WithField("service", service.Name)

	listenAddr := net.TCPAddr{IP: p.interceptIP, Port: service.Port}
	server, err := net.Listen("tcp4", listenAddr.String())
	if err != nil {
		log.Fatalln(err)
		p.closeCh <- err
		return
	}
	service.setCloser(server)

	log = log.WithField("addr", server.Addr().String())

	log.Info("service is listening")
	defer log.Info("service stopped")
	defer func() {
		p.closeCh <- fmt.Sprintf("service listener %s exited", service.Name)
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			log.WithError(err).Error("accept failed")
			p.closeCh <- err
			return
		}
		sourceAddr := service.TunnelService.GetSourceAddr(conn.RemoteAddr(), conn.LocalAddr())
		appInfo := tunnel.GetAppInfo("tcp", "", p.interceptIP.String(), strconv.Itoa(service.Port), sourceAddr)
		identity := service.TunnelService.GetDialIdentity(conn.RemoteAddr(), conn.LocalAddr())
		go tunnel.DialAndRun(p.provider, service.TunnelService, identity, conn, appInfo, true)
	}
}

func (p *interceptor) handleUDP(service *Service) {
	log := pfxlog.Logger().WithField("service", service.Name)

	listenAddr := &net.UDPAddr{IP: p.interceptIP, Port: service.Port}
	udpPacketConn, err := net.ListenUDP("udp", listenAddr)
	if err != nil {
		log.Fatalln(err)
		p.closeCh <- err
		return
	}

	service.setCloser(udpPacketConn)

	log = log.WithField("addr", udpPacketConn.LocalAddr().String())

	log.Infof("service %v is listening", service.Name)
	reader := &udpReader{
		service: service.TunnelService,
		conn:    udpPacketConn,
	}
	vconnManager := udp_vconn.NewManager(p.provider, udp_vconn.NewUnlimitedConnectionPolicy(), udp_vconn.NewDefaultExpirationPolicy())
	go reader.generateReadEvents(vconnManager)
}

func (p *interceptor) Stop() {
	log := pfxlog.Logger()
	log.Info("stopping proxy interceptor")

	for _, service := range p.services {
		_ = service.Stop()
	}
}

func (p *interceptor) StopIntercepting(serviceName string, _ intercept.AddressTracker) error {
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
