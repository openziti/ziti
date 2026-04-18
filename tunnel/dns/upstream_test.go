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
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

// testUpstream is a miekg/dns server on an ephemeral UDP port used to exercise
// the resolver's queryUpstreams fan-out over a real socket.
type testUpstream struct {
	addr    string
	server  *dns.Server
	queries atomic.Int32
}

// startTestUpstream starts a DNS test server on 127.0.0.1 at an ephemeral port.
// The handler receives each query and returns a response; returning nil drops
// the query to simulate a transport-level failure.
func startTestUpstream(t *testing.T, handler func(q *dns.Msg) *dns.Msg) *testUpstream {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)

	tu := &testUpstream{addr: pc.LocalAddr().String()}
	started := make(chan struct{})
	srv := &dns.Server{
		PacketConn:        pc,
		NotifyStartedFunc: func() { close(started) },
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
			tu.queries.Add(1)
			resp := handler(q)
			if resp == nil {
				return
			}
			_ = w.WriteMsg(resp)
		}),
	}
	tu.server = srv
	go func() {
		_ = srv.ActivateAndServe()
	}()
	<-started
	t.Cleanup(func() { _ = srv.Shutdown() })
	return tu
}

// makeResolver builds a resolver with the given upstream addresses. A 1s
// per-client timeout keeps transport-failure cases snappy.
func makeResolver(addrs ...string) *resolver {
	r := &resolver{}
	for _, addr := range addrs {
		r.upstreams = append(r.upstreams, upstream{
			server: addr,
			client: &dns.Client{Net: "udp", Timeout: time.Second},
		})
	}
	return r
}

func newQuery(name string) *dns.Msg {
	m := &dns.Msg{}
	m.SetQuestion(name+".", dns.TypeA)
	return m
}

func responseA(q *dns.Msg, ip string) *dns.Msg {
	m := &dns.Msg{}
	m.SetReply(q)
	m.Rcode = dns.RcodeSuccess
	m.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   net.ParseIP(ip).To4(),
		},
	}
	return m
}

func responseRcode(q *dns.Msg, rcode int) *dns.Msg {
	m := &dns.Msg{}
	m.SetReply(q)
	m.Rcode = rcode
	return m
}

func TestQueryUpstreams_SingleUpstream(t *testing.T) {
	up := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.1")
	})
	r := makeResolver(up.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, dns.RcodeSuccess, resp.Rcode)
	require.Len(t, resp.Answer, 1)
	require.Equal(t, "10.0.0.1", resp.Answer[0].(*dns.A).A.String())
}

func TestQueryUpstreams_FastestNoerrorWins(t *testing.T) {
	upSlow := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(500 * time.Millisecond)
		return responseA(q, "10.0.0.1")
	})
	upFast := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseA(q, "10.0.0.2")
	})
	upMid := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(200 * time.Millisecond)
		return responseA(q, "10.0.0.3")
	})
	r := makeResolver(upSlow.addr, upFast.addr, upMid.addr)

	start := time.Now()
	resp, err := r.queryUpstreams(newQuery("example.com"))
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Equal(t, "10.0.0.2", resp.Answer[0].(*dns.A).A.String())
	require.Less(t, elapsed, 150*time.Millisecond, "expected fast upstream to win (elapsed=%s)", elapsed)
}

// Motivating split-horizon case: a fast NXDOMAIN from one upstream must not
// beat a slower NOERROR answer from another.
func TestQueryUpstreams_SlowNoerrorBeatsFastNXDOMAIN(t *testing.T) {
	upFastNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseRcode(q, dns.RcodeNameError)
	})
	upSlowOk := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(200 * time.Millisecond)
		return responseA(q, "10.0.0.42")
	})
	r := makeResolver(upFastNx.addr, upSlowOk.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, resp.Rcode)
	require.Len(t, resp.Answer, 1)
	require.Equal(t, "10.0.0.42", resp.Answer[0].(*dns.A).A.String())
}

func TestQueryUpstreams_AllNXDOMAIN(t *testing.T) {
	u1 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return responseRcode(q, dns.RcodeNameError) })
	u2 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return responseRcode(q, dns.RcodeNameError) })
	r := makeResolver(u1.addr, u2.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeNameError, resp.Rcode)
}

func TestQueryUpstreams_NXDOMAINBeatsSERVFAIL(t *testing.T) {
	uNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(50 * time.Millisecond)
		return responseRcode(q, dns.RcodeNameError)
	})
	uServfail := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseRcode(q, dns.RcodeServerFailure)
	})
	r := makeResolver(uNx.addr, uServfail.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeNameError, resp.Rcode)
}

// NOERROR with empty answer (NODATA) still wins over NXDOMAIN.
func TestQueryUpstreams_NoerrorEmptyBeatsNXDOMAIN(t *testing.T) {
	uNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseRcode(q, dns.RcodeNameError)
	})
	uEmpty := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(100 * time.Millisecond)
		return responseRcode(q, dns.RcodeSuccess)
	})
	r := makeResolver(uNx.addr, uEmpty.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, resp.Rcode)
	require.Empty(t, resp.Answer)
}

func TestQueryUpstreams_OneTransportFailureOneSuccess(t *testing.T) {
	uDrop := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	uOk := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.77")
	})
	r := makeResolver(uDrop.addr, uOk.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, "10.0.0.77", resp.Answer[0].(*dns.A).A.String())
}

func TestQueryUpstreams_AllTransportFailures(t *testing.T) {
	u1 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	u2 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	r := makeResolver(u1.addr, u2.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.Error(t, err)
	require.Nil(t, resp)
}

func TestQueryUpstreams_EmptyList(t *testing.T) {
	r := &resolver{}
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "no upstream")
}

// When a fast upstream produces NOERROR, queryUpstreams returns without
// waiting for slower upstreams — the caller must not be blocked by them.
func TestQueryUpstreams_FastPathDoesNotWaitForSlow(t *testing.T) {
	uFast := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseA(q, "10.0.0.1")
	})
	uSlow := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(500 * time.Millisecond)
		return responseA(q, "10.0.0.2")
	})
	r := makeResolver(uSlow.addr, uFast.addr)

	start := time.Now()
	resp, err := r.queryUpstreams(newQuery("example.com"))
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Equal(t, "10.0.0.1", resp.Answer[0].(*dns.A).A.String())
	require.Less(t, elapsed, 150*time.Millisecond, "fast path must not wait for slow upstream (elapsed=%s)", elapsed)
}
