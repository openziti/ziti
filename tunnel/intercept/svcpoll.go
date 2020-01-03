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

package intercept

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/tunnel/dns"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func ServicePoller(context ziti.Context, interceptor Interceptor, resolver dns.Resolver, pollRate time.Duration) {
	interceptor.Start(context)
	knownServices := make(map[string]edge.Service)
	log := pfxlog.Logger()

	sig := make(chan os.Signal)
	signal.Notify(sig)

	if pollRate < time.Second {
		pollRate = 15 * time.Second
	}

	for {
		edgeServices, err := context.GetServices()
		if err != nil {
			log.Errorf("failed to get ziti services: %v", err)
			if err.Error() == "unauthorized" {
				if err := context.Authenticate(); err != nil {
					log.WithError(err).Error("could not re-authenticate, session lost")
					break
				}
			}
		}
		added, removed := diffServices(edgeServices, knownServices)
		updateServices(context, interceptor, resolver, added, removed, knownServices)

		select {
		case <-time.After(pollRate):
			continue
		case s := <-sig:
			log.Debugf("caught signal %v", s)
			switch s {
			case syscall.SIGINT, syscall.SIGKILL, syscall.SIGTERM:
				log.Debugf("caught signal %v", s)
				goto done
			}
		}
	}

done:
	// use `updateServices` to stop each intercepted service one-at-a-time.
	updateServices(context, interceptor, resolver, nil, knownServices, knownServices)
	interceptor.Stop()
}

// compare a list of services from the edge controller against the services that we are currently
// familiar with to determine which (if any) services have been added or removed since the last
// time `knownServices` was updated.
func diffServices(edgeServices []edge.Service, knownServices map[string]edge.Service) (added, removed map[string]edge.Service) {
	// find new services
	added = make(map[string]edge.Service)
	edgeServiceIds := make(map[string]struct{})
	for _, edgeSvc := range edgeServices {
		// get edge service IDs for efficiently finding removed services
		edgeServiceIds[edgeSvc.Id] = struct{}{}
		if _, ok := knownServices[edgeSvc.Id]; !ok {
			added[edgeSvc.Id] = edgeSvc
			knownServices[edgeSvc.Id] = edgeSvc
		}
	}

	// look for removed services
	removed = make(map[string]edge.Service)
	for id, knownSvc := range knownServices {
		if _, ok := edgeServiceIds[id]; !ok {
			removed[knownSvc.Id] = knownSvc
			delete(knownServices, id)
		}
	}

	return
}

func updateServices(context ziti.Context, interceptor Interceptor, resolver dns.Resolver, added, removed, all map[string]edge.Service) {
	log := pfxlog.Logger()
	for _, svc := range added {
		log.Infof("starting tunnel for newly available service %s", svc.Name)
		err := interceptor.Intercept(svc, resolver)
		if err != nil {
			log.Errorf("failed to intercept service: %v", err)
		}

		if dialAddr, ok := svc.Tags["tunneler.dial.addr"]; ok && svc.Hostable {
			log.Infof("Hosting newly available service %s", svc.Name)
			go host(context, svc, dialAddr)
		} else {
			if svc.Hostable {
				log.Infof("service %v is hostable but is missing a dial address. Add a 'tunneler.dial.addr' tag to the service to fix", svc.Name)
			} else {
				log.Debugf("service %v not hostable", svc.Name)
			}
		}
	}

	// build map of all in-use address strings, so we know when a route needs to be removed
	allAddrs := make(map[string]int, len(all))
	for _, svc := range all {
		addr := svc.Dns.Hostname
		if _, ok := allAddrs[addr]; !ok {
			allAddrs[addr] += 1
		}
	}

	for _, svc := range removed {
		log.Infof("stopping tunnel for unavailable service: %s", svc.Name)
		useCnt := allAddrs[svc.Dns.Hostname]
		err := interceptor.StopIntercepting(svc.Name, useCnt == 1)
		if err != nil {
			log.Errorf("failed to stop intercepting: %v", err)
		}
		allAddrs[svc.Dns.Hostname] -= 1
	}
}

func host(context ziti.Context, svc edge.Service, addr string) {
	log := pfxlog.Logger()
	listener, err := context.Listen(svc.Name)
	if err != nil {
		log.WithError(err).WithField("service", svc.Name).Errorf("error listening for service: %v", err)
		return
	}
	defer listener.Close()
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.WithError(err).WithField("service", svc.Name).Error("closing listener for service")
			return
		}
		addrParts := strings.SplitAfterN(addr, ":", 2)
		if len(addrParts) != 2 {
			log.WithError(err).
				WithField("service", svc.Name).
				WithField("dialAddr", addr).
				Error("unsupported external address")
			continue
		}
		externalConn, err := net.Dial(strings.TrimSuffix(addrParts[0], ":"), addrParts[1])
		if err != nil {
			log.WithError(err).
				WithField("service", svc.Name).
				WithField("dialAddr", addr).
				Error("dial failed")
			continue
		}
		log.WithField("service", svc.Name).
			WithField("dialAddr", addr).
			Error("hosting service, waiting for connections")
		pipe(svc, addr, conn, externalConn)
	}
}

func pipe(svc edge.Service, addr string, zitiConn net.Conn, externalConn net.Conn) {
	log := pfxlog.Logger()
	closeReadC := make(chan struct{})
	closeWriteC := make(chan struct{})

	copyAndClose := func(reader io.Reader, writer io.Writer, closeCh chan struct{}, context string) {
		_ = copy(reader, writer, context)
		close(closeCh)
	}

	go copyAndClose(zitiConn, externalConn, closeWriteC, "->")
	go copyAndClose(externalConn, zitiConn, closeReadC, "<-")

	go func() {
		defer externalConn.Close()
		defer zitiConn.Close()

		<-closeReadC

		log.WithField("service", svc.Name).WithField("dialAddr", addr).
			Info("communication complete, closing connections")
	}()
}

func copy(reader io.Reader, writer io.Writer, context string) error {
	log := pfxlog.Logger().WithField("type", context)
	buf := make([]byte, 1024)
	for {
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				log.Info("reached EOF on copy, returning")
				return nil
			}
			log.WithError(err).Error("error on copy read, returning")
			return err
		}
		log.WithError(err).Infof("read %v bytes", n)

		writeBuf := buf[:n]
		n, err = writer.Write(writeBuf)
		if err != nil {
			log.WithError(err).Error("error on copy write, returning")
			return err
		}
		log.WithError(err).Infof("wrote %v bytes", n)
	}
}
