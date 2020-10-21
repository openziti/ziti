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

package tcp

import (
	"fmt"
	"github.com/openziti/edge/tunnel/intercept/protocols/ip"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
	"time"
)

type tcpState uint16

const (
	tcpsClosed      tcpState = iota // closed
	tcpsListen                      // listening for connection (passive open)
	tcpsSynSent                     // have sent SYN (active open)
	tcpsSynReceived                 // have sent and received SYN; awaiting ACK
	tcpsEstablished                 // established (data transfer)
	tcpsCloseWait                   // received FIN; waiting for application close
	tcpsFinWait1                    // have closed, sent FUN; awaiting ACK and FIN
	tcpsClosing                     // simultaneous close; awaiting ACK
	tcpsLastAck                     // received FIN, have closed; awaiting ACK
	tcpsFinWait2                    // have closed; awaiting FIN
	tcpsTimeWait                    // 2MSL (maximum segment lifetime) wait state after active close
)

type ClientConn struct {
	net.Conn

	clientKey string
	svcKey    string
	srcAddr   *net.TCPAddr // TODO encode addresses into template packet
	dstAddr   *net.TCPAddr
	rxq       chan *tcpQItem // inbound packets from peer
	txq       chan []byte    // queue of layer 3 packets for peer
	dev       io.ReadWriter
	mss       uint

	tState      tcpState
	tStateMtx   sync.Locker
	rcvAdv      uint16 // advertised window from peer  - written in tcpRecv. read in tcpSend (waitForClientAcks)
	rcvWndScale uint8
	rcvAckNum   uint32
	sndUna      uint32 // oldest unacknowledged sequence number
	sndAckNum   uint32 // bytes received. written in tcpRecv. read in tcpSend
	seqCond     *sync.Cond
	log         *logrus.Entry
}

func (conn ClientConn) String() string {
	return fmt.Sprintf("tcpConn(%v -> %v)", conn.srcAddr, conn.dstAddr)
}

func NewClientConn(clientAddr, interceptAddr string, rxq chan *tcpQItem, dev io.ReadWriter, tunMTU uint) (*ClientConn, error) {
	srcAddr, err := net.ResolveTCPAddr("tcp", clientAddr)
	if err != nil {
		return nil, err
	}
	dstAddr, err := net.ResolveTCPAddr("tcp", interceptAddr)
	if err != nil {
		return nil, err
	}
	mss := tunMTU - (ip.IPv4MinimumSize + TCPMinimumSize)
	maxSegCond := &sync.Cond{L: &sync.Mutex{}}
	txq := make(chan []byte, 16)
	for i := 0; i < cap(txq); i++ {
		txq <- make([]byte, tunMTU)
	}

	return &ClientConn{
		clientKey: clientAddr,
		svcKey:    interceptAddr,
		srcAddr:   srcAddr,
		dstAddr:   dstAddr,
		rxq:       rxq,
		txq:       txq,
		dev:       dev,
		mss:       mss,
		tState:    tcpsListen,
		tStateMtx: new(sync.Mutex),
		seqCond:   maxSegCond,
		log:       log.WithFields(logrus.Fields{"src": srcAddr.String(), "dst": dstAddr.String()}),
	}, nil
}

func (conn *ClientConn) tcpRecv(segment TCP, buf []byte) (int, error) {
	conn.log.Debugf("got packet: flags=%d, state=%d", segment.Flags(), conn.tState)

	conn.seqCond.L.Lock()
	defer conn.seqCond.L.Unlock()
	conn.rcvAdv = segment.WindowSize()
	if segment.HasFlags(TCPFlagAck) {
		conn.rcvAckNum = segment.AckNumber()
		conn.seqCond.Signal()
	}

	if segment.HasFlags(TCPFlagSyn) {
		conn.sndAckNum = segment.SequenceNumber() + 1 // +1 for syn received
	}
	if segment.HasFlags(TCPFlagFin) {
		conn.sndAckNum += 1
	}
	n := 0

	conn.tStateMtx.Lock()
	defer conn.tStateMtx.Unlock()

	switch {
	// LISTEN --> SYN_RCVD
	case conn.tState == tcpsListen && segment.HasFlags(TCPFlagSyn):
		synOpts := ParseSynOptions(segment[TCPMinimumSize:segment.DataOffset()], segment.HasFlags(TCPFlagAck))
		if synOpts.WS > 0 {
			conn.rcvWndScale = uint8(synOpts.WS)
		}
		conn.tcpSend(nil, TCPFlagSyn|TCPFlagAck)
		conn.tState = tcpsSynReceived

	// SYN_RCVD --> ESTABLISHED
	case conn.tState == tcpsSynReceived && segment.HasFlags(TCPFlagAck):
		conn.tState = tcpsEstablished

	case conn.tState == tcpsEstablished:
		if segment.HasFlags(TCPFlagFin) {
			// CONNECTION TERMINATED by local application
			conn.tState = tcpsCloseWait
			conn.tcpSend(nil, TCPFlagAck)
		} else {
			// DATA TRANSFER
			n = len(segment.Payload())
			if n > 0 {
				conn.sndAckNum += uint32(n)
				conn.log.Debugf("writing %d bytes to ziti", n)
				conn.tcpSend(nil, TCPFlagAck)
				copy(buf, segment.Payload())
				return n, nil
			}
		}

	case conn.tState == tcpsLastAck && segment.HasFlags(TCPFlagAck):
		conn.tState = tcpsClosed

	// CONNECTION TERMINATED by ziti service
	case conn.tState == tcpsFinWait1:
		n = len(segment.Payload())
		conn.sndAckNum += uint32(n)
		copy(buf, segment.Payload())
		if n > 0 || segment.HasFlags(TCPFlagFin) {
			conn.tcpSend(nil, TCPFlagAck)
		}
		switch {
		case segment.HasFlags(TCPFlagFin | TCPFlagAck):
			conn.tState = tcpsTimeWait
		case segment.HasFlags(TCPFlagFin):
			conn.tState = tcpsClosing
		case segment.HasFlags(TCPFlagAck):
			conn.tState = tcpsFinWait2
		}

	case conn.tState == tcpsFinWait2:
		n = len(segment.Payload())
		conn.sndAckNum += uint32(n)
		copy(buf, segment.Payload())
		if segment.HasFlags(TCPFlagFin) {
			conn.tState = tcpsTimeWait
		}
		conn.tcpSend(nil, TCPFlagAck)

	case conn.tState == tcpsClosing && segment.HasFlags(TCPFlagAck):
		conn.tState = tcpsTimeWait

	case conn.tState == tcpsTimeWait && segment.HasFlags(TCPFlagFin):
		conn.tcpSend(nil, TCPFlagAck)

	default:
		conn.log.Errorf("unexpected state transition %s/%d", conn.srcAddr, conn.tState)
	}

	switch conn.tState {
	case tcpsClosed:
		conn.log.Warn("received packet on closed connection") // todo can this happen - yes - need to drain channel
		return 0, io.EOF
	case tcpsClosing:
		conn.log.Infof("closing %s", conn.clientKey)
		// TODO return 0, io.EOF?
	case tcpsCloseWait:
		conn.tcpSend(nil, TCPFlagFin)
		return 0, io.EOF
	}

	return n, nil
}

// Reads the next packet from the local client
// implements server state transitions described in https://raw.githubusercontent.com/GordonMcKinney/gist-assets/master/TCPIP_State_Transition_Diagram.png
func (conn *ClientConn) Read(buf []byte) (int, error) {
	segment, ok := <-conn.rxq
	if !ok {
		conn.log.Info("EOF while reading")
		return 0, io.EOF
	}
	n, err := conn.tcpRecv(segment.segment, buf)
	segment.release()
	return n, err
}

// Write to the local client.
// seqNum is sequence of first payload byte for connection
// acknum is bytes received from client
func (conn *ClientConn) tcpSend(payload []byte, flags uint8) {
	tcpOpts := []byte(nil)
	if flags&TCPFlagSyn != 0 {
		synOpts := TCPSynOptions{
			MSS: uint16(conn.mss),
			WS:  7,
		}
		tcpOpts = makeSynOptions(synOpts)
	}
	optLen := len(tcpOpts)
	dataLen := len(payload)
	packet := <-conn.txq
	defer func() {
		packet = packet[:]
		conn.txq <- packet
	}()
	packet = packet[:ip.IPv4MinimumSize+TCPMinimumSize+optLen+dataLen]
	ipHeader := ip.IPv4(packet[0:ip.IPv4MinimumSize])
	ipHeader.Encode(&ip.IPv4Fields{
		IHL:         ip.IPv4MinimumSize,
		TotalLength: uint16(len(packet)),
		Protocol:    uint8(TCPProtocolNumber),
		SrcAddr:     conn.dstAddr.IP.To4(),
		DstAddr:     conn.srcAddr.IP.To4(),
		TTL:         64,
	})
	ipHeader.SetChecksum(^ipHeader.CalculateChecksum())
	tcpHeader := TCP(packet[ip.IPv4MinimumSize : ip.IPv4MinimumSize+TCPMinimumSize+optLen])
	tcpHeader.Encode(&TCPFields{
		SrcPort:    uint16(conn.dstAddr.Port),
		DstPort:    uint16(conn.srcAddr.Port),
		SeqNum:     conn.sndUna + 1,
		AckNum:     conn.sndAckNum,
		DataOffset: TCPMinimumSize + uint8(optLen),
		Flags:      flags,
		WindowSize: 0xffff,
	})
	copy(packet[ip.IPv4MinimumSize+TCPMinimumSize:], tcpOpts)
	copy(packet[ip.IPv4MinimumSize+TCPMinimumSize+optLen:], payload)
	sum := ip.PseudoHeaderChecksum(TCPProtocolNumber, conn.dstAddr.IP.To4(), conn.srcAddr.IP.To4())
	sum = ip.Checksum(payload, sum)
	tcpHeader.SetChecksum(^tcpHeader.CalculateChecksum(sum, TCPMinimumSize+uint16(optLen+dataLen)))
	conn.log.Debugf("sending seqNum %d (len %d)", tcpHeader.SequenceNumber(), dataLen)
	nOut, err := conn.dev.Write(packet)
	if nOut != len(packet) {
		conn.log.Errorf("short write %d of %d bytes %v", nOut, len(packet), err)
	}
	inc := uint32(dataLen)
	if flags&(TCPFlagSyn|TCPFlagFin) != 0 {
		inc += 1
	}
	conn.sndUna += inc
}

func (conn *ClientConn) Write(payload []byte) (int, error) {

	switch conn.tState {
	case tcpsSynReceived, tcpsEstablished, tcpsCloseWait: // allowed write states
	default:
		conn.log.Errorf("attempted write in %v", conn.tState)
		return 0, fmt.Errorf("attempted write in %v state", conn.tState)
	}

	n := uint(len(payload))
	var chunkLen uint
	for i := uint(0); i < n; i += conn.mss {
		if i+conn.mss > n {
			chunkLen = n % conn.mss
		} else {
			chunkLen = conn.mss
		}
		chunk := payload[i : i+chunkLen]
		conn.seqCond.L.Lock()
		conn.waitForClientAcks(uint32(chunkLen))
		conn.tcpSend(chunk, TCPFlagAck)
		conn.seqCond.L.Unlock()
	}

	return int(n), nil
}

// check peer's receive window and wait for it to open if necessary
func (conn *ClientConn) waitForClientAcks(i uint32) {
	for conn.sndUna+uint32(i)-conn.rcvAckNum > uint32(conn.rcvAdv)<<conn.rcvWndScale {
		if conn.rcvAdv == 0 {
			conn.log.Debug("window closed. punting.")
			return
		}
		conn.log.Debugf("waiting for client to ack byte %d", conn.sndUna)
		conn.seqCond.Wait()
	}
}

func (conn *ClientConn) CloseWrite() error {
	conn.log.Debug("CloseWrite()")
	conn.tStateMtx.Lock()
	defer conn.tStateMtx.Unlock()

	sendFin := true
	switch conn.tState {
	case tcpsEstablished, tcpsSynReceived:
		conn.tState = tcpsFinWait1
	case tcpsCloseWait:
		conn.tState = tcpsLastAck
	default:
		sendFin = false
	}

	if sendFin {
		conn.tcpSend(nil, TCPFlagFin|TCPFlagAck)
		conn.log.Debug("FIN sent")
	}

	return nil
}

func (conn *ClientConn) Close() error {
	_ = conn.CloseWrite()

	queuesMtx.Lock()
	delete(queues, conn.clientKey) // prevent writes on outQueue which is about to be closed
	queuesMtx.Unlock()

	close(conn.rxq)

	return nil
}

func (conn *ClientConn) LocalAddr() net.Addr {
	return conn.dstAddr
}

func (conn *ClientConn) RemoteAddr() net.Addr {
	return conn.srcAddr
}

func (conn *ClientConn) SetDeadline(t time.Time) error {
	panic("implement me")
}

func (conn *ClientConn) SetReadDeadline(t time.Time) error {
	panic("implement me")
}

func (conn *ClientConn) SetWriteDeadline(t time.Time) error {
	panic("implement me")
}

func (b TCP) HasFlags(flags uint8) bool {
	return b.Flags()&flags == flags
}

const (
	// Maximum space available for options.
	maxOptionSize = 40
)

func makeSynOptions(opts TCPSynOptions) []byte {
	// Emulate linux option order. This is as follows:
	//
	// if md5: NOP NOP MD5SIG 18 md5sig(16)
	// if mss: MSS 4 mss(2)
	// if ts and sack_advertise:
	//	SACK 2 TIMESTAMP 2 timestamp(8)
	// elif ts: NOP NOP TIMESTAMP 10 timestamp(8)
	// elif sack: NOP NOP SACK 2
	// if wscale: NOP WINDOW 3 ws(1)
	// if sack_blocks: NOP NOP SACK ((2 + (#blocks * 8))
	//	[for each block] start_seq(4) end_seq(4)
	// if fastopen_cookie:
	//	if exp: EXP (4 + len(cookie)) FASTOPEN_MAGIC(2)
	// 	else: FASTOPEN (2 + len(cookie))
	//	cookie(variable) [padding to four bytes]
	//
	options := make([]byte, maxOptionSize)

	// Always encode the mss.
	offset := EncodeMSSOption(uint32(opts.MSS), options)

	// Special ordering is required here. If both TS and SACK are enabled,
	// then the SACK option precedes TS, with no padding. If they are
	// enabled individually, then we see padding before the option.
	if opts.TS && opts.SACKPermitted {
		offset += EncodeSACKPermittedOption(options[offset:])
		offset += EncodeTSOption(opts.TSVal, opts.TSEcr, options[offset:])
	} else if opts.TS {
		offset += EncodeNOP(options[offset:])
		offset += EncodeNOP(options[offset:])
		offset += EncodeTSOption(opts.TSVal, opts.TSEcr, options[offset:])
	} else if opts.SACKPermitted {
		offset += EncodeNOP(options[offset:])
		offset += EncodeNOP(options[offset:])
		offset += EncodeSACKPermittedOption(options[offset:])
	}

	// Initialize the WS option.
	if opts.WS >= 0 {
		offset += EncodeNOP(options[offset:])
		offset += EncodeWSOption(opts.WS, options[offset:])
	}

	// Padding to the end; note that this never apply unless we add a
	// fastopen option, we always expect the offset to remain the same.
	if delta := AddTCPOptionPadding(options, offset); delta != 0 {
		panic("unexpected option encoding")
	}

	return options[:offset]
}
