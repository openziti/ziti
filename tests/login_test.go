//go:build apitests

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
	"context"
	"net/http"
	"os"
	gopath "path"
	"path/filepath"
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
	zitiDir := filepath.Join(s.homeDir, ".ziti")
	if err := os.RemoveAll(zitiDir); err != nil {
		t.Errorf("remove %s: %v", zitiDir, err)
		t.Fail()
	}
	t.Logf("Removed ziti dir from: %s", zitiDir)
}

func Test_LoginSuite(t *testing.T) {
	zitiPath := `D:\git\github\openziti\nf\ziti\build\ziti.exe` //need to fix xxxx
	baseDir := filepath.Join(os.TempDir(), "cli-tests")
	if me := os.MkdirAll(baseDir, 0755); me != nil {
		t.Fatalf("failed creating baseDir dir: %v", baseDir)
	}
	targetHome, err := os.MkdirTemp(baseDir, "test-run-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}

	externalCtx, externalCancel := context.WithCancel(context.Background())
	defer externalCancel()
	ctrlUnderTestCtx, ctrlUnderTestCancel := context.WithCancel(context.Background())
	defer ctrlUnderTestCancel()

	testState := &loginTestState{
		homeDir:             util.HomeDir(),
		zitiContext:         nil,
		zitiTransport:       nil,
		externalZiti:        testutil.CreateOverlay(t, externalCtx, 0, gopath.Join(targetHome, "external")),
		controllerUnderTest: testutil.CreateOverlay(t, ctrlUnderTestCtx, 330*time.Second, gopath.Join(targetHome, "target")),
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
				t.Logf("manual cleanup may be required at %s", targetHome)
			} else {
				t.Logf("tests passed, removing temp dir at %s", targetHome)
				if rerr := os.RemoveAll(targetHome); rerr != nil {
					t.Logf("remove %s failed... **sigh**: %v", targetHome, rerr)
				}
			}
		} else {
			t.Logf("tests failed, temp dir left intact at %s", targetHome)
		}
		testState.externalZiti.CleanupPids()
		testState.controllerUnderTest.CleanupPids()
	}()

	// set ZITI_HOME so that anything here forth is not corrupting local stuff
	_ = os.Setenv("ZITI_HOME", targetHome)

	testState.externalZiti.ControllerAddress = "localhost"
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

	if ae := testState.controllerUnderTest.CreateAdminIdentity(t, now, targetHome); ae != nil {
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
func (s *loginTestState) test_CorrectPasswordSucceeds(t *testing.T) {
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

func (s *loginTestState) test_WrongPasswordFails(t *testing.T) {
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

func (s *loginTestState) test_TokenBasedLogin(t *testing.T) {
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

func (s *loginTestState) test_ClientCertAuthentication(t *testing.T) {
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

func (s *loginTestState) test_IdentityFileAuthentication(t *testing.T) {
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

func (s *loginTestState) test_ExternalJWTAuthentication(t *testing.T) {
	// TODO: Generate valid JWT token
	t.Skip("External JWT authentication requires JWT setup")
}

func (s *loginTestState) test_NetworkIdentityZitifiedConnection(t *testing.T) {
	// TODO: Create network identity file
	t.Skip("Network identity requires identity setup")
}

// Edge Cases
func (s *loginTestState) test_EmptyUsername(t *testing.T) {
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

func (s *loginTestState) test_EmptyPassword(t *testing.T) {
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

func (s *loginTestState) test_InvalidControllerURL(t *testing.T) {
	t.Run("not-a-url", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "not-a-url", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
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
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("https://non-existent-host-12345.local:9999", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: s.commonOpts, Username: s.controllerUnderTest.Username, Password: s.controllerUnderTest.Password,
			ControllerUrl: "https://non-existent-host-12345.local:9999", Yes: true, IgnoreConfig: true,
			NetworkId: s.controllerUnderTest.NetworkDialingIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})
}

func (s *loginTestState) test_NonExistentUsername(t *testing.T) {
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

func (s *loginTestState) test_ControllerUnavailable(t *testing.T) {
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
	require.Contains(t, err.Error(), "the target machine actively refused it")
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
	t.Run("correct password succeeds", s.test_CorrectPasswordSucceeds)
	t.Run("wrong password fails", s.test_WrongPasswordFails)
	t.Run("token based login", s.test_TokenBasedLogin)
	t.Run("client cert authentication - no ca", s.test_ClientCertAuthentication)
	t.Run("identity file authentication", s.test_IdentityFileAuthentication)
	t.Run("external JWT authentication", s.test_ExternalJWTAuthentication)
	t.Run("network identity zitified connection", s.test_NetworkIdentityZitifiedConnection)

	// Edge Cases
	t.Run("empty username", s.test_EmptyUsername)
	t.Run("empty password", s.test_EmptyPassword)
	t.Run("invalid controller URL", s.test_InvalidControllerURL)
	t.Run("non-existent username", s.test_NonExistentUsername)
	t.Run("controller unavailable", s.test_ControllerUnavailable)
}
