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

package subcmd

import (
	"github.com/netfoundry/ziti-edge/sdk/ziti"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net"
	"strings"
)

var log = logrus.StandardLogger()

var runCmd = &cobra.Command{
	Use:   "run <service-name:port> [sevice-name:port]",
	Short: "Run ziti-proxy",
	Args:  cobra.MinimumNArgs(1),
	Run:   runFunc,
}

var context ziti.Context

func init() {
	root.AddCommand(runCmd)
}

func runFunc(cmd *cobra.Command, args []string) {
	log.Info(args)
	closeCh := make(chan interface{})
	count := 0

	context = ziti.NewContext()

	for _, arg := range args {
		count = count + 1
		go runServiceListener(arg, closeCh)
	}

	for count > 0 {
		res := <-closeCh

		err, ok := res.(error)
		if ok {
			logrus.Error(err)
		}
		count = count - 1
	}
}

func runServiceListener(arg string, close chan interface{}) {
	logger := pfxlog.Logger()

	idx := strings.LastIndex(arg, ":")
	if idx == -1 {
		log.Error("Invalid argument")
	}

	port := arg[idx+1:]
	serviceName := arg[0:idx]

	logger = logger.WithField("service", serviceName)

	if id, ok, err := context.GetServiceId(serviceName); ok {
		// pre-fetch network session
		if ns, err := context.GetNetworkSession(id); err != nil {
			logger.WithError(err).Error("failed to acquire network session")
			close <- err
			return
		} else {
			logger.WithField("id", ns.Id).Debug("acquired network session")
		}
	} else if err != nil {
		logger.Fatalln(err)
		close <- err
		return
	} else {
		logger.Error("service is not accessible")
		close <- fmt.Errorf("service is not accessible")
		return
	}

	server, err := net.Listen("tcp4", "0.0.0.0:"+port)
	if err != nil {
		logger.Fatalln(err)
		close <- err
		return
	}

	logger = logger.WithField("addr", server.Addr().String())

	logger.Info("service is listening on")
	defer logger.Info("service stopped")
	defer func() {
		close <- fmt.Sprintf("service listener %s exited", serviceName)
	}()

	for {
		conn, err := server.Accept()
		if err != nil {
			logger.WithError(err).Error("accept failed")
			close <- err
			return
		}
		go RunConnection(serviceName, conn)
	}
}

func RunConnection(service string, conn net.Conn) {
	logger := pfxlog.Logger().WithFields(logrus.Fields{
		"service": service,
		"client":  conn.RemoteAddr().String()})

	c := conn.RemoteAddr().String()
	logger.Debugln("accepted")

	proxyConn, err := context.Dial(service)
	if err != nil {
		logger.WithError(err).Errorf("dial error")
		return
	}
	done1 := make(chan int64)
	done2 := make(chan int64)

	go myCopy(c, service, conn, proxyConn, done1)
	go myCopy(c, service, proxyConn, conn, done2)

	for count := 2; count > 0; {
		select {
		case n1 := <-done1:
			logger.WithField("bytes", n1).Debug("received")
		case n2 := <-done2:
			logger.WithField("bytes", n2).Debug("sent")
		}
		count = count - 1
	}
	logger.Debug("done")
	conn.Close()
}

func myCopy(clt string, service string, dst io.WriteCloser, src io.ReadCloser, done chan int64) {
	defer dst.Close()
	n, err := io.Copy(dst, src)
	if err != nil {
		log.WithFields(logrus.Fields{"client": clt, "service": service}).Error(err.Error())
	}
	done <- n
}
