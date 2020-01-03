// +build linux

/*
	Copyright 2019 Netfoundry, Inc.

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

package tun

import (
	"fmt"
	"github.com/netfoundry/ziti-edge/tunnel/dns"
	"github.com/netfoundry/ziti-edge/tunnel/intercept"
	"github.com/netfoundry/ziti-edge/tunnel/intercept/protocols"
	"github.com/netfoundry/ziti-edge/tunnel/intercept/protocols/tcp"
	"github.com/netfoundry/ziti-edge/tunnel/intercept/protocols/udp"
	"github.com/netfoundry/ziti-edge/tunnel/router"
	"github.com/netfoundry/ziti-edge/tunnel/utils"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"net"
)

type tunInterceptor struct {
	interceptLUT intercept.LUT
	tunIf        *tunInterface
	localPrefix  *net.IPNet
	udpManager   *udp.TunUDPManager
}

func New(devName string, mtu uint) (intercept.Interceptor, error) {
	tunIf, err := open(devName, mtu)
	if err != nil {
		return nil, fmt.Errorf("failed to open tun interface (name='%s', mtu=%d): %v", devName, mtu, err)
	}

	la, err := utils.NextIP(net.IP{169, 254, 1, 1}, net.IP{169, 254, 254, 254})
	if err != nil {
		return nil, fmt.Errorf("failed to get next IP for tun local address: %v", err)
	}
	prefix := &net.IPNet{IP: la, Mask: net.CIDRMask(32, 32)}

	err = router.AddLocalAddress(prefix, tunIf.iFace.Name)
	if err != nil {
		return nil, err
	}
	i := tunInterceptor{
		interceptLUT: intercept.NewLUT(),
		tunIf:        tunIf,
		localPrefix:  prefix,
		udpManager:   udp.NewManager(tunIf.dev),
	}

	return &i, nil
}

func (t *tunInterceptor) Start(context ziti.Context) {
	go protocols.HandleInboundPackets(context, t.tunIf.dev, uint(t.tunIf.iFace.MTU), t.udpManager)
}

func (t *tunInterceptor) Stop() {
	err := t.tunIf.close()
	if err != nil {
		// TODO
	}
}

func (t *tunInterceptor) Intercept(service edge.Service, resolver dns.Resolver) error {
	interceptAddr, err := intercept.NewInterceptAddress(service, "any", resolver)
	if err != nil {
		return fmt.Errorf("unable to intercept %s: %v", service.Name, err)
	}

	ipNet := interceptAddr.IpNet()
	err = router.AddPointToPointAddress(t.localPrefix.IP, &ipNet, t.tunIf.iFace.Name)
	if err != nil {
		return fmt.Errorf("failed to add route %v", err)
	}

	err = t.interceptLUT.Put(*interceptAddr, service.Name, nil)
	if err != nil {
		e := router.RemovePointToPointAddress(t.localPrefix.IP, &ipNet, t.tunIf.iFace.Name)
		if e != nil {
			return fmt.Errorf("failed to add service to LUT: %v; failed to clean up local route: %v", err, e)
		}
		return fmt.Errorf("failed to add service to LUT: %v", err)
	}
	tcp.RegisterService(service, ipNet.IP)
	t.udpManager.RegisterService(service, ipNet.IP)
	return nil
}

func (t *tunInterceptor) StopIntercepting(serviceName string, removeRoute bool) error {
	services := t.interceptLUT.GetByName(serviceName)
	if len(services) == 0 {
		return fmt.Errorf("service %s not found in intercept LUT", serviceName)
	}
	// keep track of routes used by all intercepts. use a map to avoid duplicates
	routes := map[string]net.IPNet{}
	for _, service := range services {
		defer t.interceptLUT.Remove(service.Addr)
		ipn := service.Addr.IpNet()
		routes[ipn.String()] = ipn
	}
	tcp.UnregisterService(serviceName)
	t.udpManager.UnregisterService(serviceName)

	if removeRoute {
		for _, ipNet := range routes {
			err := router.RemovePointToPointAddress(t.localPrefix.IP, &ipNet, t.tunIf.iFace.Name)
			if err != nil {
				return fmt.Errorf("failed to delete route: %v", err)
			}
		}
	}

	return nil
}
