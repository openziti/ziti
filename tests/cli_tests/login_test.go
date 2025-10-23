//go:build cli_tests

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
package cli_tests

import (
	"context"
	"net/http"
	"os"
	gopath "path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/tests/testutil"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/util"
	"github.com/stretchr/testify/require"
)

type loginTestState struct {
	pidsMutex           sync.Mutex
	homeDir             string
	zitiContext         *ziti.Context
	zitiTransport       *http.Transport
	commonOpts          api.Options
	externalZiti        testutil.Overlay
	controllerUnderTest testutil.Overlay
}

func (s *loginTestState) removeZitiDir(t *testing.T) {
	zitiDir, _ := util.ConfigDir()
	if err := os.RemoveAll(zitiDir); err != nil {
		t.Errorf("remove %s: %v", zitiDir, err)
		t.Fail()
	}
	t.Logf("Removed ziti dir from: %s", zitiDir)
}

func Test_LoginSuite(t *testing.T) {
	zitiPath := os.Getenv("ZITI_CLI_TEST_ZITI_BIN")
	if zitiPath == "" {
		t.Fatalf("ZITI_CLI_TEST_ZITI_BIN not set")
	}
	if _, err := os.Stat(zitiPath); err != nil {
		t.Fatalf("ziti binary not found at provided location %s: %v", zitiPath, err)
	}
	baseDir := filepath.Join(os.TempDir(), "cli-tests")
	if me := os.MkdirAll(baseDir, 0755); me != nil {
		t.Fatalf("failed creating baseDir dir: %v", baseDir)
	}
	testRunHome, err := os.MkdirTemp(baseDir, "test-run-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	// set ZITI_CONFIG_DIR so that anything here forth is not corrupting local stuff
	_ = os.Setenv("ZITI_CONFIG_DIR", filepath.Join(testRunHome, ".config/ziti"))

	externalCtx, externalCancel := context.WithCancel(context.Background())
	defer externalCancel()
	ctrlUnderTestCtx, ctrlUnderTestCancel := context.WithCancel(context.Background())
	defer ctrlUnderTestCancel()

	testState := &loginTestState{
		homeDir:             util.HomeDir(),
		zitiContext:         nil,
		zitiTransport:       nil,
		externalZiti:        testutil.CreateOverlay(t, externalCtx, 0, testRunHome, "external", false),
		controllerUnderTest: testutil.CreateOverlay(t, ctrlUnderTestCtx, 330*time.Second, testRunHome, "target", false),
		commonOpts: api.Options{
			CommonOptions: common.CommonOptions{
				Out: os.Stdout,
				Err: os.Stderr,
			},
		},
	}

	defer func() {
		if !t.Failed() {
			// allow/ensure the processes to exit windows is a pain about rm'ing folders if not
			errChan := make(chan error, 2)

			go func() { errChan <- testState.externalZiti.Stop() }()
			go func() { errChan <- testState.controllerUnderTest.Stop() }()
			success := true
			for i := 0; i < 2; i++ { // Wait for both
				if err := <-errChan; err != nil {
					t.Logf("stop error: %v", err)
					success = false
				}
			}
			if !success {
				t.Logf("manual cleanup may be required at %s", testRunHome)
			} else {
				t.Logf("tests passed, removing temp dir at %s", testRunHome)
				if rerr := os.RemoveAll(testRunHome); rerr != nil {
					t.Logf("remove %s failed... **sigh**: %v", testRunHome, rerr)
				}
			}
		} else {
			t.Logf("tests failed, temp dir left intact at %s", testRunHome)
		}
		testState.externalZiti.CleanupPids()
		testState.controllerUnderTest.CleanupPids()
	}()

	extDone := make(chan error)
	go testState.externalZiti.StartExternal(zitiPath, extDone)
	go func() {
		qsErr := <-extDone
		if qsErr == nil {
			externalCancel()
			t.Fatal("unexpected error from external quickstart?")
		}
	}()

	testState.externalZiti.WaitForControllerReadyorig(t, nil)
	testState.controllerUnderTest.Name = "target"
	targetDone := make(chan error)
	go testState.controllerUnderTest.StartExternal(zitiPath, targetDone)
	go func() {
		qsErr := <-targetDone
		if qsErr == nil {
			ctrlUnderTestCancel()
			t.Fatal("unexpected error from external quickstart?")
		}
	}()
	testState.controllerUnderTest.WaitForControllerReadyorig(t, nil)
	if _, le := testState.controllerUnderTest.Login(); le != nil {
		t.Fatalf("unable to login before running tests: %v", le)
	}
	t.Logf("Target controller at: %s", testState.controllerUnderTest.ControllerHostPort())

	now := time.Now().Format("150405")

	if ae := testState.controllerUnderTest.CreateAdminIdentity(t, now, testRunHome); ae != nil {
		t.Fatalf("unable to create controller admin: %v", ae)
	}

	t.Log("====================================================================================")
	t.Log("=========================== overlay ready. tests begin =============================")
	t.Log("====================================================================================")

	testTimeout := 600 * time.Second
	testDone := make(chan struct{})
	testTimer := time.NewTimer(testTimeout)
	defer testTimer.Stop()

	go func() {
		defer close(testDone)
		// Just signal when time is up, don't call t methods
		<-testTimer.C
	}()

	if lr, le := testState.controllerUnderTest.Login(); le != nil {
		t.Fatalf("unable to login before running tests: %v", le)
	} else {
		//set the valid token for reuse later:
		require.NotEmpty(t, lr.Token)
		testState.controllerUnderTest.Token = lr.Token
	}

	t.Run("login tests over underlay", testState.runLoginTests)

	t.Log("Cancelling controllerUnderTest to reconfigure for use with ziti")

	ctrlUnderTestCancel()
	if se := testState.controllerUnderTest.Stop(); se != nil {
		t.Fatalf("controllerUnderTest didn't stop? %v", se)
	}
	t.Log("Cancelling controllerUnderTest complete")
	testState.loginTestsOverZiti(t, now, zitiPath)
	externalCancel()

	t.Run("make sure any ziti instances are stopped", testState.externalZiti.EnsureAllPidsStopped)
	t.Run("make sure any ziti instances are stopped", testState.controllerUnderTest.EnsureAllPidsStopped)
}

// Authentication Methods
func (s *loginTestState) testCorrectPasswordSucceeds(t *testing.T) {
	opts := s.controllerUnderTest.NewTestLoginOpts()

	err := opts.Run()
	require.NoError(t, err)
	require.NotEmpty(t, opts.Token)
	t.Logf("Login successful, token: %s", opts.Token)

	// Verify we can create a management client
	client, err := opts.NewMgmtClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotEmpty(t, opts.Token)
}

func (s *loginTestState) testWrongPasswordFails(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      "wrong-password",
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "login with wrong password should fail")
	require.Contains(t, err.Error(), "401 Unauthorized")
}

func (s *loginTestState) testTokenBasedLogin(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Token:         s.controllerUnderTest.Token,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.NoError(t, err)
	require.Equal(t, s.controllerUnderTest.Token, opts.Token)
	t.Logf("Login successful, token: %s", opts.Token)
}

func (s *loginTestState) testClientCertAuthentication(t *testing.T) {
	// Setup common options
	baseOpts := edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		ClientCert:    s.controllerUnderTest.AdminCertFile,
		ClientKey:     s.controllerUnderTest.AdminKeyFile,
		CaCert:        s.controllerUnderTest.AdminCaFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	s.removeZitiDir(t)
	t.Run("all present", func(t *testing.T) {
		opts := baseOpts

		err := opts.Run()
		require.NoError(t, err, "login with cert/key/ca when all present should succeed")
		require.NotEmpty(t, opts.Token)
		t.Logf("Login successful, token: %s", opts.Token)
	})

	s.removeZitiDir(t)
	t.Run("no cert", func(t *testing.T) {
		opts := baseOpts
		opts.ClientCert = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client cert is missing")
		require.Contains(t, err.Error(), "username required but not provided")
	})

	s.removeZitiDir(t)
	t.Run("no key", func(t *testing.T) {
		opts := baseOpts
		opts.ClientKey = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client key is missing")
		require.Contains(t, err.Error(), "can't load client certificate")
	})

	s.removeZitiDir(t)
	t.Run("no CA cert with yes flag", func(t *testing.T) {
		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = true

		err := opts.Run()
		require.NoError(t, err, "expected success when CA cert is missing and IgnoreConfig is enabled and 'Yes' is true")
		require.NotEmpty(t, opts.Token)
		t.Logf("Login successful, token: %s", opts.Token)
	})

	s.removeZitiDir(t)
	t.Run("no CA cert without yes flag", func(t *testing.T) {
		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = false

		err := opts.Run()
		require.Error(t, err, "expected error when CA cert is missing")
		require.Contains(t, err.Error(), "Cannot accept certs - no terminal")
	})
}

func (s *loginTestState) testIdentityFileAuthentication(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		File:          s.controllerUnderTest.AdminIdFile,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.NoError(t, err)

	client, err := opts.NewMgmtClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotEmpty(t, opts.Token)
	t.Logf("Login successful, token: %s", opts.Token)
}

func (s *loginTestState) testExternalJWTAuthentication(t *testing.T) {
	// TODO: Generate valid JWT token
	t.Skip("External JWT authentication requires JWT setup")
}

func (s *loginTestState) testNetworkIdentityZitifiedConnection(t *testing.T) {
	// TODO: Create network identity file
	t.Skip("Network identity requires identity setup")
}

// Edge Cases
func (s *loginTestState) testEmptyUsername(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      "",
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "empty username should fail")
	require.Contains(t, err.Error(), "username required but not provided")
	t.Logf("Empty username correctly failed: %v", err)
}

func (s *loginTestState) testEmptyPassword(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      "",
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "empty password should fail")
	require.Contains(t, err.Error(), "password required but not provided")
	t.Logf("Empty password correctly failed: %v", err)
}

func (s *loginTestState) testInvalidControllerURL(t *testing.T) {
	hostErr := "i/o timeout"
	if runtime.GOOS == "windows" { //because of course it's different on linux/windows
		hostErr = "no such host"
	}
	t.Run("not-a-url", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "not-a-url", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), hostErr)
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("http://[invalid", func(t *testing.T) {
		opts := &edge.LoginOptions{
			Options:       s.commonOpts,
			Username:      s.controllerUnderTest.Username,
			Password:      s.controllerUnderTest.Password,
			ControllerUrl: "http://[invalid",
			Yes:           true,
			IgnoreConfig:  true,
			NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
		}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid controller URL")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("ftp://wrong-scheme.com", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "ftp://wrong-scheme.com", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), hostErr)
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("https://non-existent-host-12345.local:9999", func(t *testing.T) {
		dnsErr := "no such host"
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "https://non-existent-host-12345.local:9999", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), dnsErr)
		t.Logf("Invalid URL correctly failed: %v", err)
	})
}

func (s *loginTestState) testNonExistentUsername(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      "nonexistent-user-12345",
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: s.controllerUnderTest.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "non-existent username should fail")
	require.Contains(t, err.Error(), "401 Unauthorized")
	t.Logf("Non-existent username correctly failed: %v", err)
}

func (s *loginTestState) testControllerUnavailable(t *testing.T) {
	expectedErr := "connection refused"
	if runtime.GOOS == "windows" { //because of course it's different on linux/windows
		expectedErr = "the target machine actively refused it"
	}
	opts := &edge.LoginOptions{
		Options:       s.commonOpts,
		Username:      s.controllerUnderTest.Username,
		Password:      s.controllerUnderTest.Password,
		ControllerUrl: "https://127.0.0.1:9999",
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     s.controllerUnderTest.NetworkDialingIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "unavailable controller should fail")
	require.Contains(t, err.Error(), expectedErr)
	t.Logf("Unavailable controller correctly failed: %v", err)
}

func (s *loginTestState) reconfigureTargetForZiti(pkiRoot string) error {
	v2 := cmd.NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	v2.SetArgs(strings.Split("pki create server --key-file server --pki-root "+pkiRoot+" --ip 127.0.0.1,::1 --dns localhost,mgmt.ziti --ca-name intermediate-ca-quickstart --server-file mgmt.ziti", " "))
	if zitiCmdErr := v2.Execute(); zitiCmdErr != nil {
		return zitiCmdErr
	}
	return nil
}

func (s *loginTestState) loginTestsOverZiti(t *testing.T, now, zitiPath string) {
	t.Run("login tests over ziti", func(t *testing.T) {
		pkiRoot := gopath.Join(s.controllerUnderTest.Home, "pki")
		if reconfErr := s.reconfigureTargetForZiti(pkiRoot); reconfErr != nil {
			t.Fatalf("failed to reconfigure target: %v", reconfErr)
		}

		if ie := s.externalZiti.CreateOverlayIdentities(t, now); ie != nil {
			t.Fatalf("failed to initialize ziti transport for controllerUnderTest: %v", ie)
		}
		s.controllerUnderTest.NetworkDialingIdFile = s.externalZiti.NetworkDialingIdFile
		s.controllerUnderTest.NetworkBindingIdFile = s.externalZiti.NetworkBindingIdFile

		controllerUnderTestCtx2, controllerUnderTestCancel := context.WithCancel(context.Background())
		defer controllerUnderTestCancel()
		s.controllerUnderTest.Ctx = controllerUnderTestCtx2
		s.controllerUnderTest.ConfigFile = gopath.Join(s.controllerUnderTest.Home, "ctrl.yaml")
		newServerCertPath := gopath.Join(s.controllerUnderTest.Home, "pki/intermediate-ca-quickstart/certs/mgmt.ziti.chain.pem")
		if re := s.controllerUnderTest.ReplaceConfig(newServerCertPath); re != nil {
			t.Fatalf("failed to replace config: %v", re)
		}

		targetDone := make(chan error)
		s.controllerUnderTest.StartExternal(zitiPath, targetDone)
		go func() {
			qsErr := <-targetDone
			if qsErr == nil {
				controllerUnderTestCancel()
				t.Fatal("unexpected error from external quickstart?")
			}
		}()
		s.controllerUnderTest.WaitForControllerReadyorig(t, nil)
		s.controllerUnderTest.ControllerAddress = "mgmt.ziti"
		s.controllerUnderTest.ControllerPort = 443

		s.runLoginTests(t)

		controllerUnderTestCancel()
	})
}

func (s *loginTestState) runLoginTests(t *testing.T) {
	//Authentication Methods
	t.Run("correct password succeeds", s.testCorrectPasswordSucceeds)
	t.Run("wrong password fails", s.testWrongPasswordFails)
	t.Run("token based login", s.testTokenBasedLogin)
	t.Run("client cert authentication - no ca", s.testClientCertAuthentication)
	t.Run("identity file authentication", s.testIdentityFileAuthentication)
	t.Run("external JWT authentication", s.testExternalJWTAuthentication)
	t.Run("network identity zitified connection", s.testNetworkIdentityZitifiedConnection)

	// Edge Cases
	t.Run("empty username", s.testEmptyUsername)
	t.Run("empty password", s.testEmptyPassword)
	t.Run("invalid controller URL", s.testInvalidControllerURL)
	t.Run("non-existent username", s.testNonExistentUsername)
	t.Run("controller unavailable", s.testControllerUnavailable)
}
