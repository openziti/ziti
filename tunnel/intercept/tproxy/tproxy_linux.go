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

package tproxy

import (
	"context"
	"fmt"
	"github.com/coreos/go-iptables/iptables"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/tunnel"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/openziti/edge/tunnel/router"
	"github.com/openziti/edge/tunnel/udp_vconn"
	"github.com/openziti/foundation/util/info"
	"github.com/openziti/foundation/util/mempool"
	"github.com/openziti/sdk-golang/ziti"
	"golang.org/x/sys/unix"
	"net"
	"strconv"
	"syscall"
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

type tProxyInterceptor struct {
	interceptLUT intercept.LUT
	ipt          *iptables.IPTables
	tcpLn        net.Listener
	udpLn        *net.UDPConn
}

const (
	mangleTable = "mangle"
	srcChain    = "PREROUTING"
	dstChain    = "NF-INTERCEPT"
)

func contains(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func New() (intercept.Interceptor, error) {
	log := pfxlog.Logger()
	tcpLn, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:")
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener: %v", err)
	}
	log.Infof("tproxy listening on tcp:%s", tcpLn.Addr().String())

	packetLn, err := listenConfig.ListenPacket(context.Background(), "udp", "127.0.0.1:")
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP listener: %v", err)
	}
	udpLn, ok := packetLn.(*net.UDPConn)
	if !ok {
		return nil, fmt.Errorf("failed to create UDP listener. listener was not net.UDPConn")
	}
	log.Infof("tproxy listening on udp:%s, remoteAddr: %v", udpLn.LocalAddr(), udpLn.RemoteAddr())

	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize iptables handle: %v", err)
	}

	chains, err := ipt.ListChains(mangleTable)
	if err != nil {
		return nil, fmt.Errorf("failed to list iptables chains: %v", err)
	}

	if !contains(chains, dstChain) {
		err = ipt.NewChain(mangleTable, dstChain)
		if err != nil {
			return nil, fmt.Errorf("failed to create iptables chain: %v", err)
		}
	}

	err = ipt.AppendUnique(mangleTable, srcChain, []string{"-j", dstChain}...)
	if err != nil {
		return nil, fmt.Errorf("failed to create '%s' --> '%s' link: %v", srcChain, dstChain, err)
	}

	t := tProxyInterceptor{
		interceptLUT: intercept.NewLUT(),
		ipt:          ipt,
		tcpLn:        tcpLn,
		udpLn:        udpLn,
	}
	return &t, nil
}

func (t *tProxyInterceptor) Start(context ziti.Context) {
	t.accept(context)
	t.acceptUDP(context)
}

func (t *tProxyInterceptor) accept(context ziti.Context) {
	log := pfxlog.Logger()
	go func() {
		for {
			client, err := t.tcpLn.Accept()
			if err != nil {
				log.Errorf("error while accepting: %v", err)
			}
			if client == nil {
				log.Info("shutting down")
				return
			}
			log.Infof("received connection: %s --> %s", client.LocalAddr().String(), client.RemoteAddr().String())
			service, err := t.interceptLUT.GetByAddress(client.LocalAddr())
			if service == nil {
				log.Warnf("received connection for %s, which does not map to an intercepted service", client.LocalAddr().String())
				client.Close()
				continue
			}
			go tunnel.DialAndRun(context, service.Name, client)
		}
	}()
}

func (t *tProxyInterceptor) acceptUDP(context ziti.Context) {
	vconnMgr := udp_vconn.NewManager(context, udp_vconn.NewUnlimitedConnectionPolicy(), udp_vconn.NewDefaultExpirationPolicy())
	go t.generateReadEvents(vconnMgr)
}

func (t *tProxyInterceptor) generateReadEvents(manager udp_vconn.Manager) {
	oobSize := 1600
	bufPool := mempool.NewPool(16, info.MaxUdpPacketSize+oobSize)
	log := pfxlog.Logger()

	for {
		pooled := bufPool.AcquireBuffer()
		oob := pooled.Buf[info.MaxUdpPacketSize:]
		pooled.Buf = pooled.Buf[:info.MaxUdpPacketSize]
		log.Debugf("waiting for datagram")
		n, oobn, _, srcAddr, err := t.udpLn.ReadMsgUDP(pooled.Buf, oob)
		if err != nil {
			log.WithError(err).Error("failure while reading udp message. stopping UDP read loop")
			manager.QueueError(err)
			return
		}
		log.Debugf("received %d bytes from %s", n, srcAddr.String())
		pooled.Buf = pooled.Buf[:n]
		event := &udpReadEvent{
			interceptor: t,
			buf:         pooled,
			oob:         oob[:oobn],
			srcAddr:     srcAddr,
		}
		manager.QueueEvent(event)
	}
}

type udpReadEvent struct {
	interceptor *tProxyInterceptor
	buf         *mempool.DefaultPooledBuffer
	oob         []byte
	srcAddr     net.Addr
}

func (event *udpReadEvent) Handle(manager udp_vconn.Manager) error {
	writeQueue := manager.GetWriteQueue(event.srcAddr)

	if writeQueue == nil {
		log := pfxlog.Logger()
		origDest, err := getOriginalDest(event.oob)
		log.Infof("received datagram from %v (original dest %v)", event.srcAddr, origDest)
		if err != nil {
			return fmt.Errorf("error while getting original destination packet: %v", err)
		}
		service, err := event.interceptor.interceptLUT.GetByAddress(origDest)
		if err != nil {
			return err
		}

		pfxlog.Logger().Infof("Creating separate UDP socket with list addr: %v", origDest)
		packetConn, err := listenConfig.ListenPacket(context.Background(), "udp", origDest.String())
		if err != nil {
			return err
		}
		writeConn := packetConn.(*net.UDPConn)
		writeQueue, err = manager.CreateWriteQueue(event.srcAddr, service.Name, writeConn)
		if err != nil {
			return err
		}
	}

	pfxlog.Logger().Infof("received %v bytes for conn %v -> %v", len(event.buf.Buf), writeQueue.LocalAddr().String(), writeQueue.Service())
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

func (t *tProxyInterceptor) Stop() {
	log := pfxlog.Logger()
	err := t.tcpLn.Close()
	if err != nil {
		log.Errorf("failed to close TCP listener: %v", err)
	}

	err = t.udpLn.Close()
	if err != nil {
		log.Errorf("failed to close UDP listener: %v", err)
	}

	var interceptedServices []string
	for _, svc := range t.interceptLUT {
		interceptedServices = append(interceptedServices, svc.Name)
	}
	for _, svcName := range interceptedServices {
		err := t.StopIntercepting(svcName, true)
		if err != nil {
			log.Errorf("failed to clean up intercept configuration for %s: %v", svcName, err)
		}
	}

	err = t.ipt.Delete(mangleTable, srcChain, []string{"-j", dstChain}...)
	if err != nil {
		log.Errorf("failed to unlink chain %s: %v", dstChain, err)
	}

	// this shouldn't be needed after deleting rules one-by-one above, but just in case...
	err = t.ipt.ClearChain(mangleTable, dstChain)
	if err != nil {
		log.Errorf("failed to clear chain %s, %v", dstChain, err)
	}

	err = t.ipt.DeleteChain(mangleTable, dstChain)
	if err != nil {
		log.Errorf("failed to delete chain %s: %v", dstChain, err)
	}
}

func (t *tProxyInterceptor) Intercept(service *entities.Service, resolver dns.Resolver) error {
	err := t.intercept(service, resolver, (*TCPIPPortAddr)(t.tcpLn.Addr().(*net.TCPAddr)))
	if err != nil {
		return err
	}
	return t.intercept(service, resolver, (*UDPIPPortAddr)(t.udpLn.LocalAddr().(*net.UDPAddr)))
}

func (t *tProxyInterceptor) intercept(service *entities.Service, resolver dns.Resolver, port IPPortAddr) error {
	interceptAddr, err := intercept.NewInterceptAddress(service, port.GetProtocol(), resolver)
	if err != nil {
		return fmt.Errorf("unable to intercept %s: %v", service.Name, err)
	}

	ipNet := interceptAddr.IpNet()
	err = router.AddLocalAddress(&ipNet, "lo")
	if err != nil {
		return fmt.Errorf("failed to add local route: %v", err)
	}

	spec := []string{
		"-m", "comment", "--comment", service.Name,
		"-d", ipNet.String(),
		"-p", interceptAddr.Proto(),
		"--dport", strconv.Itoa(interceptAddr.Port()),
		"-j", "TPROXY",
		"--tproxy-mark", "0x1/0x1",
		fmt.Sprintf("--on-ip=%s", port.GetIP().String()),
		fmt.Sprintf("--on-port=%d", port.GetPort()),
	}

	pfxlog.Logger().Infof("Adding rule iptables -t %v -A %v %v", mangleTable, dstChain, spec)
	err = t.ipt.AppendUnique(mangleTable, dstChain, spec...)
	if err != nil {
		return fmt.Errorf("failed to append rule: %v", err)
	}

	err = t.interceptLUT.Put(*interceptAddr, service.Name, spec)
	if err != nil {
		_ = t.ipt.Delete(mangleTable, dstChain, spec...)
		return fmt.Errorf("failed to add intercept record: %v", err)
	}

	return nil
}

func (t *tProxyInterceptor) StopIntercepting(serviceName string, removeRoute bool) error {
	services := t.interceptLUT.GetByName(serviceName)
	if len(services) == 0 {
		return fmt.Errorf("service %s not found in intercept LUT", serviceName)
	}
	// keep track of routes used by all intercepts. use a map to avoid duplicates
	routes := map[string]net.IPNet{}
	for _, service := range services {
		defer t.interceptLUT.Remove(service.Addr)
		err := t.ipt.Delete(mangleTable, dstChain, service.Data.([]string)...)
		if err != nil {
			return fmt.Errorf("failed to remove iptables rule for service %s: %v", serviceName, err)
		}
		ipn := service.Addr.IpNet()
		routes[ipn.String()] = ipn
	}

	if removeRoute {
		for _, ipNet := range routes {
			err := router.RemoveLocalAddress(&ipNet, "lo")
			if err != nil {
				return fmt.Errorf("failed to remove route for service %s: %v", serviceName, err)
			}
		}
	}

	return nil
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
