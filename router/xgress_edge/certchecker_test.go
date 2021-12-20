package xgress_edge

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/router/internal/edgerouter"
	"github.com/openziti/foundation/channel2"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/openziti/foundation/util/tlz"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func Test_CertExpirationChecker(t *testing.T) {
	t.Run("getWaitTime", func(t *testing.T) {
		t.Run("both 30d out is 23d", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 30).Add(30 * time.Second) //30d 30s out

			minWaitTime := 23 * 24 * time.Hour              // 23 days out i.e. 1 week before 30 days
			maxWaitTime := 23*24*time.Hour + 30*time.Second // 23 days + 30s out i.e. 1 week before 30 days

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.GreaterOrEqual(waitTime, minWaitTime)
			req.LessOrEqual(waitTime, maxWaitTime)
		})

		t.Run("both 7d out is 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 7)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both 4d out is 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 4)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both 1m out is 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.Add(1 * time.Minute)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both 0s out errors", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both -1s prior errors", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.Add(-1 * time.Second)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both -1d prior errors", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, -1)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("both -1d prior errors", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, -1)

			certChecker.id.Cert().Leaf.NotAfter = notAfter
			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.Error(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("client 5d prior to server, returns client wait time", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			serverNotAfter := now.AddDate(0, 0, 30)
			clientNotAfter := now.AddDate(0, 0, 25).Add(30 * time.Second)

			certChecker.id.Cert().Leaf.NotAfter = clientNotAfter
			certChecker.id.ServerCert().Leaf.NotAfter = serverNotAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.LessOrEqual(waitTime, 18*24*time.Hour+30*time.Second)
			req.GreaterOrEqual(waitTime, 18*24*time.Hour)
		})

		t.Run("server -1d prior returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, -1)

			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("server 5d out returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 5)

			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("server 7d out returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 7)

			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(0*time.Second, waitTime)
		})

		t.Run("server 7d30s out returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			now := time.Now()
			notAfter := now.AddDate(0, 0, 7).Add(30 * time.Second)

			certChecker.id.ServerCert().Leaf.NotAfter = notAfter

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.GreaterOrEqual(waitTime, 20*time.Second)
			req.LessOrEqual(waitTime, 30*time.Second)
		})

		t.Run("force returns 0", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			certChecker.edgeConfig.ExtendEnrollment = true

			waitTime, err := certChecker.getWaitTime()

			req.NoError(err)
			req.Equal(time.Duration(0), waitTime)
			req.False(certChecker.edgeConfig.ExtendEnrollment)
		})
	})

	t.Run("Run", func(t *testing.T) {

		t.Run("after wait invokes extendFunc", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()
			certChecker.timeoutDuration = 10 * time.Millisecond

			invoked := false

			extender := &stubExtender{
				done: func() error {
					invoked = true
					certChecker.id.Cert().Leaf.NotAfter = time.Now().AddDate(1,0,0)
					certChecker.id.ServerCert().Leaf.NotAfter = time.Now().AddDate(1,0,0)
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

			req.True(invoked)

			certChecker.closeNotify <- struct{}{}
		})

		t.Run("double run errors", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			certChecker.isRequesting.Set(true)

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(10 * time.Millisecond)

			err := certChecker.Run()
			req.Error(err)

			certChecker.closeNotify <- struct{}{}
		})

		t.Run("timeoutDuration clears isRequesting", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()
			certChecker.timeoutDuration = 10 * time.Millisecond

			certChecker.isRequesting.Set(true)

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(50 * time.Millisecond)

			req.False(certChecker.isRequesting.Get())

			certChecker.closeNotify <- struct{}{}
		})

		t.Run("certsUpdated channel clears isRequesting pre-run", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			go func() {
				_ = certChecker.Run()
			}()

			time.Sleep(50 * time.Millisecond)

			certChecker.isRequesting.Set(true)
			certChecker.CertsUpdated()

			time.Sleep(50 * time.Millisecond)

			req.False(certChecker.isRequesting.Get())

			certChecker.closeNotify <- struct{}{}
		})

		t.Run("certsUpdated channel clears isRequesting post-run", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			certChecker.isRequesting.Set(true)

			go func() {
				_ = certChecker.Run()
			}()

			certChecker.CertsUpdated()

			time.Sleep(50 * time.Millisecond)

			req.False(certChecker.isRequesting.Get())

			certChecker.closeNotify <- struct{}{}
		})

		t.Run("client cert expired returns error", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			certChecker.id.Cert().Leaf.NotAfter = time.Now().AddDate(0, 0, -1)

			var err error

			err = certChecker.Run()
			req.Error(err)
		})
	})

	t.Run("ExtendEnrollment", func(t *testing.T) {
		t.Run("errors if control channel is closed", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			testChannel := certChecker.ctrl.(*simpleTestChannel)
			req.NotNil(testChannel)
			testChannel.isClosed = true

			err := certChecker.ExtendEnrollment()

			req.Error(err)
			req.True(certChecker.isRequesting.Get())
		})

		t.Run("errors if isRequesting = true", func(t *testing.T) {
			req := require.New(t)
			certChecker := newCertChecker()

			certChecker.isRequesting.Set(true)

			err := certChecker.ExtendEnrollment()

			req.Error(err)
			req.True(certChecker.isRequesting.Get())
		})
	})
}

var _ identity.Identity = &SimpleTestIdentity{}

type SimpleTestIdentity struct {
	TlsCert             *tls.Certificate
	TlsServerCert       *tls.Certificate
	CaPool              *x509.CertPool
	reloadCalled        bool
	setCertCalled       bool
	setServerCertCalled bool
}

func (s SimpleTestIdentity) Cert() *tls.Certificate {
	return s.TlsCert
}

func (s SimpleTestIdentity) ServerCert() *tls.Certificate {
	return s.TlsServerCert
}

func (s SimpleTestIdentity) CA() *x509.CertPool {
	return s.CaPool
}

func (s SimpleTestIdentity) ServerTLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{*s.TlsServerCert},
		RootCAs:      s.CaPool,
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tlz.GetMinTlsVersion(),
		CipherSuites: tlz.GetCipherSuites(),
	}
}

func (s SimpleTestIdentity) ClientTLSConfig() *tls.Config {
	return &tls.Config{
		RootCAs:      s.CaPool,
		Certificates: []tls.Certificate{*s.TlsCert},
	}
}

func (s SimpleTestIdentity) Reload() error {
	s.reloadCalled = true
	return nil
}

func (s SimpleTestIdentity) SetCert(string) error {
	s.setCertCalled = true
	return nil
}

func (s SimpleTestIdentity) SetServerCert(string) error {
	s.setServerCertCalled = true
	return nil
}

func (s SimpleTestIdentity) GetConfig() *identity.Config {
	return nil
}

func newCertChecker() *CertExpirationChecker {
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
		TlsServerCert:       tlsServer,
		CaPool:              caPool,
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
	return NewCertExpirationChecker(id, &edgerouter.Config{}, testChannel, closeNotify)

}

type simpleTestChannel struct {
	isClosed bool
}

func (ch *simpleTestChannel) StartRx() {
}

func (ch *simpleTestChannel) Id() *identity.TokenId {
	panic("implement Id()")
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

func (ch *simpleTestChannel) Bind(channel2.BindHandler) error {
	panic("implement Bind")
}

func (ch *simpleTestChannel) AddPeekHandler(channel2.PeekHandler) {
	panic("implement AddPeekHandler")
}

func (ch *simpleTestChannel) AddTransformHandler(channel2.TransformHandler) {
	panic("implement AddTransformHandler")
}

func (ch *simpleTestChannel) AddReceiveHandler(channel2.ReceiveHandler) {
	panic("implement AddReceiveHandler")
}

func (ch *simpleTestChannel) AddErrorHandler(channel2.ErrorHandler) {
	panic("implement me")
}

func (ch *simpleTestChannel) AddCloseHandler(channel2.CloseHandler) {
	panic("implement AddErrorHandler")
}

func (ch *simpleTestChannel) SetUserData(interface{}) {
	panic("implement SetUserData")
}

func (ch *simpleTestChannel) GetUserData() interface{} {
	panic("implement GetUserData")
}

func (ch *simpleTestChannel) Send(*channel2.Message) error {
	return nil
}

func (ch *simpleTestChannel) SendWithPriority(*channel2.Message, channel2.Priority) error {
	return nil
}

func (ch *simpleTestChannel) SendAndSync(m *channel2.Message) (chan error, error) {
	return ch.SendAndSyncWithPriority(m, channel2.Standard)
}

func (ch *simpleTestChannel) SendAndSyncWithPriority(*channel2.Message, channel2.Priority) (chan error, error) {
	result := make(chan error, 1)
	result <- nil
	return result, nil
}

func (ch *simpleTestChannel) SendWithTimeout(*channel2.Message, time.Duration) error {
	return nil
}

func (ch *simpleTestChannel) SendPrioritizedWithTimeout(*channel2.Message, channel2.Priority, time.Duration) error {
	return nil
}

func (ch *simpleTestChannel) SendAndWaitWithTimeout(*channel2.Message, time.Duration) (*channel2.Message, error) {
	panic("implement SendAndWaitWithTimeout")
}

func (ch *simpleTestChannel) SendPrioritizedAndWaitWithTimeout(*channel2.Message, channel2.Priority, time.Duration) (*channel2.Message, error) {
	panic("implement SendPrioritizedAndWaitWithTimeout")
}

func (ch *simpleTestChannel) SendAndWait(*channel2.Message) (chan *channel2.Message, error) {
	panic("implement SendAndWait")
}

func (ch *simpleTestChannel) SendAndWaitWithPriority(*channel2.Message, channel2.Priority) (chan *channel2.Message, error) {
	panic("implement SendAndWaitWithPriority")
}

func (ch *simpleTestChannel) SendForReply(channel2.TypedMessage, time.Duration) (*channel2.Message, error) {
	panic("implement SendForReply")
}

func (ch *simpleTestChannel) SendForReplyAndDecode(channel2.TypedMessage, time.Duration, channel2.TypedMessage) error {
	return nil
}

func (ch *simpleTestChannel) Close() error {
	panic("implement Close")
}

func (ch *simpleTestChannel) IsClosed() bool {
	return ch.isClosed
}

func (ch *simpleTestChannel) Underlay() channel2.Underlay {
	panic("implement Underlay")
}

func (ch *simpleTestChannel) GetTimeSinceLastRead() time.Duration {
	return 0
}

type stubExtender struct {
	isRequesting concurrenz.AtomicBoolean
	done func() error
}

func (s stubExtender) IsRequestingCompareAndSwap(expected bool, value bool) bool {
	return s.isRequesting.CompareAndSwap(expected, value)
}

func (s stubExtender) SetIsRequesting(value bool) {
	s.isRequesting.Set(value)
}

func (s stubExtender) ExtendEnrollment() error {
	s.SetIsRequesting(true)

	if s.done != nil {
		return s.done()
	}

	return nil
}

func (s stubExtender) IsRequesting() bool {
	return s.isRequesting.Get()
}