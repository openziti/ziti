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

package tunnel

import (
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"io"
	"net"
)

var log = logrus.StandardLogger()

func DialAndRun(provider FabricProvider, service string, clientConn net.Conn, halfClose bool) {
	if err := provider.TunnelService(clientConn, service, halfClose); err != nil {
		log.WithError(err).WithField("service", service).Error("tunnel failed")
		_ = clientConn.Close()
	}
}

func Run(zitiConn net.Conn, clientConn net.Conn, halfClose bool) {
	loggerFields := logrus.Fields{
		"src-remote": clientConn.RemoteAddr(), "src-local": clientConn.LocalAddr(),
		"dst-local": zitiConn.LocalAddr(), "dst-remote": zitiConn.RemoteAddr()}

	log := log.WithFields(loggerFields)
	log.Info("tunnel started")

	doneSend := make(chan int64)
	doneRecv := make(chan int64)

	go myCopy(clientConn, zitiConn, doneSend, halfClose)

	go myCopy(zitiConn, clientConn, doneRecv, halfClose)

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

func myCopy(dst net.Conn, src net.Conn, done chan int64, halfClose bool) {
	loggerFields := logrus.Fields{
		"src-remote": src.RemoteAddr(), "src-local": src.LocalAddr(),
		"dst-local": dst.LocalAddr(), "dst-remote": dst.RemoteAddr()}

	logger := log.WithFields(loggerFields)
	defer func() {
		if cw, ok := dst.(edge.CloseWriter); halfClose && ok {
			logger.Debug("doing half-close")
			_ = cw.CloseWrite()
		} else {
			logger.Debug("doing full-close")
			_ = dst.Close()
		}

	}()

	defer logger.Info("stopping pipe")
	// use smaller copyBuf so UDP payloads aren't chunked when sending to tunnelers with smaller MTU.
	// 17 bytes covers encryption overhead.
	copyBuf := make([]byte, 0x4000-17)
	n, err := io.CopyBuffer(dst, src, copyBuf)
	done <- n

	if err != nil {
		log.WithFields(loggerFields).Error(err.Error())
	}
}
