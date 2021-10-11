/*
	Copyright 2019 NetFoundry, Inc.

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

package xgress_geneve

import (
	"encoding/binary"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fabric/router/xgress"
)

type listener struct{}

func (self *listener) Listen(string, xgress.BindHandler) error {
	go func() {
		log := pfxlog.Logger()
		// Open UDP socket to listen for Geneve Packets
		conn, err := net.ListenPacket("udp", ":6081")
		if err != nil {
			log.WithError(err).Errorf("failed to open geneve interface - udp")
			// error but return gracefully
			return
		}
		// if no error, will log success
		log.Infof("geneve interface started successfully - udp: %s", conn.LocalAddr().String())
		// Close it when done
		defer conn.Close()
		// Open a raw socket to send Modified Packets to Networking Stack
		fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
		if err != nil {
			log.WithError(err).Errorf("failed to open geneve interface - fd")
			// error but return gracefully
			return
		}
		// if no error, will log success
		log.Infof("geneve interface started successfully - fd: %d", fd)
		// Close it when done
		defer syscall.Close(fd)
		// Loop to process packets
		for {
			log := pfxlog.ChannelLogger("geneveListener")
			buf := make([]byte, 9000)
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				log.WithError(err).Errorf("error reading from geneve interface - udp")
				// error but continue to read packets
				continue
			}
			// Remove Geneve layer
			packet := gopacket.NewPacket(buf[:n], layers.LayerTypeGeneve, gopacket.DecodeOptions{NoCopy: true})
			if err := packet.ErrorLayer(); err != nil {
				log.WithError(err.Error()).Errorf("Error decoding some part of the packet")
				// error but continue to read packets
				continue
			}
			// Extract IP Headers and Payload
			if ipNetwork := packet.NetworkLayer(); ipNetwork != nil {
				modifiedPacket := append(ipNetwork.LayerContents(), ipNetwork.LayerPayload()...)
				// Get Destination IP from the IP Header
				var array4byte [4]byte
				copy(array4byte[:], buf[56:60])
				sockAddress := syscall.SockaddrInet4{
					Port: 0,
					Addr: array4byte,
				}
				// Print packet details in debug or trace mode
				log.Tracef("Raw Packet Details: %X", packet)
				log.Tracef("Raw Modified Packet Details: %X", modifiedPacket)
				log.Debugf("DIPv4: %v, SPort: %v, DPort: %v", net.IP(buf[56:60]), binary.BigEndian.Uint16(buf[60:62]), binary.BigEndian.Uint16(buf[62:64]))
				// Send the new packet to be routed to Ziti TProxy
				err = syscall.Sendto(fd, modifiedPacket, 0, &sockAddress)
				if err != nil {
					log.WithError(err).Errorf("failed to send modified packet to geneve interface - fd")
					// error but continue to send packets
					continue
				}
			} else {
				log.WithError(err).Errorf("Packet is not an IP Packet")
				continue
			}
		}
	}()
	return nil
}

func (self *listener) Close() error {
	log := pfxlog.Logger()
	log.Warn("closing geneve interface")
	return nil
}
