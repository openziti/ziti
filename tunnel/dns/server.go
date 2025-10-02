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
	"errors"
	"fmt"
	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
	"net"
	"net/url"
	"strings"
	"sync"
)

var log = logrus.StandardLogger()

type unansweredDisposition int

const (
    unansweredRefused unansweredDisposition = iota
    unansweredServfail
    unansweredTimeout
)

var unansweredKeywords = map[string]unansweredDisposition{
    "refused":  unansweredRefused,
    "servfail": unansweredServfail,
    "timeout":  unansweredTimeout,
}

type resolver struct {
	server         *dns.Server
	names          map[string]net.IP
	ips            map[string]string
	namesMtx       sync.Mutex
	domains        map[string]*domainEntry
	domainsMtx     sync.Mutex
	upstreamServer string
	upstreamClient *dns.Client
	unanswered     unansweredDisposition
}

func parseUnansweredDisposition(raw string) (unansweredDisposition, error) {
	if raw == "" {
		return unansweredRefused, nil
	}

	if disp, found := unansweredKeywords[strings.ToLower(raw)]; found {
		return disp, nil
	}

	return unansweredRefused, fmt.Errorf("invalid unanswerable response '%s': must be one of timeout, servfail, or refused", raw)
}

func NewResolver(config string, upstreamConfig string, unansweredConfig string) (Resolver, error) {
	flushDnsCaches()
	if config == "" {
		return nil, nil
	}

	resolverURL, err := url.Parse(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resolver configuration '%s': %w", config, err)
	}

	unanswered := unansweredRefused
	if unansweredConfig != "" {
		unanswered, err = parseUnansweredDisposition(unansweredConfig)
		if err != nil {
			return nil, err
		}
	} else if resolverURL.RawQuery != "" {
		values := resolverURL.Query()
		if raw := strings.TrimSpace(values.Get("response")); raw != "" {
			unanswered, err = parseUnansweredDisposition(raw)
			if err != nil {
				return nil, err
			}
		} else if raw := strings.TrimSpace(values.Get("unanswerable")); raw != "" {
			unanswered, err = parseUnansweredDisposition(raw)
			if err != nil {
				return nil, err
			}
		} else {
			for key := range values {
				if disp, found := unansweredKeywords[strings.ToLower(key)]; found {
					unanswered = disp
					break
				}
			}
		}
	}

	switch resolverURL.Scheme {
	case "", "file":
		return NewRefCountingResolver(NewHostFile(resolverURL.Path)), nil
	case "udp":
		dnsResolver, err := NewDnsServer(resolverURL.Host, upstreamConfig, unanswered)
		if err != nil {
			return nil, err
		}
		return NewRefCountingResolver(dnsResolver), nil
	}

	return nil, fmt.Errorf("invalid resolver configuration '%s'. must be 'file://' or 'udp://' URL", config)
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

	_ = r.RemoveHostname(resolverTestHostname)
	return nil
}

func (r *resolver) LookupIP(name string) (net.IP, bool) {
	r.namesMtx.Lock()
	defer r.namesMtx.Unlock()
	canonical := strings.ToLower(name)
	ip, found := r.names[canonical]
	return ip, found
}

func (r *resolver) getAddress(name string) (net.IP, error) {
	a, ok := r.LookupIP(name)
	if ok {
		return a, nil
	}

	canonical := strings.ToLower(name)

	r.domainsMtx.Lock()
	defer r.domainsMtx.Unlock()
	for {
		idx := strings.IndexByte(canonical[1:], '.')
		if idx < 0 {
			break
		}
		canonical = canonical[idx+1:]

		de, ok := r.domains[canonical]

		if ok {
			name = name[:len(name)-1]
			ip, err := de.getIP(name)
			if err != nil {
				return nil, err
			}
			log.Debugf("assigned %v => %v", name, ip)
			_ = r.AddHostname(name, ip) // this resolver impl never returns an error
			return ip, err
		}
	}

	return nil, errors.New("not found")
}

func (r *resolver) queryUpstream(query *dns.Msg) (*dns.Msg, error) {
	if r.upstreamServer == "" || r.upstreamClient == nil {
		return nil, errors.New("no upstream server configured")
	}

	log.Debugf("forwarding query to upstream server %s: %s", r.upstreamServer, query.Question[0].Name)
	
	response, _, err := r.upstreamClient.Exchange(query, r.upstreamServer)
	if err != nil {
		log.Warnf("upstream query failed: %v", err)
		return nil, err
	}

	log.Debugf("received response from upstream server: %d answers", len(response.Answer))
	return response, nil
}

func (r *resolver) ServeDNS(w dns.ResponseWriter, query *dns.Msg) {
	log.Tracef("received:\n%s\n", query.String())
	msg := dns.Msg{}
	msg.SetReply(query)
	msg.RecursionAvailable = r.upstreamServer != ""
	q := query.Question[0]
	switch q.Qtype {
	case dns.TypeA:
		name := q.Name
		address, err := r.getAddress(name)
		if err == nil {
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			answer := &dns.A{
				Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   address,
			}
			msg.Answer = append(msg.Answer, answer)
			log.Tracef("response:\n%s\n", msg.String())
			err := w.WriteMsg(&msg)
			if err != nil {
				log.Errorf("write failed: %s", err)
			}
			return
		}
		if r.upstreamServer != "" {
			if upstreamResp, err := r.queryUpstream(query); err == nil {
				err := w.WriteMsg(upstreamResp)
				if err != nil {
					log.Errorf("write failed: %s", err)
				}
				return
			}
		}
		// No local match or upstream failed
		r.handleUnanswerable(w, query)
		return
	case dns.TypeAAAA:
		if _, err := r.getAddress(q.Name); err == nil {
			msg.Authoritative = true
			msg.Rcode = dns.RcodeSuccess
			log.Tracef("response:\n%s\n", msg.String())
			if err := w.WriteMsg(&msg); err != nil {
				log.Errorf("write failed: %s", err)
			}
			return
		}

		if r.upstreamServer != "" {
			if upstreamResp, err := r.queryUpstream(query); err == nil {
				if err := w.WriteMsg(upstreamResp); err != nil {
					log.Errorf("write failed: %s", err)
				}
				return
			}
		}

		r.handleUnanswerable(w, query)
		return
	}

	r.handleUnanswerable(w, query)
}

func (r *resolver) handleUnanswerable(w dns.ResponseWriter, query *dns.Msg) {
	switch r.unanswered {
	case unansweredTimeout:
		log.Tracef("unanswerable query for %s: handling with timeout (no response)", query.Question[0].Name)
		return
	case unansweredServfail:
		log.Tracef("unanswerable query for %s: responding with SERVFAIL", query.Question[0].Name)
		resp := dns.Msg{}
		resp.SetReply(query)
		resp.RecursionAvailable = r.upstreamServer != ""
		resp.Rcode = dns.RcodeServerFailure
		if err := w.WriteMsg(&resp); err != nil {
			log.Errorf("write failed: %s", err)
		}
	default:
		log.Tracef("unanswerable query for %s: responding with REFUSED", query.Question[0].Name)
		resp := dns.Msg{}
		resp.SetReply(query)
		resp.RecursionAvailable = r.upstreamServer != ""
		resp.Rcode = dns.RcodeRefused
		if err := w.WriteMsg(&resp); err != nil {
			log.Errorf("write failed: %s", err)
		}
	}
}

func (r *resolver) AddDomain(name string, ipCB func(string) (net.IP, error)) error {
	if name[0] != '*' {
		return fmt.Errorf("invalid wildcard domain")
	}
	domainSfx := name[1:] + "."
	entry := &domainEntry{
		name:  name,
		getIP: ipCB,
	}

	r.domainsMtx.Lock()
	defer r.domainsMtx.Unlock()
	if _, found := r.domains[domainSfx]; found {
		log.Warnf("domain[%v] is overwriting registered domain", name)
	}
	r.domains[domainSfx] = entry

	return nil
}

func (r *resolver) RemoveDomain(name string) {
	if name[0] != '*' {
		log.Warnf("invalid wildcard domain '%s'", name)
		return
	}
	domainSfx := name[1:] + "."
	r.domainsMtx.Lock()
	defer r.domainsMtx.Unlock()
	log.Infof("removing domain %s from resolver", domainSfx)
	delete(r.domains, domainSfx)
}

func (r *resolver) AddHostname(hostname string, ip net.IP) error {
	r.namesMtx.Lock()
	defer r.namesMtx.Unlock()

	canonical := strings.ToLower(hostname) + "."
	if _, found := r.names[canonical]; !found {
		log.Infof("adding %s = %s to resolver", hostname, ip.String())
		r.names[canonical] = ip
		r.ips[ip.String()] = canonical[0 : len(canonical)-1] // drop the dot
	} else {
		log.Infof("hostname %s already assigned (%s)", hostname, r.names[canonical])
	}

	return nil
}

func (r *resolver) Lookup(ip net.IP) (string, error) {
	if ip == nil {
		return "", errors.New("illegal argument")
	}
	key := ip.String()
	name, found := r.ips[key]
	if found {
		return name, nil
	}

	return "", errors.New("not found")
}

func (r *resolver) RemoveHostname(hostname string) net.IP {
	r.namesMtx.Lock()
	defer r.namesMtx.Unlock()

	key := strings.ToLower(hostname) + "."
	var ip net.IP
	var ok bool
	if ip, ok = r.names[key]; ok {
		log.Infof("removing %s from resolver", hostname)
		delete(r.ips, ip.String())
		delete(r.names, key)
	}

	return ip
}

func (r *resolver) Cleanup() error {
	log.Debug("shutting down")
	return r.server.Shutdown()
}
