//go:build dataflow

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

package tests

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	nfpem "github.com/openziti/foundation/v2/pem"
	idlib "github.com/openziti/identity"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/ziti/v2/common/eid"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The primary controller of both the default-ats and ha-3 config sets presents ctrl1's server
// cert, which this intermediate issued.
const (
	ctrl1CaCertFile = "testdata/pki/ctrl1/certs/ctrl1.cert"
	ctrl1CaKeyFile  = "testdata/pki/ctrl1/keys/ctrl1.key"
)

// Test_ControllerCertRenewal_Standalone renews a non-HA controller's server cert while it runs, as
// the packaged renewal timer does, and checks that tokens signed before and after the renewal
// keep validating at the controller and router, including after the controller restarts.
func Test_ControllerCertRenewal_Standalone(t *testing.T) {
	ctrlConfig, serverCertFile, err := renewableCtrlConfig(DefaultATS.CtrlConfig, t.TempDir())
	require.NoError(t, err)

	configSet := DefaultATS
	configSet.CtrlConfig = ctrlConfig

	ctx := NewTestContextWithConfigSet(t, configSet)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	preRenewalService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)
	postRenewalService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)
	edgeRouter := ctx.CreateEnrollAndStartEdgeRouter()

	_, preRenewalHost := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer preRenewalHost.Close()
	preRenewalListener, err := preRenewalHost.ListenWithOptions(preRenewalService.Name, &ziti.ListenOptions{ConnectTimeout: 10 * time.Second, WaitForNEstablishedListeners: 1})
	ctx.Req.NoError(err)
	defer func() { _ = preRenewalListener.Close() }()
	newTestServer(preRenewalListener, echoUntilEof).start()
	ctx.Req.Eventually(func() bool { return ctx.controllersMissingTerminator(preRenewalService.Id) == nil },
		10*time.Second, 100*time.Millisecond, "%v", ctx.controllersMissingTerminator(preRenewalService.Id))

	// The client authenticates and connects before the renewal, so it holds tokens signed with the
	// previous cert and has a router to push refreshed tokens to.
	_, clientIface := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientIface.Close()
	client := clientIface.(*ziti.ContextImpl)
	ctx.Req.NoError(client.Authenticate())
	ctx.Req.NoError(client.ConnectAllAvailableErs())

	previousCert, err := x509.ParseCertificate(ctx.ControllerConfig.Id.ServerCert()[0].Certificate[0])
	ctx.Req.NoError(err)

	renewedCert, err := renewServerCert(serverCertFile, ctrl1CaCertFile, ctrl1CaKeyFile)
	ctx.Req.NoError(err)

	ctx.Req.Eventually(func() bool {
		return bytes.Equal(renewedCert.Raw, ctx.ControllerConfig.Id.ServerCert()[0].Certificate[0])
	}, 10*time.Second, 100*time.Millisecond, "the controller identity never reloaded the renewed cert")

	previousKid := certKid(previousCert)
	renewedKid := certKid(renewedCert)

	// The first check publishes the renewed cert and starts the grace; the second ends it once the
	// router holds the renewed key.
	ctx.Req.NoError(ctx.EdgeController.AppEnv.CheckSigningCert(false, time.Hour))
	kidDuringGrace := ctx.EdgeController.AppEnv.GetRootTlsJwtSigner().KeyId()

	ctx.Req.Eventually(func() bool {
		return missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, renewedKid) == nil
	}, 10*time.Second, 100*time.Millisecond, "router keys: %v", missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, renewedKid))

	ctx.Req.NoError(ctx.EdgeController.AppEnv.CheckSigningCert(false, 0))

	t.Run("JWT signing stays on the previous cert during the grace", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.Equal(previousKid, kidDuringGrace)
	})

	t.Run("JWT signing moves to the renewed cert once the grace has passed", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.Equal(renewedKid, ctx.EdgeController.AppEnv.GetRootTlsJwtSigner().KeyId())
	})

	t.Run("the controller record presents the renewed cert and retains the previous one", func(t *testing.T) {
		ctx.NextTest(t)

		record, err := ctx.EdgeController.AppEnv.Managers.Controller.Read(ctx.EdgeController.AppEnv.GetId())
		ctx.Req.NoError(err)

		ctx.Req.Equal(renewedKid, certKid(nfpem.PemStringToCertificates(record.CertPem)[0]))
		ctx.Req.Contains(record.RetainedCerts, nfpem.EncodeToString(previousCert))
	})

	t.Run("the router holds the renewed and previous keys", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.Eventually(func() bool {
			return missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, previousKid, renewedKid) == nil
		}, 10*time.Second, 100*time.Millisecond, "router keys: %v", missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, previousKid, renewedKid))
	})

	t.Run("api session refreshes are accepted by the controller and router", func(t *testing.T) {
		ctx.NextTest(t)

		// The first refresh spends a refresh token signed before the renewal; the second spends
		// the one the first returned, signed after it.
		for i := 0; i < 2; i++ {
			functionalErr, routerErr := client.RefreshApiSession()
			ctx.Req.NoError(functionalErr, "refresh %d", i+1)
			ctx.Req.NoError(routerErr, "refresh %d", i+1)
		}
	})

	t.Run("a bind made before renewal serves dials", func(t *testing.T) {
		ctx.NextTest(t)

		forceV1 := true
		conn := ctx.WrapConn(client.DialWithOptions(preRenewalService.Name, &ziti.DialOptions{ConnectTimeout: 5 * time.Second, ForceConnectV1: &forceV1}))
		defer conn.Close()

		name := eid.New()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("echo: "+name, time.Second)
	})

	t.Run("a bind made after renewal serves dials", func(t *testing.T) {
		ctx.NextTest(t)

		_, postRenewalHost := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer postRenewalHost.Close()
		postRenewalListener, err := postRenewalHost.ListenWithOptions(postRenewalService.Name, &ziti.ListenOptions{ConnectTimeout: 10 * time.Second, WaitForNEstablishedListeners: 1})
		ctx.Req.NoError(err)
		defer func() { _ = postRenewalListener.Close() }()
		newTestServer(postRenewalListener, echoUntilEof).start()
		ctx.Req.Eventually(func() bool { return ctx.controllersMissingTerminator(postRenewalService.Id) == nil },
			10*time.Second, 100*time.Millisecond, "%v", ctx.controllersMissingTerminator(postRenewalService.Id))

		forceV1 := true
		conn := ctx.WrapConn(client.DialWithOptions(postRenewalService.Name, &ziti.DialOptions{ConnectTimeout: 5 * time.Second, ForceConnectV1: &forceV1}))
		defer conn.Close()

		name := eid.New()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("echo: "+name, time.Second)
	})

	t.Run("both keys survive a controller restart", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.restartPrimaryController("testdata/default.db", nil)

		ctx.Req.NoError(missingJwtKeys(ctx.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, previousKid, renewedKid))

		ctx.Req.Eventually(func() bool {
			return missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, previousKid, renewedKid) == nil
		}, 30*time.Second, 100*time.Millisecond, "router keys: %v", missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, previousKid, renewedKid))

		functionalErr, routerErr := client.RefreshApiSession()
		ctx.Req.NoError(functionalErr)
		ctx.Req.NoError(routerErr)
	})
}

// Test_ControllerCertRenewal_Cluster renews one cluster member's server cert while it runs and
// checks that every member and the router learn the renewed key without losing the previous one,
// and that tokens the renewed member signs validate at its peers, including a restarted one.
func Test_ControllerCertRenewal_Cluster(t *testing.T) {
	ctrlConfig, serverCertFile, err := renewableCtrlConfig(Ha3.CtrlConfig, t.TempDir())
	require.NoError(t, err)

	configSet := Ha3
	configSet.CtrlConfig = ctrlConfig

	ctx := NewTestContextWithConfigSet(t, configSet)
	defer ctx.Teardown()
	ctx.StartHaCluster(Ha3DataDir)
	ctx.RequireAdminManagementApiLogin()

	preRenewalService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)
	postRenewalService := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)
	edgeRouter := ctx.CreateEnrollAndStartEdgeRouter()

	_, preRenewalHost := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer preRenewalHost.Close()
	preRenewalListener, err := preRenewalHost.ListenWithOptions(preRenewalService.Name, &ziti.ListenOptions{ConnectTimeout: 10 * time.Second, WaitForNEstablishedListeners: 1})
	ctx.Req.NoError(err)
	defer func() { _ = preRenewalListener.Close() }()
	newTestServer(preRenewalListener, echoUntilEof).start()
	ctx.Req.Eventually(func() bool { return ctx.controllersMissingTerminator(preRenewalService.Id) == nil },
		10*time.Second, 100*time.Millisecond, "%v", ctx.controllersMissingTerminator(preRenewalService.Id))

	_, clientIface := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientIface.Close()
	client := clientIface.(*ziti.ContextImpl)
	ctx.Req.NoError(client.Authenticate())
	ctx.Req.NoError(client.ConnectAllAvailableErs())

	adminClient := ctx.NewEdgeClientApi(nil)
	adminClient.SetUseOidc(true)
	preRenewalSession, err := adminClient.Authenticate(ctx.NewAdminCredentials(), nil)
	ctx.Req.NoError(err)

	previousCert, err := x509.ParseCertificate(ctx.ControllerConfig.Id.ServerCert()[0].Certificate[0])
	ctx.Req.NoError(err)

	renewedCert, err := renewServerCert(serverCertFile, ctrl1CaCertFile, ctrl1CaKeyFile)
	ctx.Req.NoError(err)

	ctx.Req.Eventually(func() bool {
		return bytes.Equal(renewedCert.Raw, ctx.ControllerConfig.Id.ServerCert()[0].Certificate[0])
	}, 10*time.Second, 100*time.Millisecond, "the controller identity never reloaded the renewed cert")

	renewedId := ctx.EdgeController.AppEnv.GetId()
	previousKid := certKid(previousCert)
	renewedKid := certKid(renewedCert)

	// The first check publishes the renewed cert and starts the grace; the second ends it once every
	// peer and the router hold the renewed key.
	ctx.Req.NoError(ctx.EdgeController.AppEnv.CheckSigningCert(false, time.Hour))
	kidDuringGrace := ctx.EdgeController.AppEnv.GetRootTlsJwtSigner().KeyId()

	for _, peer := range ctx.peerControllers {
		ctx.Req.Eventually(func() bool {
			return missingJwtKeys(peer.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, renewedKid) == nil
		}, 10*time.Second, 100*time.Millisecond, "peer %s keys: %v", peer.ApiHost, missingJwtKeys(peer.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, renewedKid))
	}
	ctx.Req.Eventually(func() bool {
		return missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, renewedKid) == nil
	}, 10*time.Second, 100*time.Millisecond, "router keys: %v", missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, renewedKid))

	ctx.Req.NoError(ctx.EdgeController.AppEnv.CheckSigningCert(false, 0))

	t.Run("JWT signing stays on the previous cert during the grace", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.Equal(previousKid, kidDuringGrace)
	})

	t.Run("JWT signing moves to the renewed cert once the grace has passed", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.Equal(renewedKid, ctx.EdgeController.AppEnv.GetRootTlsJwtSigner().KeyId())
	})

	t.Run("every member holds the renewed and previous keys", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.NoError(missingJwtKeys(ctx.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, previousKid, renewedKid))

		for _, peer := range ctx.peerControllers {
			ctx.Req.Eventually(func() bool {
				return missingJwtKeys(peer.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, previousKid, renewedKid) == nil
			}, 10*time.Second, 100*time.Millisecond, "peer %s keys: %v", peer.ApiHost, missingJwtKeys(peer.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, previousKid, renewedKid))
		}
	})

	t.Run("every member's record of the renewed member retains the previous cert", func(t *testing.T) {
		ctx.NextTest(t)

		for _, peer := range ctx.peerControllers {
			record, err := peer.EdgeController.AppEnv.Managers.Controller.Read(renewedId)
			ctx.Req.NoError(err)

			ctx.Req.Equal(renewedKid, certKid(nfpem.PemStringToCertificates(record.CertPem)[0]), "peer %s", peer.ApiHost)
			ctx.Req.Contains(record.RetainedCerts, nfpem.EncodeToString(previousCert), "peer %s", peer.ApiHost)
		}
	})

	t.Run("the router holds the renewed and previous keys", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.Req.Eventually(func() bool {
			return missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, previousKid, renewedKid) == nil
		}, 10*time.Second, 100*time.Millisecond, "router keys: %v", missingJwtKeys(edgeRouter.GetRouterDataModel().PublicKeys, previousKid, renewedKid))
	})

	t.Run("a peer accepts a refresh token the renewed member issued", func(t *testing.T) {
		ctx.NextTest(t)

		postRenewalSession, err := adminClient.API.RefreshApiSession(preRenewalSession, adminClient.HttpClient)
		ctx.Req.NoError(err)

		peerClient := ctx.NewEdgeClientApiForHost(ctx.PeerControllerApiHosts()[0], nil)
		_, err = peerClient.API.RefreshApiSession(postRenewalSession, peerClient.HttpClient)
		ctx.Req.NoError(err)
	})

	t.Run("api session refreshes are accepted by the controllers and router", func(t *testing.T) {
		ctx.NextTest(t)

		for i := 0; i < 2; i++ {
			functionalErr, routerErr := client.RefreshApiSession()
			ctx.Req.NoError(functionalErr, "refresh %d", i+1)
			ctx.Req.NoError(routerErr, "refresh %d", i+1)
		}
	})

	t.Run("a bind made before renewal serves dials", func(t *testing.T) {
		ctx.NextTest(t)

		forceV1 := true
		conn := ctx.WrapConn(client.DialWithOptions(preRenewalService.Name, &ziti.DialOptions{ConnectTimeout: 5 * time.Second, ForceConnectV1: &forceV1}))
		defer conn.Close()

		name := eid.New()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("echo: "+name, time.Second)
	})

	t.Run("a bind made after renewal serves dials", func(t *testing.T) {
		ctx.NextTest(t)

		_, postRenewalHost := ctx.AdminManagementSession.RequireCreateSdkContext()
		defer postRenewalHost.Close()
		postRenewalListener, err := postRenewalHost.ListenWithOptions(postRenewalService.Name, &ziti.ListenOptions{ConnectTimeout: 10 * time.Second, WaitForNEstablishedListeners: 1})
		ctx.Req.NoError(err)
		defer func() { _ = postRenewalListener.Close() }()
		newTestServer(postRenewalListener, echoUntilEof).start()
		ctx.Req.Eventually(func() bool { return ctx.controllersMissingTerminator(postRenewalService.Id) == nil },
			10*time.Second, 100*time.Millisecond, "%v", ctx.controllersMissingTerminator(postRenewalService.Id))

		forceV1 := true
		conn := ctx.WrapConn(client.DialWithOptions(postRenewalService.Name, &ziti.DialOptions{ConnectTimeout: 5 * time.Second, ForceConnectV1: &forceV1}))
		defer conn.Close()

		name := eid.New()
		conn.WriteString(name, time.Second)
		conn.ReadExpected("echo: "+name, time.Second)
	})

	t.Run("a restarted peer rebuilds both keys", func(t *testing.T) {
		ctx.NextTest(t)

		ctx.restartPeerController(0, nil, nil)
		peer := ctx.peerControllers[0]

		ctx.Req.NoError(missingJwtKeys(peer.EdgeController.AppEnv.Broker.GetRouterDataModel().PublicKeys, previousKid, renewedKid))
	})
}

// controllersMissingTerminator returns an error naming each controller whose own store holds no
// terminator for serviceId, or nil if every controller's does. A circuit can be routed by any
// cluster member, so a dial that follows a bind waits for all of them.
func (ctx *TestContext) controllersMissingTerminator(serviceId string) error {
	appEnvs := map[string]*env.AppEnv{ctx.ApiHost: ctx.EdgeController.AppEnv}
	for _, peer := range ctx.peerControllers {
		appEnvs[peer.ApiHost] = peer.EdgeController.AppEnv
	}

	var missing []string
	for apiHost, appEnv := range appEnvs {
		result, err := appEnv.Managers.Terminator.BaseList(fmt.Sprintf(`service="%s"`, serviceId))
		if err != nil {
			return err
		}
		if len(result.Entities) == 0 {
			missing = append(missing, apiHost)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("no terminator for service %s on controllers %v", serviceId, missing)
	}
	return nil
}

// echoUntilEof is a testServer dispatcher that answers each string with "echo: " and the string
// until the client closes the connection.
func echoUntilEof(conn *testServerConn) error {
	for {
		val, eof := conn.ReadString(4096, time.Minute)
		if eof {
			return nil
		}
		conn.WriteString("echo: "+val, time.Second)
	}
}

// certKid returns the JWT key ID a controller derives from cert.
func certKid(cert *x509.Certificate) string {
	return fmt.Sprintf("%x", sha1.Sum(cert.Raw))
}

// missingJwtKeys returns an error naming each of kids that publicKeys lacks as a JWT validation
// key, or nil if it holds them all.
func missingJwtKeys(publicKeys cmap.ConcurrentMap[string, *edge_ctrl_pb.DataState_PublicKey], kids ...string) error {
	var missing []string
	for _, kid := range kids {
		key, found := publicKeys.Get(kid)
		if !found || !slices.Contains(key.Usages, edge_ctrl_pb.DataState_PublicKey_JWTValidation) {
			missing = append(missing, kid)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing JWT validation keys %v", missing)
	}
	return nil
}

// renewableCtrlConfig copies the controller config at configFile into dir, along with the cert and
// key files its identity names, pointing the copy's identity at the copied files. It returns the
// copy's path and the path of its server cert chain. Other paths in the config are left as they
// are, so the copy must be loaded from the original's working directory.
func renewableCtrlConfig(configFile, dir string) (string, string, error) {
	raw, err := os.ReadFile(configFile)
	if err != nil {
		return "", "", err
	}

	parsed := struct {
		Identity map[string]string `yaml:"identity"`
	}{}
	if err = yaml.Unmarshal(raw, &parsed); err != nil {
		return "", "", err
	}

	text := string(raw)
	serverCertFile := ""

	for _, field := range []string{"cert", "server_cert", "key", "server_key"} {
		src := parsed.Identity[field]
		if src == "" {
			return "", "", fmt.Errorf("controller config %s has no identity.%s", configFile, field)
		}

		contents, err := os.ReadFile(src)
		if err != nil {
			return "", "", err
		}

		dst := filepath.Join(dir, field+filepath.Ext(src))
		if err = os.WriteFile(dst, contents, 0600); err != nil {
			return "", "", err
		}

		text = strings.ReplaceAll(text, src, dst)
		if field == "server_cert" {
			serverCertFile = dst
		}
	}

	ctrlConfig := filepath.Join(dir, filepath.Base(configFile))
	if err = os.WriteFile(ctrlConfig, []byte(text), 0600); err != nil {
		return "", "", err
	}

	return ctrlConfig, serverCertFile, nil
}

// renewServerCert rewrites the cert chain in chainFile in place, as a renewal does, replacing its
// leaf with a new cert for the same key, subject and SANs issued by the CA in caCertFile and
// caKeyFile. The rest of the chain is kept. It returns the new leaf.
func renewServerCert(chainFile, caCertFile, caKeyFile string) (*x509.Certificate, error) {
	chain, err := idlib.LoadCert(chainFile)
	if err != nil {
		return nil, err
	}

	caCerts, err := idlib.LoadCert(caCertFile)
	if err != nil {
		return nil, err
	}

	caKey, err := idlib.LoadKey(caKeyFile)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	previous := chain[0]
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               previous.Subject,
		DNSNames:              previous.DNSNames,
		IPAddresses:           previous.IPAddresses,
		URIs:                  previous.URIs,
		KeyUsage:              previous.KeyUsage,
		ExtKeyUsage:           previous.ExtKeyUsage,
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, caCerts[0], previous.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	renewed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}

	pemChain := nfpem.EncodeToString(renewed)
	for _, cert := range chain[1:] {
		pemChain += nfpem.EncodeToString(cert)
	}

	if err = os.WriteFile(chainFile, []byte(pemChain), 0600); err != nil {
		return nil, err
	}

	return renewed, nil
}
