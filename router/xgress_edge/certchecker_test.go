package xgress_edge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/openziti/channel/v2"
	"github.com/openziti/foundation/v2/tlz"
	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/internal/edgerouter"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func Test_CertExpirationChecker(t *testing.T) {
	t.Run("getWaitTime", func(t *testing.T) {
		t.Run("both 30d out is 23d", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.Add(30 * time.Hour * 24).Add(30 * time.Second)

			minWaitTime := 23 * 24 * time.Hour          // 23 days out i.e. 1 week before 30 days
			maxWaitTime := minWaitTime + 30*time.Second // 23 days + 30s out i.e. 1 week before 30 days

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.GreaterOrEqual(waitTime, minWaitTime)
			req.LessOrEqual(waitTime, maxWaitTime)
		})

		t.Run("both 7d out is 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now().UTC()
			notAfter := now.AddDate(0, 0, 7)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both 4d out is 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 4)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both 1m out is 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.Add(1 * time.Minute)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both 0s out errors", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both -1s prior errors", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.Add(-1 * time.Second)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both -1d prior errors", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, -1)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both -1d prior errors", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, -1)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("client 5d prior to server, returns client wait time", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			serverNotAfter := now.Add(30 * time.Hour * 24)
			clientNotAfter := now.Add(25 * time.Hour * 24).Add(30 * time.Second)

			certChecker.id.Cert().Leaf.NotAfter = clientNotAfter
			certChecker.id.ServerCert()[0].Leaf.NotAfter = serverNotAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.LessOrEqual(waitTime, 18*24*time.Hour+30*time.Second)
			req.GreaterOrEqual(waitTime, 18*24*time.Hour)
		})

		t.Run("server -1d prior returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, -1)

			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("server 5d out returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 5)

			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("server 7d out returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now().UTC()
			notAfter := now.AddDate(0, 0, 7)

			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("server 7d30s out returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			now := time.Now()
			notAfter := now.Add(7 * 24 * time.Hour).Add(30 * time.Second)

			certChecker.id.ServerCert()[0].Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.GreaterOrEqual(waitTime, 20*time.Second)
			req.LessOrEqual(waitTime, 30*time.Second)
		})
	})

	t.Run("Run", func(t *testing.T) {

		t.Run("after wait invokes extendFunc", func(t *testing.T) {
			req := require.New(t)
			certChecker, closeF := newCertChecker()
			certChecker.timeoutDuration = 10 * time.Millisecond

			var invoked atomic.Bool

			extender := &stubExtender{
				done: func() error {
					invoked.Store(true)
					certChecker.id.Cert().Leaf.NotAfter = time.Now().AddDate(1, 0, 0)
					certChecker.id.ServerCert()[0].Leaf.NotAfter = time.Now().AddDate(1, 0, 0)
					return errors.New("test")
				},
			}
			certChecker.extender = extender

			//will trigger 0 wait duration
			certChecker.id.Cert().Leaf.NotAfter = time.Now().AddDate(0, 0, 1)

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(200 * time.Millisecond)

			req.True(invoked.Load())

			closeF()
		})

		t.Run("double run errors", func(t *testing.T) {
			req := require.New(t)
			certChecker, closeF := newCertChecker()

			certChecker.isRequesting.Store(true)

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(10 * time.Millisecond)

			err := certChecker.Run()
			req.Error(err)

			closeF()
		})

		t.Run("timeoutDuration clears isRequesting", func(t *testing.T) {
			req := require.New(t)
			certChecker, closeF := newCertChecker()
			certChecker.timeoutDuration = 10 * time.Millisecond

			certChecker.isRequesting.Store(true)

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(50 * time.Millisecond)

			req.False(certChecker.isRequesting.Load())

			closeF()
		})

		t.Run("certsUpdated channel clears isRequesting pre-run", func(t *testing.T) {
			req := require.New(t)
			certChecker, closeF := newCertChecker()

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(50 * time.Millisecond)

			certChecker.isRequesting.Store(true)
			certChecker.CertsUpdated()

			time.Sleep(50 * time.Millisecond)

			req.False(certChecker.isRequesting.Load())

			closeF()
		})

		t.Run("certsUpdated channel clears isRequesting post-run", func(t *testing.T) {
			req := require.New(t)
			certChecker, closeF := newCertChecker()

			certChecker.isRequesting.Store(true)

			go func() {
				_ = certChecker.Run()
			}()

			certChecker.CertsUpdated()

			time.Sleep(50 * time.Millisecond)

			req.False(certChecker.isRequesting.Load())

			closeF()
		})

		t.Run("client cert expired returns error", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			certChecker.id.Cert().Leaf.NotAfter = time.Now().AddDate(0, 0, -1)

			req.Error(certChecker.Run())
		})
	})

	t.Run("ExtendEnrollment", func(t *testing.T) {

		t.Run("errors if isRequesting = true", func(t *testing.T) {
			req := require.New(t)
			certChecker, _ := newCertChecker()

			certChecker.isRequesting.Store(true)

			err := certChecker.ExtendEnrollment()

			req.Error(err)
			req.True(certChecker.isRequesting.Load())
		})
	})
}

var _ identity.Identity = &SimpleTestIdentity{}

type SimpleTestIdentity struct {
	TlsCert             *tls.Certificate
	TlsServerCert       []*tls.Certificate
	CertPool            *x509.CertPool
	reloadCalled        bool
	setCertCalled       bool
	setServerCertCalled bool
}

func (s *SimpleTestIdentity) CaPool() *identity.CaPool {
	return nil
}

func (s *SimpleTestIdentity) WatchFiles() error {
	panic("implement me")
}

func (s *SimpleTestIdentity) StopWatchingFiles() {
	panic("implement me")
}

func (s *SimpleTestIdentity) Cert() *tls.Certificate {
	return s.TlsCert
}

func (s *SimpleTestIdentity) ServerCert() []*tls.Certificate {
	return s.TlsServerCert
}

func (s *SimpleTestIdentity) CA() *x509.CertPool {
	return s.CertPool
}

func (s *SimpleTestIdentity) ServerTLSConfig() *tls.Config {
	var certs []tls.Certificate

	for _, cert := range s.TlsServerCert {
		certs = append(certs, *cert)
	}

	return &tls.Config{
		Certificates: certs,
		RootCAs:      s.CertPool,
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tlz.GetMinTlsVersion(),
		CipherSuites: tlz.GetCipherSuites(),
	}
}

func (s *SimpleTestIdentity) ClientTLSConfig() *tls.Config {
	return &tls.Config{
		RootCAs:      s.CertPool,
		Certificates: []tls.Certificate{*s.TlsCert},
	}
}

func (s *SimpleTestIdentity) Reload() error {
	s.reloadCalled = true
	return nil
}

func (s *SimpleTestIdentity) SetCert(string) error {
	s.setCertCalled = true
	return nil
}

func (s *SimpleTestIdentity) SetServerCert(string) error {
	s.setServerCertCalled = true
	return nil
}

func (s *SimpleTestIdentity) GetConfig() *identity.Config {
	return nil
}

func newCertChecker() (*CertExpirationChecker, func()) {
	privateKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	var template = &x509.Certificate{
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		SerialNumber: big.NewInt(123456789),
		Subject: pkix.Name{
			Country:      []string{"US"},
			SerialNumber: "123456789",
			CommonName:   "test_" + eid.New(),
		},
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	clientRawCert, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)

	if err != nil {
		panic(err)
	}

	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	serverRawCert, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)

	if err != nil {
		panic(err)
	}

	clientCert, err := x509.ParseCertificate(clientRawCert)

	if err != nil {
		panic(err)
	}

	tlsClient := &tls.Certificate{
		Certificate: [][]byte{clientRawCert},
		PrivateKey:  privateKey,
		Leaf:        clientCert,
	}

	serverCert, err := x509.ParseCertificate(serverRawCert)

	if err != nil {
		panic(err)
	}

	tlsServer := &tls.Certificate{
		Certificate: [][]byte{serverRawCert},
		PrivateKey:  privateKey,
		Leaf:        serverCert,
	}

	caPool := x509.NewCertPool()

	testIdentity := &SimpleTestIdentity{
		TlsCert:             tlsClient,
		TlsServerCert:       []*tls.Certificate{tlsServer},
		CertPool:            caPool,
		reloadCalled:        false,
		setCertCalled:       false,
		setServerCertCalled: false,
	}

	testChannel := &simpleTestChannel{}
	closeNotify := make(chan struct{})

	id := &identity.TokenId{
		Identity: testIdentity,
		Token:    eid.New(),
		Data:     nil,
	}
	ctrlDialer := env.CtrlDialer(func(address transport.Address, bindHandler channel.BindHandler) error {
		return testChannel.Bind(bindHandler)
	})
	ctrls := env.NewNetworkControllers(time.Second, ctrlDialer, env.NewDefaultHeartbeatOptions())
	ctrls.UpdateControllerEndpoints([]string{"tls:localhost:6262"})
	start := time.Now()
	for {
		if ctrls.AnyCtrlChannel() != nil {
			break
		}
		if time.Since(start) > time.Second {
			panic("control channel not setup")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return NewCertExpirationChecker(id, &edgerouter.Config{}, ctrls, closeNotify), func() { close(closeNotify) }
}

type simpleTestUnderlay struct{}

func (s simpleTestUnderlay) Rx() (*channel.Message, error) {
	panic("implement me")
}

func (s simpleTestUnderlay) Tx(*channel.Message) error {
	panic("implement me")
}

func (s simpleTestUnderlay) Id() string {
	return "id-test"
}

func (s simpleTestUnderlay) LogicalName() string {
	return "logical-test"
}

func (s simpleTestUnderlay) ConnectionId() string {
	return "conn-test"
}

func (s simpleTestUnderlay) Certificates() []*x509.Certificate {
	panic("implement me")
}

func (s simpleTestUnderlay) Label() string {
	return "label-test"
}

func (s simpleTestUnderlay) Close() error {
	panic("implement me")
}

func (s simpleTestUnderlay) IsClosed() bool {
	return false
}

func (s simpleTestUnderlay) Headers() map[int32][]byte {
	v, err := versions.StdVersionEncDec.Encode(&versions.VersionInfo{
		Version:   "0.0.0",
		Revision:  "1",
		BuildDate: "2000-01-01",
		OS:        "linux",
		Arch:      "amd64",
	})
	if err != nil {
		panic(err)
	}
	return map[int32][]byte{
		channel.HelloVersionHeader: v,
	}
}

func (s simpleTestUnderlay) SetWriteTimeout(time.Duration) error {
	panic("implement me")
}

func (s simpleTestUnderlay) SetWriteDeadline(time.Time) error {
	panic("implement me")
}

func (s simpleTestUnderlay) GetLocalAddr() net.Addr {
	panic("implement me")
}

func (s simpleTestUnderlay) GetRemoteAddr() net.Addr {
	panic("implement me")
}

type simpleTestChannel struct {
	isClosed bool
}

func (ch *simpleTestChannel) Bind(h channel.BindHandler) error {
	return h.BindChannel(ch)
}

func (ch *simpleTestChannel) AddPeekHandler(channel.PeekHandler) {
}

func (ch *simpleTestChannel) AddTransformHandler(channel.TransformHandler) {
}

func (ch *simpleTestChannel) AddReceiveHandler(int32, channel.ReceiveHandler) {
}

func (ch *simpleTestChannel) AddReceiveHandlerF(int32, channel.ReceiveHandlerF) {
}

func (ch *simpleTestChannel) AddTypedReceiveHandler(channel.TypedReceiveHandler) {
}

func (ch *simpleTestChannel) AddErrorHandler(channel.ErrorHandler) {
}

func (ch *simpleTestChannel) AddCloseHandler(channel.CloseHandler) {
}

func (ch *simpleTestChannel) SetUserData(interface{}) {
}

func (ch *simpleTestChannel) GetUserData() interface{} {
	return nil
}

func (ch *simpleTestChannel) GetChannel() channel.Channel {
	return ch
}

func (ch *simpleTestChannel) TrySend(channel.Sendable) (bool, error) {
	panic("implement me")
}

func (ch *simpleTestChannel) Send(channel.Sendable) error {
	panic("implement me")
}

func (ch *simpleTestChannel) Underlay() channel.Underlay {
	return simpleTestUnderlay{}
}

func (ch *simpleTestChannel) StartRx() {
}

func (ch *simpleTestChannel) Id() string {
	return "test"
}

func (ch *simpleTestChannel) LogicalName() string {
	panic("implement LogicalName()")
}

func (ch *simpleTestChannel) ConnectionId() string {
	panic("implement ConnectionId()")
}

func (ch *simpleTestChannel) Certificates() []*x509.Certificate {
	panic("implement Certificates()")
}

func (ch *simpleTestChannel) Label() string {
	return "testchannel"
}

func (ch *simpleTestChannel) SetLogicalName(string) {
	panic("implement SetLogicalName")
}

func (ch *simpleTestChannel) Close() error {
	panic("implement Close")
}

func (ch *simpleTestChannel) IsClosed() bool {
	return ch.isClosed
}

func (ch *simpleTestChannel) GetTimeSinceLastRead() time.Duration {
	return 0
}

type stubExtender struct {
	isRequesting atomic.Bool
	done         func() error
}

func (s *stubExtender) IsRequestingCompareAndSwap(expected bool, value bool) bool {
	return s.isRequesting.CompareAndSwap(expected, value)
}

func (s *stubExtender) SetIsRequesting(value bool) {
	s.isRequesting.Store(value)
}

func (s *stubExtender) ExtendEnrollment() error {
	s.SetIsRequesting(true)

	if s.done != nil {
		return s.done()
	}

	return nil
}

func (s *stubExtender) IsRequesting() bool {
	return s.isRequesting.Load()
}
