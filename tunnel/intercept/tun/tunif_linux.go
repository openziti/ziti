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

package tun

// Create and manipulate tunnel interfaces in Linux.
//
// References:
// - https://backreference.org/2010/03/26/tuntap-interface-tutorial/
// - https://medium.com/@mdlayher/linux-netlink-and-go-part-1-netlink-4781aaeeaca8

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/achanda/go-sysctl"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"github.com/michaelquigley/pfxlog"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"unsafe"
)

const (
	tunCloneDevice = "/dev/net/tun"
)

type tunInterface struct {
	dev   *os.File
	iFace *net.Interface
}

type ifreqFlags struct {
	name  [unix.IFNAMSIZ]byte
	flags int32
}

func open(name string, mtu uint) (*tunInterface, error) {
	const In6AddrGenModeNone = "1"
	f, err := unix.Open(tunCloneDevice, unix.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	tunFlags := ifreqFlags{
		[unix.IFNAMSIZ]byte{},
		unix.IFF_TUN | unix.IFF_NO_PI, // | unix.IFF_TUN_EXCL,
	}
	copy(tunFlags.name[:len(tunFlags.name)-1], []byte(name+"\000"))

	err = ioctl(f, unix.TUNSETIFF, uintptr(unsafe.Pointer(&tunFlags)))
	if err != nil {
		return nil, err
	}

	ifName := string(bytes.TrimRight(tunFlags.name[:], "\000"))
	unix.SetNonblock(f, true)

	// prevent auto-assigned link local IPv6 address when the interface is brought up
	// https://www.toradex.com/community/questions/16932/ipv6-addrconfnetdev-up-eth0-link-is-not-ready.html
	if err = sysctl.Set(fmt.Sprintf("net.ipv6.conf.%s.addr_gen_mode", ifName), In6AddrGenModeNone); err != nil {
		pfxlog.Logger().Warnf("failed setting addr_gen_mode on %s: %v", ifName, err)
	}

	if err = ioctl(f, unix.TUNSETOWNER, 0); err != nil {
		return nil, err
	}

	if err = ioctl(f, unix.TUNSETGROUP, 0); err != nil {
		return nil, err
	}

	tunFlags.flags = int32(mtu)
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, unix.IPPROTO_IP)
	if err != nil {
		return nil, err
	}
	sock := os.NewFile(uintptr(fd), "raw")
	if err = ioctl(int(sock.Fd()), unix.SIOCSIFMTU, uintptr(unsafe.Pointer(&tunFlags))); err != nil {
		return nil, err
	}

	iFace, err := net.InterfaceByName(ifName)
	tun := &tunInterface{
		dev:   os.NewFile(uintptr(f), ifName),
		iFace: iFace,
	}

	err = tun.setFlags([]int16{unix.IFF_UP})
	if err != nil {
		return nil, err
	}

	return tun, nil
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

func (tunIf *tunInterface) close() error {
	setFlagErr := tunIf.setFlags([]int16{^unix.IFF_UP})
	closeErr := tunIf.dev.Close()

	if setFlagErr == nil {
		return closeErr
	}
	if closeErr == nil {
		return setFlagErr
	}
	return fmt.Errorf("multiple failures while closing tun device: (%v) (%v)", setFlagErr, closeErr)
}

func (tunIf *tunInterface) setFlags(flags []int16) error {
	c, err := netlink.Dial(unix.AF_UNSPEC, nil)
	if err != nil {
		return err
	}
	defer closeNetlink(c)

	var ifFlags, mask uint32
	for _, f := range flags {
		ifFlags |= uint32(f)
		if f < 0 {
			mask |= uint32(^f)
		} else {
			mask |= uint32(f)
		}
	}
	ifmBytes := marshalIfInfomsg(&unix.IfInfomsg{
		Family: unix.AF_UNSPEC,
		Type:   unix.ARPHRD_NONE,
		Index:  int32(tunIf.iFace.Index),
		Flags:  ifFlags,
		Change: mask,
	})

	req := netlink.Message{
		Header: netlink.Header{
			Type:  unix.RTM_NEWLINK,
			Flags: unix.NLM_F_REQUEST | unix.NLM_F_ACK,
		},
		Data: ifmBytes,
	}

	msgs, err := c.Execute(req)
	if err != nil {
		return err
	}

	if n := len(msgs); n != 1 {
		return errors.New("expected 1 netlink message, but got something else")
	}

	return nil
}

func closeNetlink(conn *netlink.Conn) {
	err := conn.Close()
	if err != nil {
		pfxlog.Logger().Errorf("failure closing netlink connection (%v)", err)
	}
}

func ioctl(f int, request uint32, argp uintptr) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(f), uintptr(request), argp)
	if errno != 0 {
		return fmt.Errorf("ioctl failed with '%s'", errno)
	}
	return nil
}
