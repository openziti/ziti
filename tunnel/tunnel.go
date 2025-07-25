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

package tunnel

import (
	"encoding/json"
	"github.com/michaelquigley/pfxlog"
	"strconv"
	"time"

	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"strings"
)

type Service interface {
	GetName() string
	GetId() string
	GetDialTimeout() time.Duration
	IsEncryptionRequired() bool
}

func DialAndRun(fabricProvider FabricProvider, service Service, instanceId string, clientConn net.Conn, appInfo map[string]string, halfClose bool) {
	log := pfxlog.Logger().WithField("service", service.GetName()).WithField("src", clientConn.RemoteAddr().String())
	appInfoJson, err := json.Marshal(appInfo)
	if err != nil {
		log.WithError(err).Error("unable to marshal appInfo")
		_ = clientConn.Close()
		return
	}

	if err = fabricProvider.TunnelService(service, instanceId, clientConn, halfClose, appInfoJson); err != nil {
		log.WithError(err).Error("tunnel failed")
		_ = clientConn.Close()
	}
}

func GetIpAndPort(addr net.Addr) (string, string) {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return tcpAddr.IP.String(), strconv.Itoa(tcpAddr.Port)
	}
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		return udpAddr.IP.String(), strconv.Itoa(udpAddr.Port)
	}

	ipPort := addr.String()
	if idx := strings.LastIndexByte(ipPort, ':'); idx > 0 {
		return ipPort[0:idx], ipPort[idx+1:]
	}

	return "", ""
}

func GetAppInfo(protocol, dstHostname, dstIp, dstPort, sourceAddr string) map[string]string {
	result := map[string]string{}
	result[DestinationProtocolKey] = protocol
	if dstHostname != "" {
		result[DestinationHostname] = dstHostname
	}
	result[DestinationIpKey] = dstIp
	result[DestinationPortKey] = dstPort
	if sourceAddr != "" {
		result[SourceAddrKey] = sourceAddr
	}
	return result
}

func Run(zitiConn edge.Conn, clientConn net.Conn, halfClose bool) {
	loggerFields := logrus.Fields{
		"src-remote": clientConn.RemoteAddr().String(), "src-local": clientConn.LocalAddr().String(),
		"dst-local": zitiConn.LocalAddr().String(), "dst-remote": zitiConn.RemoteAddr().String(),
		"circuitId": zitiConn.GetCircuitId()}

	log := pfxlog.Logger().WithFields(loggerFields)
	log.Info("tunnel started")

	doneSend := make(chan int64)
	doneRecv := make(chan int64)

	go myCopy(clientConn, zitiConn, doneSend, halfClose, zitiConn.GetCircuitId())

	go myCopy(zitiConn, clientConn, doneRecv, halfClose, zitiConn.GetRouterId())

	defer func() {
		_ = clientConn.Close()
		_ = zitiConn.Close()
	}()

	var n1, n2 int64
	for count := 2; count > 0; {
		select {
		case n1 = <-doneSend:
		case n2 = <-doneRecv:
		}
		count = count - 1
	}

	log.Infof("tunnel closed: %d bytes sent; %d bytes received", n2, n1)
}

func myCopy(dst net.Conn, src net.Conn, done chan int64, halfClose bool, circuitId string) {
	loggerFields := logrus.Fields{
		"src-remote": src.RemoteAddr(), "src-local": src.LocalAddr(),
		"dst-local": dst.LocalAddr(), "dst-remote": dst.RemoteAddr(),
		"circuitId": circuitId}

	log := pfxlog.Logger().WithFields(loggerFields)
	defer func() {
		if cw, ok := dst.(edge.CloseWriter); halfClose && ok {
			log.Debug("doing half-close")
			_ = cw.CloseWrite()
		} else {
			log.Debug("doing full-close")
			_ = dst.Close()
		}

	}()

	defer log.Info("stopping pipe")
	// use smaller copyBuf so UDP payloads aren't chunked when sending to tunnelers with smaller MTU.
	// 17 bytes covers encryption overhead.
	copyBuf := make([]byte, 0x4000-17)
	n, err := io.CopyBuffer(dst, src, copyBuf)
	done <- n

	if err != nil {
		log.WithError(err).Error("copy failed")
	}
}
