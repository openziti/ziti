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
	upstream := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.1")
	})
	r := makeResolver(upstream.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, dns.RcodeSuccess, resp.Rcode)
	require.Len(t, resp.Answer, 1)
	require.Equal(t, "10.0.0.1", resp.Answer[0].(*dns.A).A.String())
}

func TestQueryUpstreams_FastestNoerrorWins(t *testing.T) {
	upstreamSlow := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(500 * time.Millisecond)
		return responseA(q, "10.0.0.1")
	})
	upstreamFast := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseA(q, "10.0.0.2")
	})
	upstreamMid := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(200 * time.Millisecond)
		return responseA(q, "10.0.0.3")
	})
	r := makeResolver(upstreamSlow.addr, upstreamFast.addr, upstreamMid.addr)

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
	upstream1 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return responseRcode(q, dns.RcodeNameError) })
	upstream2 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return responseRcode(q, dns.RcodeNameError) })
	r := makeResolver(upstream1.addr, upstream2.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeNameError, resp.Rcode)
}

func TestQueryUpstreams_NXDOMAINBeatsSERVFAIL(t *testing.T) {
	upstreamNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(50 * time.Millisecond)
		return responseRcode(q, dns.RcodeNameError)
	})
	upstreamServfail := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseRcode(q, dns.RcodeServerFailure)
	})
	r := makeResolver(upstreamNx.addr, upstreamServfail.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeNameError, resp.Rcode)
}

// NOERROR with empty answer (NODATA) still wins over NXDOMAIN.
func TestQueryUpstreams_NoerrorEmptyBeatsNXDOMAIN(t *testing.T) {
	upstreamNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseRcode(q, dns.RcodeNameError)
	})
	upstreamEmpty := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(100 * time.Millisecond)
		return responseRcode(q, dns.RcodeSuccess)
	})
	r := makeResolver(upstreamNx.addr, upstreamEmpty.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, resp.Rcode)
	require.Empty(t, resp.Answer)
}

func TestQueryUpstreams_OneTransportFailureOneSuccess(t *testing.T) {
	upstreamDrop := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	upstreamOk := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.77")
	})
	r := makeResolver(upstreamDrop.addr, upstreamOk.addr)
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, "10.0.0.77", resp.Answer[0].(*dns.A).A.String())
}

func TestQueryUpstreams_AllTransportFailures(t *testing.T) {
	upstream1 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	upstream2 := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	r := makeResolver(upstream1.addr, upstream2.addr)
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

func TestParseUpstreamMode(t *testing.T) {
	cases := map[string]upstreamMode{
		"":         upstreamParallel,
		"parallel": upstreamParallel,
		"serial":   upstreamSerial,
		"failover": upstreamFailover,
		"random":   upstreamRandom,
		"SERIAL":   upstreamSerial,
		" serial ": upstreamSerial,
	}
	for raw, expected := range cases {
		mode, err := parseUpstreamMode(raw)
		require.NoError(t, err, "raw=%q", raw)
		require.Equal(t, expected, mode, "raw=%q", raw)
	}

	_, err := parseUpstreamMode("bogus")
	require.Error(t, err)
}

func TestValidateUpstreamMode(t *testing.T) {
	require.NoError(t, ValidateUpstreamMode(""))
	require.NoError(t, ValidateUpstreamMode("serial"))
	require.NoError(t, ValidateUpstreamMode("FAILOVER"))
	require.Error(t, ValidateUpstreamMode("bogus"))
}

func TestValidateUnansweredDisposition(t *testing.T) {
	require.NoError(t, ValidateUnansweredDisposition(""))
	require.NoError(t, ValidateUnansweredDisposition("servfail"))
	require.NoError(t, ValidateUnansweredDisposition("REFUSED"))
	require.Error(t, ValidateUnansweredDisposition("bogus"))
}

// In serial mode a healthy first upstream answers every query, and the
// remaining upstreams are never contacted (no fan-out traffic).
func TestQueryUpstreams_SerialFirstUpstreamWinsNoFanout(t *testing.T) {
	upstreamFirst := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.1")
	})
	upstreamSecond := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.2")
	})
	r := makeResolver(upstreamFirst.addr, upstreamSecond.addr)
	r.upstreamMode = upstreamSerial

	for i := 0; i < 3; i++ {
		resp, err := r.queryUpstreams(newQuery("example.com"))
		require.NoError(t, err)
		require.Equal(t, "10.0.0.1", resp.Answer[0].(*dns.A).A.String())
	}

	require.Equal(t, int32(3), upstreamFirst.queries.Load())
	require.Equal(t, int32(0), upstreamSecond.queries.Load(), "second upstream must not be queried while the first answers")
}

// In serial mode a transport failure on the first upstream fails through to the
// next, which answers.
func TestQueryUpstreams_SerialFailsThroughOnError(t *testing.T) {
	upstreamDrop := startTestUpstream(t, func(q *dns.Msg) *dns.Msg { return nil })
	upstreamOk := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.55")
	})
	r := makeResolver(upstreamDrop.addr, upstreamOk.addr)
	r.upstreamMode = upstreamSerial

	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, "10.0.0.55", resp.Answer[0].(*dns.A).A.String())
	require.Equal(t, int32(1), upstreamDrop.queries.Load())
	require.Equal(t, int32(1), upstreamOk.queries.Load())
}

// In serial mode a non-NOERROR response fails through to the next upstream
// rather than being returned, even though that response would win a parallel
// rcode-ranked comparison.
func TestQueryUpstreams_SerialFailsThroughOnNXDOMAIN(t *testing.T) {
	upstreamNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseRcode(q, dns.RcodeNameError)
	})
	upstreamOk := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.66")
	})
	r := makeResolver(upstreamNx.addr, upstreamOk.addr)
	r.upstreamMode = upstreamSerial

	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeSuccess, resp.Rcode)
	require.Equal(t, "10.0.0.66", resp.Answer[0].(*dns.A).A.String())
}

// When no upstream returns NOERROR, serial mode returns the best-ranked
// non-NOERROR response, matching the parallel path's selection.
func TestQueryUpstreams_SerialBestRcodeWhenNoNoerror(t *testing.T) {
	upstreamServfail := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseRcode(q, dns.RcodeServerFailure)
	})
	upstreamNx := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseRcode(q, dns.RcodeNameError)
	})
	r := makeResolver(upstreamServfail.addr, upstreamNx.addr)
	r.upstreamMode = upstreamSerial

	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, dns.RcodeNameError, resp.Rcode)
}

// In failover mode, once the first upstream stops answering the resolver sticks
// to the upstream that answered and stops re-probing the dead one on every query.
func TestQueryUpstreams_FailoverSticksToHealthyUpstream(t *testing.T) {
	var firstUp atomic.Bool
	firstUp.Store(true)
	upstreamFirst := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		if firstUp.Load() {
			return responseA(q, "10.0.0.1")
		}
		return nil // simulate the first upstream going down
	})
	upstreamSecond := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.2")
	})
	r := makeResolver(upstreamFirst.addr, upstreamSecond.addr)
	r.upstreamMode = upstreamFailover

	// first query: first upstream answers, preferred stays at index 0
	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, "10.0.0.1", resp.Answer[0].(*dns.A).A.String())

	// first upstream goes down; next query fails through to the second, which
	// becomes the new preferred upstream
	firstUp.Store(false)
	resp, err = r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, "10.0.0.2", resp.Answer[0].(*dns.A).A.String())

	firstQueriesAfterFailover := upstreamFirst.queries.Load()

	// subsequent queries go straight to the second upstream without re-probing
	// the dead first upstream
	for i := 0; i < 3; i++ {
		resp, err = r.queryUpstreams(newQuery("example.com"))
		require.NoError(t, err)
		require.Equal(t, "10.0.0.2", resp.Answer[0].(*dns.A).A.String())
	}
	require.Equal(t, firstQueriesAfterFailover, upstreamFirst.queries.Load(), "dead upstream must not be re-probed once failover settles")
	// second upstream answered the failover query plus the 3 follow-ups (the
	// very first query was served by the then-healthy first upstream).
	require.Equal(t, int32(4), upstreamSecond.queries.Load())
}

// Random mode still returns a NOERROR answer; with a single upstream the start
// index is deterministic.
func TestQueryUpstreams_RandomSingleUpstream(t *testing.T) {
	upstream := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		return responseA(q, "10.0.0.9")
	})
	r := makeResolver(upstream.addr)
	r.upstreamMode = upstreamRandom

	resp, err := r.queryUpstreams(newQuery("example.com"))
	require.NoError(t, err)
	require.Equal(t, "10.0.0.9", resp.Answer[0].(*dns.A).A.String())
}

// When a fast upstream produces NOERROR, queryUpstreams returns without
// waiting for slower upstreams — the caller must not be blocked by them.
func TestQueryUpstreams_FastPathDoesNotWaitForSlow(t *testing.T) {
	upstreamFast := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(10 * time.Millisecond)
		return responseA(q, "10.0.0.1")
	})
	upstreamSlow := startTestUpstream(t, func(q *dns.Msg) *dns.Msg {
		time.Sleep(500 * time.Millisecond)
		return responseA(q, "10.0.0.2")
	})
	r := makeResolver(upstreamSlow.addr, upstreamFast.addr)

	start := time.Now()
	resp, err := r.queryUpstreams(newQuery("example.com"))
	elapsed := time.Since(start)
	require.NoError(t, err)
	require.Equal(t, "10.0.0.1", resp.Answer[0].(*dns.A).A.String())
	require.Less(t, elapsed, 150*time.Millisecond, "fast path must not wait for slow upstream (elapsed=%s)", elapsed)
}
