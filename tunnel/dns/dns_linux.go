//go:build linux

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

package dns

import (
	"fmt"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
	"os/exec"
	"sync"
	"time"
)

func NewDnsServer(addr string) (Resolver, error) {
	log.Infof("starting dns server...")
	s := &dns.Server{
		Addr: addr,
		Net:  "udp",
	}

	names := make(map[string]net.IP)
	r := &resolver{
		server:     s,
		names:      names,
		ips:        make(map[string]string),
		namesMtx:   sync.Mutex{},
		domains:    make(map[string]*domainEntry),
		domainsMtx: sync.Mutex{},
	}
	s.Handler = r

	errChan := make(chan error)
	go func() {
		errChan <- s.ListenAndServe()
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return nil, fmt.Errorf("dns server failed to start: %w", err)
		} else {
			return nil, fmt.Errorf("dns server stopped prematurely")
		}
	case <-time.After(2 * time.Second):
		log.Infof("dns server running at %s", s.Addr)
	}

	const resolverConfigHelp = "ziti-tunnel runs an internal DNS server which must be first in the host's\n" +
		"resolver configuration. On systems that use NetManager/dhclient, this can\n" +
		"be achieved by adding the following to /etc/dhcp/dhclient.conf:\n" +
		"\n" +
		"    prepend domain-name-servers %s;\n\n"

	err := r.testSystemResolver()
	if err != nil {
		log.Errorf("system resolver test failed: %s\n\n"+resolverConfigHelp, err, addr)
	}

	return r, nil
}

func flushDnsCaches() {
	bin, err := exec.LookPath("systemd-resolve")
	arg := "--flush-caches"
	if err != nil {
		bin, err = exec.LookPath("resolvectl")
		if err != nil {
			logrus.WithError(err).Warn("unable to find systemd-resolve or resolvectl in path, consider adding a dns flush to your restart process")
			return
		}
		arg = "flush-caches"
	}

	cmd := exec.Command(bin, arg)
	if err = cmd.Run(); err != nil {
		logrus.WithError(err).Warn("unable to flush dns caches, consider adding a dns flush to your restart process")
	} else {
		logrus.Info("dns caches flushed")
	}
}
