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

package router

import (
	"errors"
	"fmt"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"net"
	"os"
)

// Add an address (or prefix) to the specified network interface.
func AddLocalAddress(prefix *net.IPNet, ifName string) error {
	logrus.Debugf("adding local address '%v' to interface %v", prefix.String(), ifName)
	return nlAddrReq(prefix, nil, ifName, unix.RTM_NEWADDR)
}

func RemoveLocalAddress(prefix *net.IPNet, ifName string) error {
	logrus.Debugf("removing local address '%v' from interface %v", prefix.String(), ifName)
	return nlAddrReq(prefix, nil, ifName, unix.RTM_DELADDR)
}

func ipToIPNet(ip net.IP) *net.IPNet {
	var prefixLen int
	if ip.To4() != nil {
		prefixLen = 32
	} else {
		prefixLen = 128
	}

	return &net.IPNet{IP: ip, Mask: net.CIDRMask(prefixLen, prefixLen)}
}

func AddPointToPointAddress(localIP net.IP, peerPrefix *net.IPNet, ifName string) error {
	return nlAddrReq(ipToIPNet(localIP), peerPrefix, ifName, unix.RTM_NEWADDR)
}

func RemovePointToPointAddress(localIP net.IP, peerPrefix *net.IPNet, ifName string) error {
	return nlAddrReq(ipToIPNet(localIP), peerPrefix, ifName, unix.RTM_DELADDR)
}

func nlAddrReq(localPrefix, peerPrefix *net.IPNet, ifName string, t netlink.HeaderType) error {
	netIf, err := net.InterfaceByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to find interface %s: %v", ifName, err)
	}

	var localIP net.IP
	var addrFamily uint8
	var prefixLen int

	if localPrefix.IP.To4() != nil {
		localIP = localPrefix.IP.To4()
		addrFamily = unix.AF_INET
	} else {
		localIP = localPrefix.IP
		addrFamily = unix.AF_INET6
	}

	rtAttrs := []netlink.Attribute{{Type: unix.IFA_LOCAL, Data: localIP}}

	if peerPrefix == nil {
		// local address - use prefix length from local
		prefixLen, _ = localPrefix.Mask.Size()
	} else {
		// point-to-point address - use prefix length from peer, and add routing attribute.
		// see ip-address(8), rtnetlink(7)
		peerIP := peerPrefix.IP
		if peerIP.To4() != nil {
			if addrFamily != unix.AF_INET {
				return fmt.Errorf("local address '%s' and peer address '%s' have different address family",
					localIP.String(), peerIP.String())
			}
			peerIP = peerIP.To4()
		}
		prefixLen, _ = peerPrefix.Mask.Size()
		rtAttrs = append(rtAttrs, netlink.Attribute{Type: unix.IFA_ADDRESS, Data: peerIP})
	}

	c, err := netlink.Dial(unix.AF_UNSPEC, nil)
	if err != nil {
		return fmt.Errorf("error dialing netlink: %v", err)
	}
	defer closeNetlink(c)

	ifmBytes := marshalIfAddrmsg(&unix.IfAddrmsg{
		Family:    addrFamily,
		Prefixlen: uint8(prefixLen),
		Scope:     unix.RT_SCOPE_HOST,
		Index:     uint32(netIf.Index),
	})
	attrBytes, err := netlink.MarshalAttributes(rtAttrs)
	if err != nil {
		return fmt.Errorf("failed marshalling routing attributes: %v", err)
	}

	req := netlink.Message{
		Header: netlink.Header{
			Type:  t,
			Flags: unix.NLM_F_REQUEST | unix.NLM_F_ACK,
		},
		Data: append(ifmBytes, attrBytes...),
	}

	_, err = c.Execute(req)
	if err != nil {
		var nlErr *netlink.OpError
		if errors.As(err, &nlErr) {
			if os.IsExist(nlErr.Err) {
				return nil
			}
		}
	}

	return err
}

// marshalIfAddrmsg packs a unix.IfAddrmsg into a byte slice using host byte order.
// The returned slice can be included in the payload of a netlink message.
func marshalIfAddrmsg(m *unix.IfAddrmsg) []byte {
	b := make([]byte, unix.SizeofIfAddrmsg)

	b[0] = m.Family
	b[1] = m.Prefixlen
	b[2] = m.Flags
	b[3] = m.Scope
	nlenc.PutUint32(b[4:8], m.Index)

	return b
}

func marshalIfInfomsg(m *unix.IfInfomsg) []byte {
	b := make([]byte, unix.SizeofIfInfomsg)

	b[0] = m.Family
	b[1] = 0 // pad
	nlenc.PutUint16(b[2:4], m.Type)
	nlenc.PutInt32(b[4:8], m.Index)
	nlenc.PutUint32(b[8:12], m.Flags)
	nlenc.PutUint32(b[12:16], m.Change)

	return b
}

func closeNetlink(conn *netlink.Conn) {
	err := conn.Close()
	if err != nil {
		pfxlog.Logger().Errorf("failure closing netlink connection (%v)", err)
	}
}
