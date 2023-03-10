package xlink_transport

import (
	"crypto/x509"
	"fmt"
	"github.com/openziti/channel/v2"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2"
	"github.com/stretchr/testify/assert"
	"net"
	"testing"
	"time"
)

type testUnderlay struct {
}

func (t testUnderlay) Rx() (*channel.Message, error) {
	time.Sleep(time.Hour)
	return nil, nil
}

func (t testUnderlay) Tx(*channel.Message) error {
	time.Sleep(10 * time.Microsecond)
	return nil
}

func (t testUnderlay) Id() string {
	return "test"
}

func (t testUnderlay) LogicalName() string {
	return "test"
}

func (t testUnderlay) ConnectionId() string {
	return "test"
}

func (t testUnderlay) Certificates() []*x509.Certificate {
	return nil
}

func (t testUnderlay) Label() string {
	return "test"
}

func (t testUnderlay) Close() error {
	return nil
}

func (t testUnderlay) IsClosed() bool {
	return false
}

func (t testUnderlay) Headers() map[int32][]byte {
	return nil
}

func (t testUnderlay) SetWriteTimeout(time.Duration) error {
	return nil
}

func (t testUnderlay) SetWriteDeadline(time.Time) error {
	return nil
}

func (t testUnderlay) GetLocalAddr() net.Addr {
	panic("implement me")
}

func (t testUnderlay) GetRemoteAddr() net.Addr {
	panic("implement me")
}

type testUnderlayFactory struct {
	underlay testUnderlay
}

func (t testUnderlayFactory) Create(time.Duration, transport.Configuration) (channel.Underlay, error) {
	return t.underlay, nil
}

func Test_Throughput(t *testing.T) {
	factory := testUnderlayFactory{
		underlay: testUnderlay{},
	}

	t.SkipNow()

	options := channel.DefaultOptions()
	options.OutQueueSize = 64
	ch, err := channel.NewChannel("test", factory, nil, options)
	assert.NoError(t, err)

	registry := metrics.NewRegistry("test", nil)
	drops := registry.Meter("drops")
	msgs := registry.Meter("msgs")

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			v := registry.Poll().Meters["drops"]
			fmt.Printf("drops - m1: %v, count: %v\n", v.M1Rate, v.Count)
			v = registry.Poll().Meters["msgs"]
			fmt.Printf("msgs  - m1: %v, count: %v\n", v.M1Rate, v.Count)
		}
	}()

	go func() {
		for {
			m := channel.NewMessage(1, nil)
			sent, err := ch.TrySend(m)
			assert.NoError(t, err)
			if !sent {
				drops.Mark(1)
			}
			msgs.Mark(1)
			time.Sleep(10 * time.Microsecond)
		}
	}()

	time.Sleep(time.Minute)
}
