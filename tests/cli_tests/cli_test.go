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
	"io"
	"net/http"
	"os"
	gopath "path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/tests/testutil"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/util"
	"github.com/stretchr/testify/require"
)

type cliTestState struct {
	homeDir             string
	zitiContext         *ziti.Context
	zitiTransport       *http.Transport
	commonOpts          api.Options
	externalZiti        testutil.Overlay
	controllerUnderTest testutil.Overlay
}

func (s *cliTestState) removeZitiDir(t *testing.T) {
	zitiDir, _ := util.ConfigDir()
	if err := os.RemoveAll(zitiDir); err != nil {
		t.Errorf("remove %s: %v", zitiDir, err)
		t.Fail()
	}
	t.Logf("Removed ziti dir from: %s", zitiDir)
}

func Test_CLI_Test_Suite(t *testing.T) {
	zitiPath := os.Getenv("ZITI_CLI_TEST_ZITI_BIN")
	if zitiPath == "" {
		t.Fatalf("ZITI_CLI_TEST_ZITI_BIN not set")
	}
	if _, statErr := os.Stat(zitiPath); statErr != nil {
		t.Fatalf("ziti binary not found at provided location %s: %v", zitiPath, statErr)
	}
	baseDir := filepath.Join(os.TempDir(), "cli-tests")
	if me := os.MkdirAll(baseDir, 0755); me != nil {
		t.Fatalf("failed creating baseDir dir: %v", baseDir)
	}
	testRunHome, mkdirErr := os.MkdirTemp(baseDir, "test-run-*")
	if mkdirErr != nil {
		t.Fatalf("failed creating temp dir: %v", mkdirErr)
	}

	//testRunHome = filepath.Join(baseDir, "persistent")
	os.MkdirAll(testRunHome, 0755)
	// set ZITI_CONFIG_DIR so that anything here forth is not corrupting local stuff
	cfgDir := filepath.Join(testRunHome, ".config/underlay")
	t.Logf("ZITI_CONFIG_DIR: %s", cfgDir)
	_ = os.Setenv("ZITI_CONFIG_DIR", cfgDir)
	_ = os.RemoveAll(cfgDir)
	externalCtx, externalCancel := context.WithCancel(context.Background())
	defer externalCancel()
	ctrlUnderTestCtx, ctrlUnderTestCancel := context.WithCancel(context.Background())
	defer ctrlUnderTestCancel()

	testState := &cliTestState{
		homeDir:             testRunHome,
		zitiContext:         nil,
		zitiTransport:       nil,
		externalZiti:        testutil.CreateOverlay(t, externalCtx, 600*time.Second, testRunHome, "external", false),
		controllerUnderTest: testutil.CreateOverlay(t, ctrlUnderTestCtx, 600*time.Second, testRunHome, "target", false),
		commonOpts: api.Options{
			CommonOptions: common.CommonOptions{
				Out: os.Stdout,
				Err: os.Stderr,
			},
		},
	}

	//testState.externalZiti.ControllerPort = 3000
	//testState.externalZiti.RouterPort = 3001
	//testState.controllerUnderTest.ControllerPort = 4000
	//testState.controllerUnderTest.RouterPort = 4001

	defer func() {
		if !t.Failed() {
			// allow/ensure the processes to exit windows is a pain about rm'ing folders if not
			errChan := make(chan error, 2)

			go func() { errChan <- testState.externalZiti.Stop() }()
			go func() { errChan <- testState.controllerUnderTest.Stop() }()
			success := true
			for i := 0; i < 2; i++ { // Wait for both
				if deferErr := <-errChan; deferErr != nil {
					t.Logf("stop error: %v", deferErr)
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
	targetDone := make(chan error)
	go testState.controllerUnderTest.StartExternal(zitiPath, targetDone)

	exStartErr := testState.externalZiti.WaitForControllerReady(20 * time.Second)
	if exStartErr != nil {
		log.Fatalf("externalZiti start failed: %v", exStartErr)
	}

	cutStartErr := testState.controllerUnderTest.WaitForControllerReady(20 * time.Second)
	if cutStartErr != nil {
		log.Fatalf("controllerUnderTest start failed: %v", cutStartErr)
	}

	if lo, le := testState.controllerUnderTest.Login(); le != nil {
		t.Fatalf("unable to login before running tests: %v", le)
	} else {
		testState.controllerUnderTest.ApiSession = lo.ApiSession
	}

	require.NotEmpty(t, testState.controllerUnderTest.ApiSession)
	require.NotEmpty(t, testState.controllerUnderTest.ApiSession.GetToken())

	now := time.Now().Format("150405")

	if ae := testState.controllerUnderTest.CreateAdminIdentity(t, now, testRunHome); ae != nil {
		t.Fatalf("unable to create controller admin: %v", ae)
	}

	t.Log("====================================================================================")
	t.Log("=========================== overlay ready. tests begin =============================")
	t.Log("====================================================================================")

	testTimeout := 120 * time.Second
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
		require.NotEmpty(t, lr.ApiSession)
		testState.controllerUnderTest.ApiSession = lr.ApiSession
	}

	t.Run("cli tests over underlay", testState.cliTests)

	t.Log("Cancelling controllerUnderTest to reconfigure for use with ziti")
	ctrlUnderTestCancel()
	if se := testState.controllerUnderTest.Stop(); se != nil {
		t.Fatalf("controllerUnderTest didn't stop? %v", se)
	}
	t.Log("Cancelling controllerUnderTest complete")

	s := testState
	pkiRoot := gopath.Join(s.controllerUnderTest.Home, "pki")
	if reconfErr := s.reconfigureTargetForZiti(pkiRoot); reconfErr != nil {
		t.Fatalf("failed to reconfigure target: %v", reconfErr)
	}

	if ie := s.externalZiti.CreateOverlayIdentities(t, now); ie != nil {
		t.Fatalf("failed to initialize ziti transport for controllerUnderTest: %v", ie)
	}
	s.controllerUnderTest.NetworkDialingIdFile = s.externalZiti.NetworkDialingIdFile
	s.controllerUnderTest.NetworkBindingIdFile = s.externalZiti.NetworkBindingIdFile

	s.controllerUnderTest.ConfigFile = gopath.Join(s.controllerUnderTest.Home, "ctrl.yaml")
	newServerCertPath := gopath.Join(s.controllerUnderTest.Home, "pki/intermediate-ca-quickstart/certs/mgmt.ziti.chain.pem")
	if re := s.controllerUnderTest.ReplaceConfig(newServerCertPath); re != nil {
		t.Fatalf("failed to replace config: %v", re)
	}

	controllerUnderTestCtx2, cutOverZitiCancel := context.WithCancel(context.Background())
	defer cutOverZitiCancel()
	s.controllerUnderTest.Ctx = controllerUnderTestCtx2

	targetOverZitiDone := make(chan error)
	go s.controllerUnderTest.StartExternal(zitiPath, targetOverZitiDone)
	cutStartOverZitiErr := s.controllerUnderTest.WaitForControllerReady(20 * time.Second)
	if cutStartErr != nil {
		log.Fatalf("controllerUnderTest start failed: %v", cutStartOverZitiErr)
	}
	testState.cliTestsOverZiti(t, zitiPath)

	s.controllerUnderTest.ControllerName = s.externalZiti.ControllerName
	testState.cliTestsOverAddressableTerminators(t, zitiPath)

	cutOverZitiCancel()
	externalCancel()

	test(t, "make sure any ziti instances are stopped", testState.externalZiti.EnsureAllPidsStopped)
	test(t, "make sure any ziti instances are stopped", testState.controllerUnderTest.EnsureAllPidsStopped)
}

func (s *cliTestState) cliTests(t *testing.T) {
	t.Run("Login Tests", s.loginTests)
	s.controllerUnderTest.PrintLoginCommand(t)
	s.testCorrectPasswordSucceeds(t)
	time.Sleep(1 * time.Second)
	t.Run("Fabric Tests", s.fabricTests)
}

func (s *cliTestState) runCLI(cmdToRun string) (string, error) {
	return s.runCLIWithContext(context.Background(), cmdToRun)
}

func (s *cliTestState) runCLIWithContext(ctx context.Context, cmdToRun string) (string, error) {
	orig := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	v2 := cmd.NewRootCommand(os.Stdin, w, w)
	v2.SetArgs(strings.Split(cmdToRun, " "))
	v2.SetContext(ctx)

	err := v2.Execute()

	_ = w.Close()
	os.Stdout = orig

	outBytes, _ := io.ReadAll(r)
	return string(outBytes), err
}

func test(t *testing.T, name string, fn func(*testing.T)) {
	testWithTimeout(t, name, fn, 100*time.Second)
}

func testWithTimeout(t *testing.T, name string, fn func(*testing.T), d time.Duration) {
	t.Helper()

	t.Run(name, func(t *testing.T) {
		done := make(chan struct{})
		go func() {
			fn(t)
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(d):
			t.Fatal("timeout")
		}
	})
}
