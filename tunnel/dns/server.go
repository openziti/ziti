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

package dns

import (
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
	"net/url"
	"sync"
	"time"
)

var log = logrus.StandardLogger()

type resolver struct {
	server   *dns.Server
	names    map[string]net.IP
	namesMtx sync.Mutex
}

func NewResolver(config string) Resolver {
	if config == "" {
		return nil
	}

	resolverURL, err := url.Parse(config)
	if err != nil {
		log.Fatalf("failed to parse resolver configuration '%s': %s", config, err)
	}

	switch resolverURL.Scheme {
	case "", "file":
		return NewHostFile(resolverURL.Path)
	case "udp":
		return NewDnsServer(resolverURL.Host)
	}

	log.Fatalf("invalid resolver configuration '%s'. must be 'file://' or 'udp://' URL", config)
	return nil
}

func NewDnsServer(addr string) Resolver {
	log.Infof("starting dns server...")
	s := &dns.Server{
		Addr: addr,
		Net:  "udp",
	}

	errChan := make(chan error)
	go func() {
		errChan <- s.ListenAndServe()
	}()

	select {
	case err := <-errChan:
		if err != nil {
			log.Fatalf("dns server failed to start: %s", err)
		} else {
			log.Fatal("dns server stopped prematurely")
		}
	case <-time.After(2 * time.Second):
		log.Infof("dns server running at %s", s.Addr)
	}

	names := make(map[string]net.IP)
	r := &resolver{server: s, names: names, namesMtx: sync.Mutex{}}
	s.Handler = r

	const resolverConfigHelp = "ziti-tunnel runs an internal DNS server which must be first in the host's\n" +
		"resolver configuration. On systems that use NetManager/dhclient, this can\n" +
		"be achieved by adding the following to /etc/dhcp/dhclient.conf:\n" +
		"\n" +
		"    prepend domain-name-servers %s;\n\n"

	err := r.testSystemResolver()
	if err != nil {
		_ = r.Cleanup()
		log.Fatalf("system resolver test failed: %s\n\n"+resolverConfigHelp, err, addr)
	}

	return r
}

func (r *resolver) testSystemResolver() error {
	const resolverTestHostname = "ziti-tunnel.resolver.test"
	resolverTestIP := net.IP{19, 65, 28, 94}
	log.Debug("testing system resolver configuration")

	err := r.AddHostname(resolverTestHostname, resolverTestIP)
	if err != nil {
		return errors.New("failed to add self-test hostname")
	}

	resolved, err := net.ResolveIPAddr("ip", resolverTestHostname)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %v", resolverTestHostname, err)
	}

	// resolverTestIP = net.IP{19, 65, 28, 96} // force test failure by uncommenting
	if !resolved.IP.Equal(resolverTestIP) {
		return fmt.Errorf("unexpected resolved address %s", resolved.IP.String())
	}

	_ = r.RemoveHostname(resolverTestHostname, resolverTestIP)
	return nil
}

func (r *resolver) ServeDNS(w dns.ResponseWriter, query *dns.Msg) {
	log.Debugf("received:\n%s\n", query.String())
	msg := dns.Msg{}
	msg.SetReply(query)
	msg.RecursionAvailable = false
	msg.Rcode = dns.RcodeNotImplemented
	q := query.Question[0]
	switch q.Qtype {
	case dns.TypeA:
		name := q.Name
		address, ok := r.names[name]
		if ok {
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			answer := &dns.A{
				Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   address,
			}
			msg.Answer = append(msg.Answer, answer)
		} else {
			msg.Rcode = dns.RcodeRefused // fail fast, and inspire resolver to query next name server in its list.
		}
	}
	log.Debugf("response:\n%s\n", msg.String())
	err := w.WriteMsg(&msg)
	if err != nil {
		log.Errorf("write failed: %s", err)
	}

}

func (r *resolver) AddHostname(hostname string, ip net.IP) error {
	r.namesMtx.Lock()
	log.Infof("adding %s = %s to resolver", hostname, ip.String())
	r.names[hostname+"."] = ip
	r.namesMtx.Unlock()
	return nil
}

func (r *resolver) RemoveHostname(hostname string, ip net.IP) error {
	r.namesMtx.Lock()
	if _, ok := r.names[hostname+"."]; ok {
		log.Infof("removing %s from resolver", hostname)
	}
	delete(r.names, hostname+".")
	r.namesMtx.Unlock()
	return nil
}

func (r *resolver) Cleanup() error {
	log.Debug("shutting down")
	return r.server.Shutdown()
}
