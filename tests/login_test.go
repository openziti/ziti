//go:build cli_tests

package tests

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_management_api_client"
	restidentity "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/ops"
	"github.com/openziti/ziti/ziti/run"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	username = "admin"
	password = "admin"
)

var ctrlUrl string
var commonOpts = api.Options{
	CommonOptions: common.CommonOptions{
		Out: os.Stdout,
		Err: os.Stderr,
	},
}
var token string
var homeDir = util.HomeDir()
var tmpDir string
var idFile string
var certFile string
var caFile string
var keyFile string

func findAvailablePort(t *testing.T) int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func removeZitiDir(t *testing.T) {
	zitiDir := filepath.Join(homeDir, ".ziti")
	if err := os.RemoveAll(zitiDir); err != nil {
		t.Errorf("remove %s: %v", zitiDir, err)
		t.Fail()
	}
	t.Logf("Removed ziti dir from: %s", zitiDir)
}

func waitForControllerReady(ctrlUrl string, cmdComplete chan error) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := rest_util.GetControllerWellKnownCas(ctrlUrl)
			if err == nil {
				logrus.Infof("Controller ready at %s", ctrlUrl)
				cmdComplete <- nil
				return
			}
		}
	}
}

func waitForRouter(address string, port int, done chan struct{}) {
	for {
		addr := fmt.Sprintf("%s:%d", address, port)
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			fmt.Printf("Router is available on %s:%d\n", address, port)
			close(done)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestLoginSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	removeZitiDir(t)

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := findAvailablePort(t)
	routerPort := findAvailablePort(t)
	ctrlUrl = fmt.Sprintf("https://%s:%d", ctrlAddy, ctrlPort)

	t.Logf("Controller starting at: %s", ctrlUrl)
	t.Logf("Controller address: %s", ctrlAddy)
	t.Logf("Controller port: %d", ctrlPort)
	t.Logf("Router port: %d", routerPort)

	qs := run.NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
	qs.SetArgs([]string{
		fmt.Sprintf("--ctrl-port=%d", ctrlPort),
		fmt.Sprintf("--router-port=%d", routerPort),
	})

	go func() {
		_ = qs.Execute()
	}()

	cmdComplete := make(chan error)
	go waitForControllerReady(ctrlUrl, cmdComplete)

	timeout := 30 * time.Second
	select {
	case err := <-cmdComplete:
		if err != nil {
			t.Fatal(err)
		}

		r := make(chan struct{})
		go waitForRouter(ctrlAddy, routerPort, r)
		select {
		case <-r:
			//completed normally
		case <-time.After(timeout):
			t.Errorf("timed out waiting for router on port: %d", routerPort)
			t.Fail()
		}

		t.Log("====================================================================================")
		t.Log("=========================== quickstart ready tests begin ===========================")
		t.Log("====================================================================================")

		// Get CA certs
		caCerts, err := rest_util.GetControllerWellKnownCas(ctrlUrl)
		require.NoError(t, err)

		caPool := x509.NewCertPool()
		for _, ca := range caCerts {
			caPool.AddCert(ca)
		}

		// Authentication Methods
		// do not move this test...
		t.Run("correct password succeeds", testCorrectPasswordSucceeds) // this test MUST be first as it sets the token for reuse later
		// do not move the test above!

		p := common.NewOptionsProvider(os.Stdout, os.Stderr)
		tmpDir = os.TempDir()

		jwtPath := filepath.Join(tmpDir, "test-admin.jwt")
		zitiCmd := edge.NewCmdEdge(os.Stdout, os.Stderr, p)
		zitiCmd.SetArgs([]string{"create", "identity", "test-admin", "-o", jwtPath, "--admin"})
		zitiCmdErr := zitiCmd.Execute()
		if zitiCmdErr != nil {
			t.Errorf("unable to create identity: %v", zitiCmdErr)
			t.Fail()
		}

		zitiCmd.SetArgs([]string{"enroll", jwtPath})
		zitiCmdErr = zitiCmd.Execute()
		if zitiCmdErr != nil {
			t.Errorf("unable to create identity: %v", zitiCmdErr)
			t.Fail()
		}
		idFile = strings.TrimSuffix(jwtPath, ".jwt") + ".json"
		t.Logf("identity file should exist at: %v", idFile)

		unwrapCmd := ops.NewUnwrapIdentityFileCommand(os.Stdout, os.Stderr)
		unwrapCmd.SetArgs([]string{idFile})
		unwrapCmdErr := unwrapCmd.Execute()
		if unwrapCmdErr != nil {
			t.Errorf("unable to unwrap identity: %v", unwrapCmdErr)
			t.Fail()
		}
		certFile = strings.TrimSuffix(jwtPath, ".jwt") + ".cert"
		caFile = strings.TrimSuffix(jwtPath, ".jwt") + ".ca"
		keyFile = strings.TrimSuffix(jwtPath, ".jwt") + ".key"
		t.Logf("certfile should exist at: %v", certFile)
		t.Logf("caFile should exist at: %v", caFile)
		t.Logf("keyFile should exist at: %v", keyFile)

		t.Run("wrong password fails", testWrongPasswordFails)
		t.Run("token based login", testTokenBasedLogin)
		t.Run("client cert authentication - no ca", testClientCertAuthentication)
		t.Run("identity file authentication", testIdentityFileAuthentication)
		t.Run("external JWT authentication", testExternalJWTAuthentication)
		t.Run("network identity zitified connection", testNetworkIdentityZitifiedConnection)

		// Edge Cases
		t.Run("empty username", testEmptyUsername)
		t.Run("empty password", testEmptyPassword)
		t.Run("invalid controller URL", testInvalidControllerURL)
		t.Run("non-existent username", testNonExistentUsername)
		t.Run("controller unavailable", testControllerUnavailable)

		cancel()
		time.Sleep(2 * time.Second)
		t.Log("Tests run complete.")

	case <-time.After(timeout):
		cancel()
		time.Sleep(2 * time.Second)
		panic("test failed! timed out waiting for controller to start")
	}
}

func listIdentities(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement) error {
	params := &restidentity.ListIdentitiesParams{
		Context: context.Background(),
	}
	params.SetTimeout(5 * time.Second)

	resp, err := client.Identity.ListIdentities(params, nil)
	if err != nil {
		t.Fatalf("Failed to list identities: %v", err)
	}

	identities := resp.GetPayload().Data
	t.Logf("Successfully listed %d identities", len(identities))

	return nil
}

// Authentication Methods
func testCorrectPasswordSucceeds(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      password,
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	require.NoError(t, err)
	assert.NotEmpty(t, opts.Token)
	t.Logf("Login successful, token: %s", opts.Token)

	//set the valid token for reuse later:
	token = opts.Token

	// Verify we can create a management client
	client, err := opts.NewMgmtClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func testWrongPasswordFails(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      "wrong-password",
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	assert.Error(t, err, "login with wrong password should fail")
	t.Logf("Login correctly failed with wrong password: %v", err)
}

func testTokenBasedLogin(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Token:         token,
		ControllerUrl: ctrlUrl,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	require.NoError(t, err)
	assert.Equal(t, token, opts.Token)
	t.Logf("Token login successful")
}

func testClientCertAuthentication(t *testing.T) {
	// Setup common options
	baseOpts := edge.LoginOptions{
		Options:       commonOpts,
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
		ClientCert:    certFile,
		ClientKey:     keyFile,
		CaCert:        certFile,
	}

	t.Run("all present", func(t *testing.T) {
		opts := baseOpts

		err := opts.Run()
		require.NoError(t, err, "login with cert should succeed")
	})

	t.Run("no cert", func(t *testing.T) {
		opts := baseOpts
		opts.ClientCert = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client cert is missing")
	})

	t.Run("no key", func(t *testing.T) {
		opts := baseOpts
		opts.ClientKey = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client key is missing")
	})

	t.Run("no CA cert with yes flag", func(t *testing.T) {
		removeZitiDir(t)

		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = true

		err := opts.Run()
		require.NoError(t, err, "expected success when CA cert is missing and IgnoreConfig is enabled and 'Yes' is true")
	})

	t.Run("no CA cert without yes flag", func(t *testing.T) {
		zitiDir := filepath.Join(homeDir, ".ziti")
		if err := os.RemoveAll(zitiDir); err != nil {
			t.Errorf("remove %s: %v", zitiDir, err)
			t.Fail()
		}
		t.Logf("Removed ziti dir from: %s", zitiDir)

		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = false

		err := opts.Run()
		require.Error(t, err, "expected error when CA cert is missing")
	})
}

func testIdentityFileAuthentication(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
		File:          idFile,
	}

	err := opts.Run()
	require.NoError(t, err)

	client, err := opts.NewMgmtClient()
	require.NoError(t, err)
	assert.NotNil(t, client)
	lerr := listIdentities(t, client)
	if lerr != nil {
		t.Logf("Failed to list identities: %v", lerr)
		t.Fail()
	}
}

func testExternalJWTAuthentication(t *testing.T) {
	// TODO: Generate valid JWT token
	t.Skip("External JWT authentication requires JWT setup")
}

func testNetworkIdentityZitifiedConnection(t *testing.T) {
	// TODO: Create network identity file
	t.Skip("Network identity requires identity setup")
}

// Edge Cases
func testEmptyUsername(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      "",
		Password:      password,
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	assert.Error(t, err, "empty username should fail")
	t.Logf("Empty username correctly failed: %v", err)
}

func testEmptyPassword(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      "",
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	assert.Error(t, err, "empty password should fail")
	t.Logf("Empty password correctly failed: %v", err)
}

func testInvalidControllerURL(t *testing.T) {
	invalidUrls := []string{
		"not-a-url",
		"http://[invalid",
		"ftp://wrong-scheme.com",
		"https://non-existent-host-12345.local:9999",
	}

	for _, invalidUrl := range invalidUrls {
		t.Run(invalidUrl, func(t *testing.T) {
			opts := &edge.LoginOptions{
				Options:       commonOpts,
				Username:      username,
				Password:      password,
				ControllerUrl: invalidUrl,
				Yes:           true,
				IgnoreConfig:  true,
			}

			err := opts.Run()
			assert.Error(t, err, "invalid URL should fail")
			t.Logf("Invalid URL %s correctly failed: %v", invalidUrl, err)
		})
	}
}

func testNonExistentUsername(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      "nonexistent-user-12345",
		Password:      password,
		ControllerUrl: ctrlUrl,
		Yes:           true,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	assert.Error(t, err, "non-existent username should fail")
	t.Logf("Non-existent username correctly failed: %v", err)
}

func testControllerUnavailable(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      password,
		ControllerUrl: "https://127.0.0.1:9999",
		Yes:           true,
		IgnoreConfig:  true,
	}

	err := opts.Run()
	assert.Error(t, err, "unavailable controller should fail")
	t.Logf("Unavailable controller correctly failed: %v", err)
}
