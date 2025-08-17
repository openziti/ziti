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

package tproxy

import (
	"context"
	stdErr "errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/info"
	"github.com/openziti/foundation/v2/mempool"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/tunnel"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/intercept"
	"github.com/openziti/ziti/tunnel/router"
	"github.com/openziti/ziti/tunnel/udp_vconn"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

const (
	DefaultUdpIdleTimeout   = 5 * time.Minute
	DefaultUdpCheckInterval = 30 * time.Second
)

// https://github.com/torvalds/linux/blob/master/Documentation/networking/tproxy.txt

// Configure listening sockets with options that must be set before the socket is bound to an address (IP_TRANSPARENT).
var listenConfig = net.ListenConfig{
	Control: func(network, address string, c syscall.RawConn) error {
		var sockOptErr error
		controlErr := c.Control(func(sockFd uintptr) {
			// - https://www.kernel.org/doc/Documentation/networking/tproxy.txt
			if err := unix.SetsockoptInt(int(sockFd), unix.IPPROTO_IP, unix.IP_TRANSPARENT, 1); err != nil {
				sockOptErr = fmt.Errorf("error setting IP_TRANSPARENT socket option: %v", err)
				return
			}
			if err := unix.SetsockoptInt(int(sockFd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
				sockOptErr = fmt.Errorf("error setting SO_REUSEADDR socket option: %v", err)
				return
			}

			if err := unix.SetsockoptInt(int(sockFd), syscall.SOL_IP, unix.IP_RECVORIGDSTADDR, 1); err != nil {
				sockOptErr = fmt.Errorf("error setting SO_REUSEADDR socket option: %v", err)
				return
			}
		})
		if controlErr != nil {
			return fmt.Errorf("error invoking listener socket control function: %v", controlErr)
		}
		return sockOptErr
	},
}

func New(config Config) (intercept.Interceptor, error) {
	log := pfxlog.Logger()

	self := &interceptor{
		lanIf:            config.LanIf,
		diverter:         config.Diverter,
		udpIdleTimeout:   config.UDPIdleTimeout,
		udpCheckInterval: config.UDPCheckInterval,
		serviceProxies:   cmap.New[*tProxy](),
		ipt:              nil,
	}

	if self.udpIdleTimeout < 5*time.Second {
		self.udpIdleTimeout = DefaultUdpIdleTimeout
		log.Infof("udpIdleTimeout is less than 5s, using default value of %s", DefaultUdpIdleTimeout.String())
	}
	if self.udpCheckInterval < time.Second {
		self.udpCheckInterval = DefaultUdpCheckInterval
		log.Infof("udpCheckInterval is less than 1s, using default value of %s", DefaultUdpCheckInterval.String())
	}

	log.Infof("tproxy config: lanIf            =  [%s]", self.lanIf)
	log.Infof("tproxy config: diverter         =  [%s]", self.diverter)
	log.Infof("tproxy config: udpIdleTimeout   =  [%s]", self.udpIdleTimeout.String())
	log.Infof("tproxy config: udpCheckInterval =  [%s]", self.udpCheckInterval.String())

	dnsNet := intercept.GetDnsInterceptIpRange()
	err := router.AddLocalAddress(dnsNet, "lo")
	if err != nil {
		log.WithError(err).Errorf("unable to add %v to lo", dnsNet)
		return nil, err
	}

	if self.diverter != "" {
		cmd := exec.Command(self.diverter, "-V")
		out, err := cmd.CombinedOutput()
		if err != nil {
			logrus.Errorf("failed to launch external tproxy diverter %s: %v", cmd.String(), out)
			return nil, err
		} else {
			logrus.Infof("using external tproxy diverter %s, version info %s", self.diverter, out)
		}
		return self, nil
	}

	ipt, err := iptables.New()
	if err != nil {
		return nil, errors.Wrap(err, "tproxy: failed to initialize iptables handle")
	}
	self.ipt = ipt
	if err := self.addIptablesChain(self.ipt, mangleTable, "PREROUTING", dstChain); err != nil {
		return nil, err
	}

	if self.lanIf != "" {
		_, err := net.InterfaceByName(self.lanIf)
		if err != nil {
			return nil, fmt.Errorf("invalid lanIf '%s'", self.lanIf)
		}
		err = self.addIptablesChain(self.ipt, filterTable, "INPUT", dstChain)
		if err != nil {
			return nil, err
		}
	} else {
		logrus.Infof("no lan interface specified with '-lanIf'. please ensure firewall accepts intercepted service addresses")
	}

	return self, err
}

type alwaysRemoveAddressTracker struct{}

func (a alwaysRemoveAddressTracker) AddAddress(string) {}

func (a alwaysRemoveAddressTracker) RemoveAddress(string) bool {
	return true
}

type interceptor struct {
	lanIf            string
	diverter         string // external tproxy configuration utility. use internal iptables implementation if not specified.
	udpIdleTimeout   time.Duration
	udpCheckInterval time.Duration

	serviceProxies cmap.ConcurrentMap[string, *tProxy]
	ipt            *iptables.IPTables
}

func (self *interceptor) Stop() {
	self.serviceProxies.IterCb(func(key string, proxy *tProxy) {
		proxy.Stop(alwaysRemoveAddressTracker{})
	})
	self.serviceProxies.Clear()
	self.cleanupChains()
	dnsNet := intercept.GetDnsInterceptIpRange()
	err := router.RemoveLocalAddress(dnsNet, "lo")
	if err != nil {
		logrus.WithError(err).Errorf("failed to remove route for dns IP range '%v' on 'lo'", dnsNet)
	}
}

func (self *interceptor) Intercept(service *entities.Service, resolver dns.Resolver, tracker intercept.AddressTracker) error {
	// only attempt to intercept if the appropriate config is present
	if service.InterceptV1Config == nil {
		return nil
	}

	tproxy, err := self.newTproxy(service, resolver, tracker)
	if err != nil {
		return err
	}
	self.serviceProxies.Set(*service.Name, tproxy)
	return nil
}

func (self *interceptor) StopIntercepting(serviceName string, tracker intercept.AddressTracker) error {
	if proxy, found := self.serviceProxies.Get(serviceName); found {
		proxy.Stop(tracker)
		self.serviceProxies.Remove(serviceName)
	}
	return nil
}

func (self *interceptor) cleanupChains() {
	if self.diverter != "" {
		return
	}
	if self.serviceProxies.IsEmpty() {
		deleteIptablesChain(self.ipt, mangleTable, "PREROUTING", dstChain)
		if self.lanIf != "" {
			deleteIptablesChain(self.ipt, filterTable, "INPUT", dstChain)
		}
	}
}

func (self *interceptor) newTproxy(service *entities.Service, resolver dns.Resolver, tracker intercept.AddressTracker) (*tProxy, error) {
	t := &tProxy{
		interceptor: self,
		service:     service,
		tracker:     tracker,
		resolver:    resolver,
	}

	config := service.InterceptV1Config

	if config == nil {
		return nil, errors.Errorf("service %v has no intercept information", *service.Name)
	}

	if stringz.Contains(config.Protocols, "tcp") {
		tcpLn, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create TCP listener for service: %v", *service.Name)
		}
		logrus.Infof("tproxy listening on tcp:%s", tcpLn.Addr().String())
		t.tcpLn = tcpLn
	}

	if stringz.Contains(config.Protocols, "udp") {
		packetLn, err := listenConfig.ListenPacket(context.Background(), "udp", "127.0.0.1:")
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create UDP listener for service: %v", *service.Name)
		}
		udpLn, ok := packetLn.(*net.UDPConn)
		if !ok {
			return nil, errors.New("failed to create UDP listener. listener was not net.UDPConn")
		}
		logrus.Infof("tproxy listening on udp:%s, remoteAddr: %v", udpLn.LocalAddr(), udpLn.RemoteAddr())
		t.udpLn = udpLn
	}

	if t.tcpLn == nil && t.udpLn == nil {
		return nil, errors.Errorf("service %v has no supported protocols (tcp, udp). Service protocols: %+v", *service.Name, config.Protocols)
	}

	if t.tcpLn != nil {
		go t.acceptTCP()
	}

	if t.udpLn != nil {
		go t.acceptUDP()
	}

	return t, t.Intercept(resolver, tracker)
}

func (self *interceptor) addIptablesChain(ipt *iptables.IPTables, table, srcChain, dstChain string) error {
	chains, err := ipt.ListChains(table)
	if err != nil {
		return fmt.Errorf("failed to list iptables %s chains: %v", table, err)
	}

	if !stringz.Contains(chains, dstChain) {
		err = ipt.NewChain(table, dstChain)
		if err != nil {
			return fmt.Errorf("failed to create iptables chain: %v", err)
		}
	}

	err = ipt.AppendUnique(table, srcChain, []string{"-j", dstChain}...)
	if err != nil {
		return errors.Wrapf(err, "failed to create '%v' link: '%v' --> '%v'", table, srcChain, dstChain)
	} else {
		pfxlog.Logger().Infof("added iptables '%v' link '%v' --> '%v'", table, srcChain, dstChain)
	}

	return nil
}

type tProxy struct {
	interceptor *interceptor
	service     *entities.Service
	addresses   []*intercept.InterceptAddress
	tcpLn       net.Listener
	udpLn       *net.UDPConn
	tracker     intercept.AddressTracker
	resolver    dns.Resolver
	interfaces  []string
}

const (
	mangleTable = "mangle"
	filterTable = "filter"
	dstChain    = "NF-INTERCEPT"
)

func (self *tProxy) acceptTCP() {
	log := pfxlog.Logger()
	for {
		client, err := self.tcpLn.Accept()
		if err != nil {
			log.Errorf("error while accepting: %v", err)
		}
		if client == nil {
			log.Info("shutting down")
			return
		}
		log.Infof("received connection: %s --> %s", client.LocalAddr().String(), client.RemoteAddr().String())
		dstIp, dstPort := tunnel.GetIpAndPort(client.LocalAddr())
		dstHostname, _ := self.resolver.Lookup(client.LocalAddr().(*net.TCPAddr).IP)
		sourceAddr := self.service.GetSourceAddr(client.RemoteAddr(), client.LocalAddr())
		appInfo := tunnel.GetAppInfo("tcp", dstHostname, dstIp, dstPort, sourceAddr)
		identity := self.service.GetDialIdentity(client.RemoteAddr(), client.LocalAddr())
		go tunnel.DialAndRun(self.service.FabricProvider, self.service, identity, client, appInfo, true)
	}
}

func (self *tProxy) acceptUDP() {
	expirationPolicy := udp_vconn.NewTimeoutExpirationPolicy(self.interceptor.udpIdleTimeout, self.interceptor.udpCheckInterval)
	vconnMgr := udp_vconn.NewManager(self.service.GetFabricProvider(), udp_vconn.NewUnlimitedConnectionPolicy(), expirationPolicy)
	self.generateReadEvents(vconnMgr)
}

func (self *tProxy) generateReadEvents(manager udp_vconn.Manager) {
	oobSize := 1600
	bufPool := mempool.NewPool(16, info.MaxUdpPacketSize+oobSize)
	log := pfxlog.Logger()

	for {
		pooled := bufPool.AcquireBuffer()
		oob := pooled.Buf[info.MaxUdpPacketSize:]
		pooled.Buf = pooled.Buf[:info.MaxUdpPacketSize]
		log.Debugf("waiting for datagram")
		n, oobn, _, srcAddr, err := self.udpLn.ReadMsgUDP(pooled.Buf, oob)
		if err != nil {
			log.WithError(err).Error("failure while reading udp message. stopping UDP read loop")
			manager.QueueError(err)
			return
		}
		log.Debugf("received %d bytes from %s", n, srcAddr.String())
		pooled.Buf = pooled.Buf[:n]
		event := &udpReadEvent{
			interceptor: self,
			buf:         pooled,
			oob:         oob[:oobn],
			srcAddr:     srcAddr,
		}
		manager.QueueEvent(event)
	}
}

type udpReadEvent struct {
	interceptor *tProxy
	buf         *mempool.DefaultPooledBuffer
	oob         []byte
	srcAddr     net.Addr
}

func (event *udpReadEvent) Handle(manager udp_vconn.Manager) error {
	writeQueue := manager.GetWriteQueue(event.srcAddr)

	if writeQueue == nil {
		log := pfxlog.Logger()
		origDest, err := getOriginalDest(event.oob)
		if err != nil {
			event.buf.Release()
			return fmt.Errorf("error while getting original destination packet: %v", err)
		}
		log.Infof("received datagram from %v (original dest %v). Creating udp listen socket on original dest", event.srcAddr, origDest)
		packetConn, err := listenConfig.ListenPacket(context.Background(), "udp", origDest.String())
		if err != nil {
			event.buf.Release()
			return err
		}
		writeConn := packetConn.(*net.UDPConn)
		writeQueue, err = manager.CreateWriteQueue(origDest, event.srcAddr, event.interceptor.service, writeConn)
		if err != nil {
			event.buf.Release()
			return err
		}
	}

	pfxlog.Logger().Debugf("received %v bytes for conn %v -> %v", len(event.buf.Buf), writeQueue.LocalAddr().String(), writeQueue.Service())
	writeQueue.Accept(event.buf)

	return nil
}

func getOriginalDest(oob []byte) (*net.UDPAddr, error) {
	cmsgs, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}
	for _, cmsg := range cmsgs {
		if cmsg.Header.Level == syscall.SOL_IP && cmsg.Header.Type == syscall.IP_ORIGDSTADDR {
			ip := cmsg.Data[4:8]
			port := int(cmsg.Data[2])<<8 + int(cmsg.Data[3])
			return &net.UDPAddr{IP: ip, Port: port}, nil
		}
	}
	return nil, fmt.Errorf("original destination not found in out of band data")
}

func deleteIptablesChain(ipt *iptables.IPTables, table, srcChain, dstChain string) {
	log := pfxlog.Logger().WithField("chain", dstChain)
	log.Infof("removing iptables '%v' link '%v' --> '%v'", table, srcChain, dstChain)

	if err := ipt.Delete(table, srcChain, []string{"-j", dstChain}...); err != nil {
		log.WithError(err).Error("failed to unlink chain")
	}

	if err := ipt.ClearChain(table, dstChain); err != nil {
		log.WithError(err).Error("failed to clear chain")
	}

	if err := ipt.DeleteChain(table, dstChain); err != nil {
		log.WithError(err).Error("failed to delete chain")
	}
}

func (self *tProxy) Stop(tracker intercept.AddressTracker) {
	log := pfxlog.Logger().WithField("service", *self.service.Name)
	if self.tcpLn != nil {
		if err := self.tcpLn.Close(); err != nil {
			log.WithError(err).Error("failed to close TCP listener")
		}
	}

	if self.udpLn != nil {
		if err := self.udpLn.Close(); err != nil {
			log.WithError(err).Error("failed to close UDP listener")
		}
	}

	err := self.StopIntercepting(tracker)
	if err != nil {
		log.WithError(err).Error("failed to clean up intercept configuration")
	}
}

func (self *tProxy) tcpPort() IPPortAddr {
	if self.tcpLn != nil {
		return (*TCPIPPortAddr)(self.tcpLn.Addr().(*net.TCPAddr))
	}
	logrus.Errorf("invalid state: no tcp listener for tproxy[%s]", *self.service.Name)
	return nil
}

func (self *tProxy) udpPort() IPPortAddr {
	if self.udpLn != nil {
		return (*UDPIPPortAddr)(self.udpLn.LocalAddr().(*net.UDPAddr))
	}

	logrus.Errorf("invalid state: no udp listener for tproxy[%s]", *self.service.Name)
	return nil
}

func (self *tProxy) Intercept(resolver dns.Resolver, tracker intercept.AddressTracker) error {
	service := self.service
	if service.InterceptV1Config == nil {
		return errors.Errorf("no client configuration for service %v", *service.Name)
	}

	config := service.InterceptV1Config
	logrus.Debugf("service %v using intercept.v1", *service.Name)
	var ports []IPPortAddr
	for _, p := range config.Protocols {
		if p == "tcp" {
			logrus.Debugf("service %v intercepting tcp", *service.Name)
			ports = append(ports, self.tcpPort())
		} else if p == "udp" {
			logrus.Debugf("service %v intercepting udp", *service.Name)
			ports = append(ports, self.udpPort())
		}
	}

	return self.intercept(service, resolver, ports, tracker)
}

func (self *tProxy) Apply(addr *intercept.InterceptAddress) {
	logrus.Debugf("for service %v, intercepting proto: %v, cidr: %v, ports: %v:%v", *self.service.Name, addr.Proto(), addr.IpNet(), addr.LowPort(), addr.HighPort())

	var port IPPortAddr
	switch addr.Proto() {
	case "tcp":
		port = self.tcpPort()
	case "udp":
		port = self.udpPort()
	default:
		logrus.Errorf("unknown proto[%s] for tproxy[%s]", addr.Proto(), *self.service.Name)
		return
	}
	if err := self.addInterceptAddr(addr, self.service, port, self.tracker); err != nil {
		logrus.Debugf("failed for service %v, intercepting proto: %v, cidr: %v, ports: %v:%v", *self.service.Name, addr.Proto(), addr.IpNet(), addr.LowPort(), addr.HighPort())

		// do we undo the previous successful ones?
		// only fail at end and return all that failed?
	}
}

func (self *tProxy) intercept(service *entities.Service, resolver dns.Resolver, ports []IPPortAddr, tracker intercept.AddressTracker) error {
	var protocols []string
	for _, p := range ports {
		protocols = append(protocols, p.GetProtocol())
	}

	err := intercept.GetInterceptAddresses(service, protocols, resolver, self)
	if err != nil {
		return err
	}

	return nil
}

func (self *tProxy) addInterceptAddr(interceptAddr *intercept.InterceptAddress, service *entities.Service, port IPPortAddr, tracker intercept.AddressTracker) error {
	ipNet := interceptAddr.IpNet()
	if interceptAddr.RouteRequired() {
		if err := router.AddLocalAddress(ipNet, "lo"); err != nil {
			return errors.Wrapf(err, "failed to add local route %v", ipNet)
		}
	}
	self.addresses = append(self.addresses, interceptAddr)

	if self.interceptor.diverter != "" {
		interfaces := &entities.InterfacesV1Config{}
		found, err := service.GetConfigOfType(entities.InterfacesV1, &interfaces)
		if err != nil {
			return fmt.Errorf("unable to parse interfaces.v1 config for service %s (%w)", *service.Name, err)
		}

		if !found {
			self.interfaces = []string{""}
		} else {
			self.interfaces = interfaces.Interfaces
		}

		for _, intf := range self.interfaces {
			if err = self.interceptWithDiverter(service, interceptAddr, port, ipNet, intf); err != nil {
				return err
			}
		}
	} else {
		baseSpec := []string{
			"-m", "comment", "--comment", *service.Name,
			"-d", ipNet.String(),
			"-p", interceptAddr.Proto(),
			"--dport", fmt.Sprintf("%v:%v", interceptAddr.LowPort(), interceptAddr.HighPort()),
		}

		if service.InterceptV1Config.AllowedSourceAddresses != nil {
			baseSpec = append(baseSpec, "-s", strings.Join(service.InterceptV1Config.AllowedSourceAddresses, ","))
		}

		interceptAddr.TproxySpec = append(baseSpec,
			"-j", "TPROXY",
			"--tproxy-mark", "0x1/0x1",
			fmt.Sprintf("--on-ip=%s", port.GetIP().String()),
			fmt.Sprintf("--on-port=%d", port.GetPort()),
		)

		pfxlog.Logger().Infof("Adding rule iptables -t %v -A %v %v", mangleTable, dstChain, interceptAddr.TproxySpec)
		if err := self.interceptor.ipt.Insert(mangleTable, dstChain, 1, interceptAddr.TproxySpec...); err != nil {
			return errors.Wrap(err, "failed to insert rule")
		}

		if self.interceptor.lanIf != "" {
			interceptAddr.AcceptSpec = []string{
				"-i", self.interceptor.lanIf,
				"-m", "comment", "--comment", *service.Name,
				"-d", ipNet.String(),
				"-p", interceptAddr.Proto(),
				"--dport", fmt.Sprintf("%v:%v", interceptAddr.LowPort(), interceptAddr.HighPort()),
				"-j", "ACCEPT",
			}
			pfxlog.Logger().Infof("Adding rule iptables -t %v -A %v %v", filterTable, dstChain, interceptAddr.AcceptSpec)
			if err := self.interceptor.ipt.Insert(filterTable, dstChain, 1, interceptAddr.AcceptSpec...); err != nil {
				return errors.Wrap(err, "failed to insert rule")
			}
		}
	}

	return nil
}

func (self *tProxy) interceptWithDiverter(service *entities.Service, interceptAddr *intercept.InterceptAddress, port IPPortAddr, ipNet *net.IPNet, intf string) error {
	for _, srcCidr := range self.getSourceAddressesAsCidrs() {
		cidr := strings.Split(ipNet.String(), "/")
		if len(cidr) != 2 {
			return errors.Errorf("failed parsing '%s' as cidr", ipNet.String())
		}

		args := []string{
			"-I",
			"-c", cidr[0],
			"-m", cidr[1],
			"-p", interceptAddr.Proto(),
			"-o", srcCidr.ip,
			"-n", srcCidr.prefixLen,
			"-l", fmt.Sprintf("%d", interceptAddr.LowPort()),
			"-h", fmt.Sprintf("%d", interceptAddr.HighPort()),
			"-t", fmt.Sprintf("%d", port.GetPort()),
			"-s", *service.ID,
		}

		if intf != "" {
			args = append(args, "-N", intf)
		}

		cmd := exec.Command(self.interceptor.diverter, args...)

		cmdLogger := pfxlog.Logger().WithField("command", cmd.String())
		cmdLogger.Debug("running external diverter")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Errorf("diverter command failed. output: %s", out)
		} else {
			cmdLogger.Infof("diverter command succeeded. output: %s", out)
		}
	}
	return nil
}

func (self *tProxy) StopIntercepting(tracker intercept.AddressTracker) error {
	var errorList []error

	log := pfxlog.Logger().WithField("service", *self.service.Name)

	for _, addr := range self.addresses {
		log := log.WithField("route", addr.IpNet())
		log.Infof("removing intercepted low-port: %v, high-port: %v", addr.LowPort(), addr.HighPort())

		if self.interceptor.diverter != "" {
			for _, intf := range self.interfaces {
				for _, srcCidr := range self.getSourceAddressesAsCidrs() {
					cidr := strings.Split(addr.IpNet().String(), "/")
					if len(cidr) != 2 {
						return errors.Errorf("failed parsing '%s' as cidr", addr.IpNet().String())
					}

					args := []string{
						"-D",
						"-c", cidr[0],
						"-m", cidr[1],
						"-p", addr.Proto(),
						"-o", srcCidr.ip,
						"-n", srcCidr.prefixLen,
						"-l", fmt.Sprintf("%d", addr.LowPort()),
						"-h", fmt.Sprintf("%d", addr.HighPort()),
					}

					if intf != "" {
						args = append(args, "-N", intf)
					}

					cmd := exec.Command(self.interceptor.diverter, args...)
					cmdLogger := pfxlog.Logger().WithField("command", cmd.String())
					cmdLogger.Debug("running external diverter")
					out, err := cmd.CombinedOutput()
					if err != nil {
						errorList = append(errorList, err)
						cmdLogger.Errorf("diverter command failed. output: %s", out)
					} else {
						cmdLogger.Infof("diverter command succeeded. output: %s", out)
					}
				}
			}
		} else {
			log.Infof("Removing rule iptables -t %v -A %v %v", mangleTable, dstChain, addr.TproxySpec)
			err := self.interceptor.ipt.Delete(mangleTable, dstChain, addr.TproxySpec...)
			if err != nil {
				errorList = append(errorList, err)
				log.WithError(err).Errorf("failed to remove iptables rule for service %s", *self.service.Name)
			}
			if self.interceptor.lanIf != "" {
				pfxlog.Logger().Infof("Removing rule iptables -t %v -A %v %v", filterTable, dstChain, addr.TproxySpec)
				err = self.interceptor.ipt.Delete(filterTable, dstChain, addr.AcceptSpec...)
				if err != nil {
					errorList = append(errorList, err)
					log.WithError(err).Errorf("failed to remove iptables rule for service %s", *self.service.Name)
				}
			}
		}

		ipNet := addr.IpNet()
		if addr.RouteRequired() {
			if tracker.RemoveAddress(ipNet.String()) {
				err := router.RemoveLocalAddress(ipNet, "lo")
				if err != nil {
					errorList = append(errorList, err)
					log.WithError(err).Errorf("failed to remove route %v for service %s", ipNet, *self.service.Name)
				}
			}
		}
	}

	if len(errorList) == 0 {
		return nil
	}
	if len(errorList) == 1 {
		return errorList[0]
	}
	return stdErr.Join(errorList...)
}

type IPPortAddr interface {
	GetIP() net.IP
	GetPort() int
	GetProtocol() string
}

type UDPIPPortAddr net.UDPAddr

func (addr *UDPIPPortAddr) GetIP() net.IP {
	return addr.IP
}

func (addr *UDPIPPortAddr) GetPort() int {
	return addr.Port
}

func (addr *UDPIPPortAddr) GetProtocol() string {
	return "udp"
}

type TCPIPPortAddr net.TCPAddr

func (addr *TCPIPPortAddr) GetIP() net.IP {
	return addr.IP
}

func (addr *TCPIPPortAddr) GetPort() int {
	return addr.Port
}

func (addr *TCPIPPortAddr) GetProtocol() string {
	return "tcp"
}

type cidrString = struct {
	ip        string
	prefixLen string
}

func (self *tProxy) getSourceAddressesAsCidrs() []cidrString {
	var srcAddrs []string
	if self.service.InterceptV1Config.AllowedSourceAddresses != nil {
		srcAddrs = self.service.InterceptV1Config.AllowedSourceAddresses
	} else {
		srcAddrs = []string{"0.0.0.0/0"}
	}

	var cidrStrings []cidrString

	for _, srcAddr := range srcAddrs {
		srcCidr := strings.Split(srcAddr, "/")
		ip := srcCidr[0]
		prefixLen := "32"
		if len(srcCidr) == 2 {
			prefixLen = srcCidr[1]
		}
		cidrStrings = append(cidrStrings, cidrString{ip: ip, prefixLen: prefixLen})

	}

	return cidrStrings
}
