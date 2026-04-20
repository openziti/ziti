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
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/miekg/dns"
	"github.com/sirupsen/logrus"
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

// upstream represents a single upstream DNS server the resolver can forward to.
type upstream struct {
	server string
	client *dns.Client
}

type resolver struct {
	server     *dns.Server
	names      map[string]net.IP
	ips        map[string]string
	namesMtx   sync.Mutex
	domains    map[string]*domainEntry
	domainsMtx sync.Mutex
	upstreams  []upstream
	unanswered unansweredDisposition
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

// NewResolver constructs a Resolver from a listener URL, zero or more upstream URLs,
// and an unanswered-query disposition. An empty upstreams slice disables upstream
// forwarding. When multiple upstreams are provided, queries are fanned out in
// parallel and the first NOERROR response wins (see queryUpstreams).
func NewResolver(config string, upstreams []string, unansweredConfig string) (Resolver, error) {
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
		dnsResolver, err := NewDnsServer(resolverURL.Host, upstreams, unanswered)
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

// rcodeScore ranks non-NOERROR DNS response codes. Higher is better.
// NXDOMAIN is an authoritative "doesn't exist" and is most useful to the
// caller, followed by SERVFAIL (server problem) and REFUSED (policy).
func rcodeScore(rcode int) int {
	switch rcode {
	case dns.RcodeNameError:
		return 3
	case dns.RcodeServerFailure:
		return 2
	case dns.RcodeRefused:
		return 1
	default:
		return 0
	}
}

// queryUpstreams forwards the query to all configured upstreams concurrently
// and selects a winner based on response code:
//   - the first NOERROR response is returned immediately;
//   - otherwise, after all upstreams have returned, the best-ranked
//     non-NOERROR response wins (NXDOMAIN > SERVFAIL > REFUSED > other);
//   - if every upstream errored at the transport layer, the last error
//     is returned so the caller can fall through to handleUnanswerable.
//
// When the fast path returns early, goroutines for slower upstreams remain
// running until their per-client Timeout fires. The results channel is
// buffered so these late writes don't block. This is a bounded wait, not a
// leak — miekg/dns ExchangeContext honors ctx.Deadline but not ctx.Done,
// so there's no mechanism to abort an in-flight UDP read sooner.
func (r *resolver) queryUpstreams(query *dns.Msg) (*dns.Msg, error) {
	if len(r.upstreams) == 0 {
		return nil, errors.New("no upstream server configured")
	}

	type result struct {
		resp *dns.Msg
		err  error
		from string
	}

	results := make(chan result, len(r.upstreams))
	for _, u := range r.upstreams {
		u := u
		go func() {
			log.Debugf("forwarding query to upstream server %s: %s", u.server, query.Question[0].Name)
			resp, _, err := u.client.Exchange(query, u.server)
			results <- result{resp: resp, err: err, from: u.server}
		}()
	}

	var bestResp *dns.Msg
	bestScore := -1
	var lastErr error
	for i := 0; i < len(r.upstreams); i++ {
		res := <-results
		if res.err != nil || res.resp == nil {
			if res.err != nil {
				log.Warnf("upstream query to %s failed: %v", res.from, res.err)
				lastErr = res.err
			}
			continue
		}
		if res.resp.Rcode == dns.RcodeSuccess {
			log.Debugf("received NOERROR from %s: %d answers", res.from, len(res.resp.Answer))
			return res.resp, nil
		}
		if s := rcodeScore(res.resp.Rcode); s > bestScore {
			bestScore = s
			bestResp = res.resp
		}
	}

	if bestResp != nil {
		return bestResp, nil
	}
	return nil, lastErr
}

func (r *resolver) ServeDNS(w dns.ResponseWriter, query *dns.Msg) {
	log.Tracef("received:\n%s\n", query.String())
	msg := dns.Msg{}
	msg.SetReply(query)
	msg.RecursionAvailable = len(r.upstreams) > 0
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
		if len(r.upstreams) > 0 {
			if upstreamResp, err := r.queryUpstreams(query); err == nil {
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

		if len(r.upstreams) > 0 {
			if upstreamResp, err := r.queryUpstreams(query); err == nil {
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
		resp.RecursionAvailable = len(r.upstreams) > 0
		resp.Rcode = dns.RcodeServerFailure
		if err := w.WriteMsg(&resp); err != nil {
			log.Errorf("write failed: %s", err)
		}
	default:
		log.Tracef("unanswerable query for %s: responding with REFUSED", query.Question[0].Name)
		resp := dns.Msg{}
		resp.SetReply(query)
		resp.RecursionAvailable = len(r.upstreams) > 0
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
