//go:build cli_tests

package tests

import (
	"context"
	"errors"
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

func waitForControllerReady(t *testing.T, ctrlUrl string, cmdComplete chan error) {
	t.Logf("Waiting for controller at %s\n", ctrlUrl)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := rest_util.GetControllerWellKnownCas(ctrlUrl)
			if err == nil {
				t.Logf("Controller ready at %s", ctrlUrl)
				cmdComplete <- nil
				return
			}
		}
	}
}

func waitForRouter(t *testing.T, address string, port int, done chan struct{}) {
	addr := fmt.Sprintf("%s:%d", address, port)
	fmt.Printf("Waiting for router at %s\n", addr)
	for {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = conn.Close()
			t.Logf("Router is available on %s:%d\n", address, port)
			close(done)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestLoginSuite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	print(ctx)
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
		fmt.Sprintf("--ctrl-address=%s", ctrlAddy),
		fmt.Sprintf("--router-port=%d", routerPort),
		fmt.Sprintf("--router-address=%s", ctrlAddy),
	})

	go func() {
		qsErr := qs.Execute()
		if qsErr != nil {
			t.Errorf("error executing quickstart command: %v", qsErr)
			t.Fail()
		}
	}()

	cmdComplete := make(chan error)
	go waitForControllerReady(t, ctrlUrl, cmdComplete)

	testTimeout := 60 * time.Second
	select {
	case err := <-cmdComplete:
		if err != nil {
			t.Error(err)
			t.Fail()
		}

		r := make(chan struct{})
		go waitForRouter(t, ctrlAddy, routerPort, r)
		routerOnlineTimeout := 30 * time.Second
		select {
		case <-r:
			//completed normally
		case <-time.After(routerOnlineTimeout):
			t.Errorf("timed out waiting for router on port: %d", routerPort)
			t.Fail()
			return
		}

		t.Log("====================================================================================")
		t.Log("=========================== quickstart ready tests begin ===========================")
		t.Log("====================================================================================")

		initialLogin := &edge.LoginOptions{
			Options:       commonOpts,
			Username:      username,
			Password:      password,
			ControllerUrl: ctrlUrl,
			Yes:           true,
		}
		loginErr := initialLogin.Run()
		if loginErr != nil {
			t.Errorf("Could not login? %v: ", loginErr)
			t.Fail()
			return
		}

		p := common.NewOptionsProvider(os.Stdout, os.Stderr)
		tmpDir = os.TempDir()

		initialIdName := fmt.Sprintf("test-admin-%s", time.Now().Format("150405"))
		jwtPath := filepath.Join(tmpDir, initialIdName+".jwt")
		zitiCmd := edge.NewCmdEdge(os.Stdout, os.Stderr, p)
		zitiCmd.SetArgs([]string{"create", "identity", initialIdName, "-o", jwtPath, "--admin"})
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

		// Authentication Methods
		t.Run("correct password succeeds", testCorrectPasswordSucceeds)
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

	case <-time.After(testTimeout):
		cancel()
		time.Sleep(2 * time.Second)
		t.Error("test failed! timed out waiting for controller to start")
		t.Fail()
	}
}

func listIdentities(t *testing.T, client *rest_management_api_client.ZitiEdgeManagement) error {
	params := &restidentity.ListIdentitiesParams{
		Context: context.Background(),
	}
	params.SetTimeout(5 * time.Second)

	resp, err := client.Identity.ListIdentities(params, nil)
	if err != nil {
		t.Errorf("Failed to list identities: %v", err)
		t.Fail()
		return err
	}

	identities := resp.GetPayload().Data
	t.Logf("Successfully listed %d identities", len(identities))
	if len(identities) < 1 {
		return errors.New("no identities found, this is unexpected")
	}
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
		IgnoreConfig:  false,
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
	assert.Contains(t, err.Error(), "401 Unauthorized")
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
	require.Equal(t, token, opts.Token)
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
		require.NoError(t, err, "login with cert/key/ca when all present should succeed")
	})

	t.Run("no cert", func(t *testing.T) {
		opts := baseOpts
		opts.ClientCert = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client cert is missing")
		require.Contains(t, err.Error(), "username required but not provided")
	})

	t.Run("no key", func(t *testing.T) {
		opts := baseOpts
		opts.ClientKey = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client key is missing")
		require.Contains(t, err.Error(), "can't load client certificate")
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
		require.Contains(t, err.Error(), "Cannot accept certs - no terminal")
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
	require.NotNil(t, client)
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
	require.Error(t, err, "empty username should fail")
	require.Contains(t, err.Error(), "username required but not provided")
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
	require.Error(t, err, "empty password should fail")
	require.Contains(t, err.Error(), "password required but not provided")
	t.Logf("Empty password correctly failed: %v", err)
}

func testInvalidControllerURL(t *testing.T) {
	t.Run("not-a-url", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "not-a-url", Yes: true, IgnoreConfig: true}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("http://[invalid", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "http://[invalid", Yes: true, IgnoreConfig: true}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid controller URL")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("ftp://wrong-scheme.com", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "ftp://wrong-scheme.com", Yes: true, IgnoreConfig: true}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("https://non-existent-host-12345.local:9999", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "https://non-existent-host-12345.local:9999", Yes: true, IgnoreConfig: true}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})
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
	require.Error(t, err, "non-existent username should fail")
	require.Contains(t, err.Error(), "401 Unauthorized")
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
	require.Error(t, err, "unavailable controller should fail")
	require.Contains(t, err.Error(), "the target machine actively refused it")
	t.Logf("Unavailable controller correctly failed: %v", err)
}
