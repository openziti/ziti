package intercept

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"io"
	"net"
	"strings"
	"time"
)

// record type
const (
	A   = 1
	MX  = 15
	TXT = 16
	SRV = 33
)

// response code
const (
	NOERROR  = 0
	FORMERR  = 1
	SERVFAIL = 2
	NXDOMAIN = 3
	NOTIMP   = 4
	REFUSED  = 5
)

type DnsQuestion struct {
	Name string
	Type int
}

type DnsAnswer struct {
	Name string
	Type int
	TTL int `json:"TTL"`
	Data string
}

type DnsMessage struct {
	Status int
	Question []*DnsQuestion
	Answer []*DnsAnswer
	Comment string `json:",omitempty"`
}

type resolvConn struct {
	ctx *hostingContext

	respQueue chan *DnsMessage
	closed bool
}

func newResolvConn(hostCtx *hostingContext) (net.Conn, bool, error) {
	log := pfxlog.Logger().WithField("service", hostCtx.service.Name)
	log.Infof("starting resolver connection")
	r := &resolvConn{ctx: hostCtx, respQueue: make(chan *DnsMessage, 16)}
	return r, false, nil
}

func (r *resolvConn) Read(b []byte) (n int, err error) {

	msg, ok := <- r.respQueue
	if !ok {
		return 0, io.EOF
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return 0, err
	}
	if len(msgBytes) > len(b) {
		return 0, errors.New("short buffer")
	}
	return copy(b, msgBytes), nil
}

func (r *resolvConn) Write(b []byte) (int, error) {
	resp := &DnsMessage{}
	var q DnsQuestion
	var matchName string
	dnsMatch := false

	err := json.Unmarshal(b, &q)
	if err != nil {
		resp.Status = FORMERR
		goto  done
	}

	resp.Question = []*DnsQuestion{&q}

	matchName = q.Name
	if strings.HasSuffix(matchName, ".") {
		matchName = matchName[0:len(matchName) - 1]
	}
	for _, allowed := range r.ctx.config.GetAllowedAddresses() {
		if allowed.Allows(matchName) {
			dnsMatch = true
			break
		}
	}

	if !dnsMatch {
		resp.Status = NXDOMAIN
		goto done
	}

	switch q.Type {
	case SRV:
		_, srvs, err := net.LookupSRV("","", q.Name)
		if err != nil {
			resp.Comment = err.Error()
			resp.Status = SERVFAIL
			goto done
		}

		resp.Status = NOERROR
		for _, srv := range srvs {
			ans := &DnsAnswer{
				Name: q.Name,
				Type: q.Type,
				Data: fmt.Sprintf("%d %d %d %s", srv.Priority, srv.Weight, srv.Port, srv.Target),
				TTL: 86400,
			}
			resp.Answer = append(resp.Answer, ans)
		}

	case TXT:
		txts, err := net.LookupTXT(q.Name)
		if err != nil {
			resp.Comment = err.Error()
			resp.Status = SERVFAIL
			goto done
		}
		resp.Status = NOERROR
		for _, txt := range txts {
			ans := &DnsAnswer{
				Name: q.Name,
				Type: q.Type,
				Data: txt,
				TTL: 86400,
			}
			resp.Answer = append(resp.Answer, ans)
		}

	case MX:
		mxs, err := net.LookupMX(q.Name)
		if err != nil {
			resp.Comment = err.Error()
			resp.Status = SERVFAIL
			goto done
		}
		resp.Status = NOERROR
		for _, mx := range mxs {
			ans := &DnsAnswer{
				Name: q.Name,
				Type: q.Type,
				Data: fmt.Sprintf("%d %s", mx.Pref, mx.Host),
				TTL: 86400,
			}
			resp.Answer = append(resp.Answer, ans)
		}

	default:
		resp.Status = NOTIMP
	}

done:
	r.respQueue <- resp
	return len(b), nil
}

func (r *resolvConn) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	log := pfxlog.Logger().WithField("service", r.ctx.ServiceName())
	log.Infof("resolver connection closed")
	close(r.respQueue)
	return nil
}

func (r *resolvConn) LocalAddr() net.Addr {
	return nil
}

func (r *resolvConn) RemoteAddr() net.Addr {
	return nil
}

func (r *resolvConn) SetDeadline(t time.Time) error {
	pfxlog.Logger().Warn("should not be here")
	return nil
}

func (r *resolvConn) SetReadDeadline(t time.Time) error {
	pfxlog.Logger().Warn("should not be here")
	return nil
}

func (r *resolvConn) SetWriteDeadline(t time.Time) error {
	pfxlog.Logger().Warn("should not be here")
	return nil
}

