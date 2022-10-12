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

package utils

import (
	"net"
	"os"
	"syscall"
	"unsafe"
)

// Return all IP addresses that are currently in use on the local host.
//
// This function differs from net.InterfaceAddrs() in that peer addresses
// for point-to-point interfaces are returned by this function, in addition
// to local interface addresses.
func AllInterfaceAddrs() ([]net.Addr, error) {
	tab, err := syscall.NetlinkRIB(syscall.RTM_GETADDR, syscall.AF_INET)
	if err != nil {
		return nil, os.NewSyscallError("netlinkrib", err)
	}
	msgs, err := syscall.ParseNetlinkMessage(tab)
	if err != nil {
		return nil, os.NewSyscallError("parsenetlinkmessage", err)
	}

	var ifat []net.Addr
loop:
	for _, m := range msgs {
		switch m.Header.Type {
		case syscall.NLMSG_DONE:
			break loop
		case syscall.RTM_NEWADDR:
			ifam := (*syscall.IfAddrmsg)(unsafe.Pointer(&m.Data[0]))
			attrs, err := syscall.ParseNetlinkRouteAttr(&m)
			if err != nil {
				return nil, os.NewSyscallError("parsenetlinkrouteattr", err)
			}
			ifa := newAddr(ifam, attrs)
			ifat = append(ifat, ifa...)
		}
	}
	return ifat, nil
}

func newAddr(ifam *syscall.IfAddrmsg, attrs []syscall.NetlinkRouteAttr) []net.Addr {
	var addrs []net.Addr
	for _, a := range attrs {
		switch a.Attr.Type {
		case syscall.IFA_ADDRESS:
			fallthrough
		case syscall.IFA_LOCAL:
			switch ifam.Family {
			case syscall.AF_INET:
				addr := &net.IPNet{IP: net.IPv4(a.Value[0], a.Value[1], a.Value[2], a.Value[3]), Mask: net.CIDRMask(int(ifam.Prefixlen), 8*net.IPv4len)}
				addrs = append(addrs, addr)
			case syscall.AF_INET6:
				addr := &net.IPNet{IP: make(net.IP, net.IPv6len), Mask: net.CIDRMask(int(ifam.Prefixlen), 8*net.IPv6len)}
				copy(addr.IP, a.Value[:])
				addrs = append(addrs, addr)
			}
		}
	}
	return addrs
}
