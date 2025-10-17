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
package tests

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	gopath "path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_util"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/ops"
	"github.com/openziti/ziti/ziti/run"
	"github.com/openziti/ziti/ziti/util"
	"github.com/stretchr/testify/require"
)

const (
	username = "admin"
	password = "admin"
)

var activePids []int
var pidsMutex sync.Mutex
var commonOpts = api.Options{
	CommonOptions: common.CommonOptions{
		Out: os.Stdout,
		Err: os.Stderr,
	},
}
var token string
var homeDir = util.HomeDir()
var tmpDir string
var controllerIdFile string
var adminCertFile string
var adminCaFile string
var adminKeyFile string
var adminIdFile string
var networkIdFile string
var networkClientIdFile string
var zitiContext *ziti.Context
var zitiTransport *http.Transport

var externalZiti overlay
var controllerUnderTest overlay

func findAvailablePort(t *testing.T) uint16 {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return (uint16)(listener.Addr().(*net.TCPAddr).Port)
}

func removeZitiDir(t *testing.T) {
	zitiDir := filepath.Join(homeDir, ".ziti")
	if err := os.RemoveAll(zitiDir); err != nil {
		t.Errorf("remove %s: %v", zitiDir, err)
		t.Fail()
	}
	t.Logf("Removed ziti dir from: %s", zitiDir)
}

func (o *overlay) waitForControllerReady(t *testing.T, cmdComplete chan error) {
	t.Logf("Waiting for controller at %s", o.controllerHostPort())
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	itls := &tls.Config{InsecureSkipVerify: true}
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: itls,
		},
	}
	if ot, ok := http.DefaultClient.Transport.(*http.Transport); ok {
		it := ot.Clone()
		it.TLSClientConfig = itls
		http.DefaultClient.Transport = it
		defer func() {
			http.DefaultClient.Transport = ot
		}()
	}

	for range ticker.C {
		testUrl := o.controllerHostPort() + "/.well-known/est/cacerts"
		t.Logf("Waiting for controller at %s", testUrl)
		resp, err := client.Get(testUrl)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			t.Logf("Controller ready at %s", o.controllerHostPort())
			cmdComplete <- nil
			return
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
	}
}

func (o *overlay) waitForControllerReadyorig(t *testing.T, cmdComplete chan error) {
	t.Logf("Waiting for controller at %s\n", o.controllerHostPort())
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_, err := rest_util.GetControllerWellKnownCas(o.controllerHostPort())
			if err == nil {
				t.Logf("Controller ready at %s", o.controllerHostPort())
				if cmdComplete != nil {
					cmdComplete <- nil
				}
				return
			}
		}
	}
}

func waitForRouter(t *testing.T, address string, port int, done chan struct{}) {
	addr := fmt.Sprintf("%s:%d", address, port)
	fmt.Printf("Waiting for router at %s\n", addr)
	for {
		var err error
		var conn net.Conn
		if zitiContext != nil {
			zc := *zitiContext
			conn, err = zc.DialAddr("tcp", addr)
		} else {
			conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
		}
		if err == nil {
			_ = conn.Close()
			t.Logf("Router is available on %s:%d\n", address, port)
			close(done)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

type overlay struct {
	t            *testing.T
	ctx          context.Context
	startTimeout time.Duration
	name         string
	extCmd       *exec.Cmd
	cmdDone      chan error
	pid          int
	*run.QuickstartOpts
}

func (o *overlay) controllerHostPort() string {
	return fmt.Sprintf("https://%s:%d", o.ControllerAddress, o.ControllerPort)
}
func (o *overlay) routerHostPort() string {
	return fmt.Sprintf("https://%s:%d", o.RouterAddress, o.RouterPort)
}

func createOverlay(t *testing.T, ctx context.Context, startTimeout time.Duration, home string) overlay {
	o := overlay{
		t:            t,
		ctx:          ctx,
		startTimeout: startTimeout,
		cmdDone:      make(chan error, 1),
		QuickstartOpts: &run.QuickstartOpts{
			Home:              home,
			ControllerAddress: "localhost", //helpers.GetCtrlAdvertisedAddress(),
			ControllerPort:    findAvailablePort(t),
			RouterAddress:     "localhost", //helpers.GetRouterAdvertisedAddress(),
			RouterPort:        findAvailablePort(t),
			Routerless:        false,
			TrustDomain:       "target",
			InstanceID:        "target",
			IsHA:              false,
			Username:          "username",
			Password:          "password",
			ConfigureAndExit:  false,
		},
	}

	return o
}

func (o *overlay) startArgs() []string {
	args := []string{
		fmt.Sprintf("--home=%s", o.Home),
		fmt.Sprintf("--ctrl-address=%s", o.ControllerAddress),
		fmt.Sprintf("--ctrl-port=%d", o.ControllerPort),
		fmt.Sprintf("--router-address=%s", o.RouterAddress),
		fmt.Sprintf("--router-port=%d", o.RouterPort),
	}
	if o.Routerless {
		args = append(args, "--no-router")
	}
	if o.ConfigureAndExit {
		args = append(args, "--configure-and-exit")
	}
	return args
}

func (o *overlay) waitForCompletion() error {
	done := make(chan error, 1)
	go func() {
		done <- o.extCmd.Wait()
	}()

	select {
	case err := <-done:
		if errors.Is(err, context.Canceled) ||
			(err != nil && strings.Contains(err.Error(), "signal killed")) {
			return nil
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("timeout waiting for command completion")
	}
}

func (o *overlay) start(ctx context.Context) error {
	qs := run.NewQuickStartCmd(os.Stdout, os.Stderr, o.ctx)
	qs.SetContext(ctx)
	qs.SetArgs(o.startArgs())

	//o.cmdDone = make(chan error, 1)
	go func() {
		o.cmdDone <- qs.Execute()
		fmt.Println("done?")
	}()

	if o.ConfigureAndExit {
		//err := <-o.cmdDone
		//if err != nil {
		//	o.t.Fatalf("error executing quickstart: %v", err)
		//}
		//return nil // expect the controller and router will be shutting down, don't wait for them
		err := o.waitForCompletion()
		if err != nil {
			o.t.Fatalf("error executing quickstart: %v", err)
		}
	}

	cmdComplete := make(chan error)
	go o.waitForControllerReady(o.t, cmdComplete)
	select {
	case err := <-cmdComplete:
		if err != nil {
			return err
		}
		if !o.Routerless {
			go o.QuickstartOpts.WaitForRouter(cmdComplete)
			select {
			case err := <-cmdComplete:
				return err
			case <-time.After(o.startTimeout):
				return fmt.Errorf("timed out waiting for controller")
			}
		}
		return nil
	case <-time.After(o.startTimeout):
		return fmt.Errorf("timed out waiting for controller")
	}
}

func TestLoginSuite(t *testing.T) {
	zitiPath := `D:\git\github\openziti\nf\ziti\build\ziti.exe`
	baseDir := filepath.Join(os.TempDir(), "cli-tests")
	if me := os.MkdirAll(baseDir, 0755); me != nil {
		t.Fatalf("failed creating baseDir dir: %v", baseDir)
	}
	targetHome, err := os.MkdirTemp(baseDir, "test-run-*")
	if err != nil {
		t.Fatalf("failed creating temp dir: %v", err)
	}
	defer func() {
		if !t.Failed() {
			// allow/ensure the processes to exit windows is a pain about rm'ing folders if not
			errChan := make(chan error, 2)

			go func() { errChan <- externalZiti.stop() }()
			go func() { errChan <- controllerUnderTest.stop() }()
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
		cleanupPids()
	}()

	// set ZITI_HOME so that anything here forth is not corrupting local stuff
	_ = os.Setenv("ZITI_HOME", targetHome)

	externalCtx, externalCancel := context.WithCancel(context.Background())
	defer externalCancel()
	externalZiti = createOverlay(t, externalCtx, 0, gopath.Join(targetHome, "external"))
	//xxx externalZiti.ControllerPort = 4000
	externalZiti.ControllerAddress = "localhost"
	extDone := make(chan error)
	go externalZiti.startExternal(zitiPath, extDone)
	go func() {
		qsErr := <-extDone
		if qsErr == nil {
			externalCancel()
			t.Fatal("unexpected error from external quickstart?")
		}
	}()

	externalZiti.waitForControllerReadyorig(t, nil)

	targetCtx1, ctrlUnderTestCancel := context.WithCancel(context.Background())
	defer ctrlUnderTestCancel()
	controllerUnderTest = createOverlay(t, targetCtx1, 330*time.Second, gopath.Join(targetHome, "target"))
	//xxx controllerUnderTest.ControllerPort = 2000
	//xxx controllerUnderTest.RouterPort = 2001
	controllerUnderTest.name = "target"
	targetDone := make(chan error)
	go controllerUnderTest.startExternal(zitiPath, targetDone)
	go func() {
		qsErr := <-targetDone
		if qsErr == nil {
			ctrlUnderTestCancel()
			t.Fatal("unexpected error from external quickstart?")
		}
	}()
	controllerUnderTest.waitForControllerReadyorig(t, nil)
	if _, le := controllerUnderTest.Login(); le != nil {
		t.Fatalf("unable to login before running tests: %v", le)
	}
	t.Logf("Target controller at: %s", controllerUnderTest.controllerHostPort())

	now := time.Now().Format("150405")

	if ae := controllerUnderTest.createTestAdmin(t, now, targetHome); ae != nil {
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

	if lr, le := controllerUnderTest.Login(); le != nil {
		t.Fatalf("unable to login before running tests: %v", le)
	} else {
		//set the valid token for reuse later:
		require.NotEmpty(t, lr.Token)
		token = lr.Token
	}

	t.Run("login tests over underlay", runLoginTests)

	t.Log("Cancelling controllerUnderTest to reconfigure for use with ziti")

	ctrlUnderTestCancel()
	if se := controllerUnderTest.stop(); se != nil {
		t.Fatalf("controllerUnderTest didn't stop? %v", se)
	}
	t.Log("Cancelling controllerUnderTest complete")
	loginTestsOverZiti(t, now, zitiPath, externalZiti)
	externalCancel()

	t.Run("make sure any ziti instances are stopped", ensureAllPidsStopped)
}

func newTestLoginOpts() edge.LoginOptions {
	return edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      password,
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		NetworkId:     networkIdFile,
	}
}

// Authentication Methods
func testCorrectPasswordSucceeds(t *testing.T) {
	opts := newTestLoginOpts()

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

func testWrongPasswordFails(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      username,
		Password:      "wrong-password",
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     networkIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "login with wrong password should fail")
	require.Contains(t, err.Error(), "401 Unauthorized")
}

func testTokenBasedLogin(t *testing.T) {
	opts := &edge.LoginOptions{
		Options:       commonOpts,
		Token:         token,
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		IgnoreConfig:  true,
		NetworkId:     networkIdFile,
	}

	err := opts.Run()
	require.NoError(t, err)
	require.Equal(t, token, opts.Token)
	t.Logf("Login successful, token: %s", opts.Token)
}

func testClientCertAuthentication(t *testing.T) {
	// Setup common options
	baseOpts := edge.LoginOptions{
		Options:       commonOpts,
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		ClientCert:    adminCertFile,
		ClientKey:     adminKeyFile,
		CaCert:        adminCaFile,
		NetworkId:     networkIdFile,
	}

	removeZitiDir(t)
	t.Run("all present", func(t *testing.T) {
		opts := baseOpts

		err := opts.Run()
		require.NoError(t, err, "login with cert/key/ca when all present should succeed")
		require.NotEmpty(t, opts.Token)
		t.Logf("Login successful, token: %s", opts.Token)
	})

	removeZitiDir(t)
	t.Run("no cert", func(t *testing.T) {
		opts := baseOpts
		opts.ClientCert = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client cert is missing")
		require.Contains(t, err.Error(), "username required but not provided")
	})

	removeZitiDir(t)
	t.Run("no key", func(t *testing.T) {
		opts := baseOpts
		opts.ClientKey = ""

		err := opts.Run()
		require.Error(t, err, "expected error when client key is missing")
		require.Contains(t, err.Error(), "can't load client certificate")
	})

	removeZitiDir(t)
	t.Run("no CA cert with yes flag", func(t *testing.T) {
		opts := baseOpts
		opts.CaCert = ""
		opts.Yes = true

		err := opts.Run()
		require.NoError(t, err, "expected success when CA cert is missing and IgnoreConfig is enabled and 'Yes' is true")
		require.NotEmpty(t, opts.Token)
		t.Logf("Login successful, token: %s", opts.Token)
	})

	removeZitiDir(t)
	t.Run("no CA cert without yes flag", func(t *testing.T) {
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
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		File:          adminIdFile,
		NetworkId:     networkIdFile,
	}

	err := opts.Run()
	require.NoError(t, err)

	client, err := opts.NewMgmtClient()
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotEmpty(t, opts.Token)
	t.Logf("Login successful, token: %s", opts.Token)
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
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     networkIdFile,
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
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     networkIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "empty password should fail")
	require.Contains(t, err.Error(), "password required but not provided")
	t.Logf("Empty password correctly failed: %v", err)
}

func testInvalidControllerURL(t *testing.T) {
	t.Run("not-a-url", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "not-a-url", Yes: true, IgnoreConfig: true,
			NetworkId: networkIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("http://[invalid", func(t *testing.T) {
		opts := &edge.LoginOptions{
			Options:       commonOpts,
			Username:      username,
			Password:      password,
			ControllerUrl: "http://[invalid",
			Yes:           true,
			IgnoreConfig:  true,
			NetworkId:     networkIdFile,
		}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid controller URL")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("ftp://wrong-scheme.com", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "ftp://wrong-scheme.com", Yes: true, IgnoreConfig: true,
			NetworkId: networkIdFile}
		err := opts.Run()
		require.Error(t, err)
		require.Contains(t, err.Error(), "no such host")
		t.Logf("Invalid URL correctly failed: %v", err)
	})

	t.Run("https://non-existent-host-12345.local:9999", func(t *testing.T) {
		opts := &edge.LoginOptions{Options: commonOpts, Username: username, Password: password,
			ControllerUrl: "https://non-existent-host-12345.local:9999", Yes: true, IgnoreConfig: true,
			NetworkId: networkIdFile}
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
		ControllerUrl: controllerUnderTest.controllerHostPort(),
		Yes:           true,
		IgnoreConfig:  true,
		NetworkId:     networkIdFile,
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
		NetworkId:     networkIdFile,
	}

	err := opts.Run()
	require.Error(t, err, "unavailable controller should fail")
	require.Contains(t, err.Error(), "the target machine actively refused it")
	t.Logf("Unavailable controller correctly failed: %v", err)
}

func (o *overlay) replaceConfig1(t *testing.T, idFile string) {
	content, _ := os.ReadFile(o.QuickstartOpts.ConfigFile)
	re := regexp.MustCompile(`(?s)bindPoints:.*?\n    identity:`)
	newBp := `bindPoints:
      - identity:
          file: ` + idFile + `
          service: "mgmt.tls"
          serveTLS: true
    identity:`

	newContent := re.ReplaceAllString(string(content), newBp)

	if we := os.WriteFile(o.QuickstartOpts.ConfigFile, []byte(newContent), 0644); we != nil {
		t.Fatal("failed to write new content to target controller file", we)
	}
}

func (o *overlay) replaceConfig(newServerCertPath string) error {
	content, err := os.ReadFile(o.QuickstartOpts.ConfigFile)
	if err != nil {
		return err
	}
	newContent := string(content)

	// remove edge-management section
	reEdgeMgmt := regexp.MustCompile(`(?m)^ *- binding: edge-management[\s\S]+?options: \{\ }\n`)
	newContent = reEdgeMgmt.ReplaceAllString(newContent, "")

	// replace "- binding: fabric" block with single "-"
	reFabric := regexp.MustCompile(`(?m)- binding: fabric[\s\S]+?options: \{\ }\n {6}-`)
	newContent = reFabric.ReplaceAllString(newContent, "-")

	reServerCert := regexp.MustCompile(`(?m)^( *)(server_cert:.*server.chain.pem")$`)
	newContent = reServerCert.ReplaceAllString(newContent, "$1#$2\n${1}server_cert: "+newServerCertPath)

	newContent = newContent + `
  - name: secured-by-ziti-http
    bindPoints:
      - identity:
          file: ` + controllerIdFile + `
          service: "mgmt"
          serveTLS: true
    apis:
      - binding: edge-management
        options: { }
      - binding: fabric
        options: { }
      - binding: zac
        options:
          location: "/ctrl/zac/ziti-console-v3.12.5"
          indexFile: index.html`
	if we := os.WriteFile(o.QuickstartOpts.ConfigFile, []byte(newContent), 0644); we != nil {
		return fmt.Errorf("failed to write new content to target controller file: %v", we)
	} else {
		fmt.Println("CHANGED FILE AT : " + o.QuickstartOpts.ConfigFile)
	}
	return nil
}

func (o *overlay) startExternal(zitiPath string, done chan error) {
	args := append([]string{"edge", "quickstart"}, o.startArgs()...)
	o.extCmd = exec.CommandContext(
		o.ctx,
		zitiPath,
		args...,
	)
	_ = os.Mkdir(o.Home, 0755)
	stdoutFile, err := os.Create(filepath.Join(o.Home, "ctrl-stdout.log"))
	if err != nil {
		done <- err
		return
	}
	stderrFile, err := os.Create(filepath.Join(o.Home, "ctrl-stderr.log"))
	if err != nil {
		_ = stdoutFile.Close()
		done <- err
		return
	}
	o.extCmd.Stdout = stdoutFile
	o.extCmd.Stderr = stderrFile

	fmt.Printf("ctrl logs at: %s\n", filepath.Join(o.Home, "ctrl-stdout.log"))
	if err := o.extCmd.Start(); err != nil {
		done <- err
		return
	}

	fmt.Printf("started ziti quickstart (pid=%d)\n", o.extCmd.Process.Pid)
	trackPid(o.extCmd.Process.Pid)

	go func() {
		err := o.extCmd.Wait()
		if errors.Is(err, context.Canceled) || (err != nil && strings.Contains(err.Error(), "signal killed")) {
			err = nil
		}
		done <- err
	}()
}

func (o *overlay) createTestAdmin(t *testing.T, now, baseDir string) error {
	if lr, le := controllerUnderTest.Login(); le != nil {
		return le
	} else {
		//set the valid token for reuse later:
		require.NotEmpty(t, lr.Token)
		token = lr.Token
	}
	adminIdName := fmt.Sprintf("test-admin-%s", now)
	adminJwtPath := filepath.Join(baseDir, adminIdName+".jwt")
	zitiCmd := edge.NewCmdEdge(os.Stdout, os.Stderr, common.NewOptionsProvider(os.Stdout, os.Stderr))
	zitiCmd.SetArgs(strings.Split("create identity "+adminIdName+" -o "+adminJwtPath+" --admin", " "))
	if zitiCmdErr := zitiCmd.Execute(); zitiCmdErr != nil {
		t.Fatalf("unable to create identity: %v", zitiCmdErr)
	}
	zitiCmd.SetArgs([]string{"enroll", adminJwtPath})
	if zitiCmdErr := zitiCmd.Execute(); zitiCmdErr != nil {
		t.Fatalf("unable to create identity: %v", zitiCmdErr)
	}
	adminIdFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".json"
	t.Logf("identity file should exist at: %v", adminIdFile)

	unwrapCmd := ops.NewUnwrapIdentityFileCommand(os.Stdout, os.Stderr)
	unwrapCmd.SetArgs([]string{adminIdFile})
	unwrapCmdErr := unwrapCmd.Execute()
	if unwrapCmdErr != nil {
		t.Fatalf("unable to unwrap identity: %v", unwrapCmdErr)
	}
	adminCertFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".cert"
	adminCaFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".ca"
	adminKeyFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".key"
	t.Logf("certfile should exist at: %v", adminCertFile)
	t.Logf("caFile should exist at: %v", adminCaFile)
	t.Logf("keyFile should exist at: %v", adminKeyFile)
	return nil
}

func (o *overlay) initializeZitiTransportOverlay(t *testing.T, now string, transportOverlay overlay) error {
	p := common.NewOptionsProvider(os.Stdout, os.Stderr)
	if _, le := transportOverlay.Login(); le != nil {
		return le
	}
	v2a := cmd.NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	v2a.SetArgs(strings.Split("ops import --username "+username+" --password "+password+" ./login_test_import.yml", " "))
	if v2Err := v2a.Execute(); v2Err != nil {
		t.Fatalf("unable to import zitified-login-test.yml: %v", v2Err)
	}

	controllerIdName := fmt.Sprintf("controller-binder-%s", now)
	controllerJwtPath := filepath.Join(o.Home, controllerIdName+".jwt")
	zitiCmd1 := edge.NewCmdEdge(os.Stdout, os.Stderr, p)
	zitiCmd1.SetArgs(strings.Split("create identity "+controllerIdName+" -o "+controllerJwtPath+" --admin -a mgmtservers", " "))
	if zitiCmdErr := zitiCmd1.Execute(); zitiCmdErr != nil {
		t.Fatalf("unable to create identity: %v", zitiCmdErr)
	}
	zitiCmd1.SetArgs([]string{"enroll", controllerJwtPath})
	if zitiCmdErr := zitiCmd1.Execute(); zitiCmdErr != nil {
		t.Fatalf("unable to create identity: %v", zitiCmdErr)
	}
	controllerIdFile = strings.TrimSuffix(controllerJwtPath, ".jwt") + ".json"
	t.Logf("controllerIdFile should exist at: %v", controllerIdFile)
	networkIdFile = controllerIdFile

	clientIdName := fmt.Sprintf("controller-client-%s", now)
	clientJwtPath := filepath.Join(o.Home, clientIdName+".jwt")
	zitiCmd1 = edge.NewCmdEdge(os.Stdout, os.Stderr, p)
	zitiCmd1.SetArgs(strings.Split("create identity "+clientIdName+" -o "+clientJwtPath+" -a mgmtclients", " "))
	if zitiCmdErr := zitiCmd1.Execute(); zitiCmdErr != nil {
		t.Fatalf("unable to create identity: %v", zitiCmdErr)
	}
	zitiCmd1.SetArgs([]string{"enroll", clientJwtPath})
	if zitiCmdErr := zitiCmd1.Execute(); zitiCmdErr != nil {
		t.Fatalf("unable to create identity: %v", zitiCmdErr)
	}
	networkClientIdFile = strings.TrimSuffix(clientJwtPath, ".jwt") + ".json"
	t.Logf("networkIdFile should exist at: %v", networkClientIdFile)

	v2 := cmd.NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	v2.SetArgs(strings.Split("ops import --username "+username+" --password "+password+" ./login_test_import.yml", " "))
	if v2Err := v2.Execute(); v2Err != nil {
		t.Fatalf("unable to import zitified-login-test.yml: %v", v2Err)
	}
	return nil
}

func reconfigureTargetForZiti(pkiRoot string) error {
	v2 := cmd.NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	v2.SetArgs(strings.Split("pki create server --key-file server --pki-root "+pkiRoot+" --ip 127.0.0.1,::1 --dns localhost,mgmt.ziti --ca-name intermediate-ca-quickstart --server-file mgmt.ziti", " "))
	if zitiCmdErr := v2.Execute(); zitiCmdErr != nil {
		return zitiCmdErr
	}
	return nil
}

func loginTestsOverZiti(t *testing.T, now, zitiPath string, externalZiti overlay) {
	t.Run("login tests over ziti", func(t *testing.T) {
		pkiRoot := gopath.Join(controllerUnderTest.Home, "pki")
		if reconfErr := reconfigureTargetForZiti(pkiRoot); reconfErr != nil {
			t.Fatalf("failed to reconfigure target: %v", reconfErr)
		}

		if ie := controllerUnderTest.initializeZitiTransportOverlay(t, now, externalZiti); ie != nil {
			t.Fatalf("failed to initialize ziti transport for controllerUnderTest: %v", ie)
		}

		controllerUnderTestCtx2, controllerUnderTestCancel := context.WithCancel(context.Background())
		defer controllerUnderTestCancel()
		controllerUnderTest.ctx = controllerUnderTestCtx2
		controllerUnderTest.ConfigFile = gopath.Join(controllerUnderTest.Home, "ctrl.yaml")
		newServerCertPath := gopath.Join(controllerUnderTest.Home, "pki/intermediate-ca-quickstart/certs/mgmt.ziti.chain.pem")
		if re := controllerUnderTest.replaceConfig(newServerCertPath); re != nil {
			t.Fatalf("failed to replace config: %v", re)
		}

		targetDone := make(chan error)
		controllerUnderTest.startExternal(zitiPath, targetDone)
		go func() {
			qsErr := <-targetDone
			if qsErr == nil {
				controllerUnderTestCancel()
				t.Fatal("unexpected error from external quickstart?")
			}
		}()
		controllerUnderTest.waitForControllerReadyorig(t, nil)
		controllerUnderTest.ControllerAddress = "mgmt.ziti"
		controllerUnderTest.ControllerPort = 443

		networkIdFile = networkClientIdFile
		runLoginTests(t)

		controllerUnderTestCancel()
	})
}

func runLoginTests(t *testing.T) {
	//Authentication Methods
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
}

func (o *overlay) stop() error {
	if o.extCmd != nil && o.extCmd.Process != nil {
		_ = o.extCmd.Process.Kill()

		// Poll until process is gone
		for i := 0; i < 120; i++ { // 60 seconds total
			time.Sleep(500 * time.Millisecond)

			// Try to find the process - if it fails, process is gone
			if proc, err := os.FindProcess(o.extCmd.Process.Pid); err != nil || proc == nil {
				o.closeFileHandles()
				return nil
			}

			// On Windows, FindProcess can succeed, send signal 0 to check
			if err := o.extCmd.Process.Signal(syscall.Signal(0)); err != nil {
				o.closeFileHandles()
				return nil // Process is gone
			}
		}

		return fmt.Errorf("overlay %s did not exit after kill", o.name)
	}
	return nil
}

func (o *overlay) closeFileHandles() {
	if f, ok := o.extCmd.Stdout.(*os.File); ok {
		if err := f.Close(); err != nil {
			fmt.Println("failed to close stdout")
		}
	}
	if f, ok := o.extCmd.Stderr.(*os.File); ok {
		if err := f.Close(); err != nil {
			fmt.Println("failed to close stderr")
		}
	}
}

func trackPid(pid int) {
	pidsMutex.Lock()
	defer pidsMutex.Unlock()
	activePids = append(activePids, pid)
}

func cleanupPids() {
	pidsMutex.Lock()
	defer pidsMutex.Unlock()

	for _, pid := range activePids {
		if !isPidRunning(pid) {
			fmt.Printf("Process (pid=%d) already exited\n", pid)
			continue
		}

		fmt.Printf("Process (pid=%d) still running, killing...\n", pid)
		proc, _ := os.FindProcess(pid)
		if err := proc.Kill(); err != nil {
			fmt.Printf("Failed to kill process (pid=%d): %v\n", pid, err)
		} else {
			fmt.Printf("Successfully killed process (pid=%d)\n", pid)
		}
	}
}

func forceKillPids(pids []int) {
	if runtime.GOOS != "windows" {
		return
	}

	for _, pid := range pids {
		fmt.Printf("Force killing process (pid=%d)...\n", pid)
		cmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
		_ = cmd.Run()
	}
}

func ensureAllPidsStopped(t *testing.T) {
	running := getRunningPids()
	if len(running) == 0 {
		fmt.Printf("All processes stopped\n")
		return
	}

	fmt.Printf("Processes still running: %v, waiting 10s...\n", running)
	time.Sleep(10 * time.Second)

	running = getRunningPids()
	if len(running) > 0 {
		fmt.Printf("Force killing remaining processes: %v\n", running)
		forceKillPids(running)

		// Poll for up to 60s for processes to exit
		start := time.Now()
		for time.Since(start) < 60*time.Second {
			running = getRunningPids()
			if len(running) == 0 {
				fmt.Printf("All processes stopped after force kill\n")
				return
			}
			fmt.Printf("Processes still running after %.0fs: %v\n", time.Since(start).Seconds(), running)
			time.Sleep(2 * time.Second)
		}

		running = getRunningPids()
		if len(running) > 0 {
			t.Fatalf("Processes still running after 60s wait: %v", running)
		}
	}

	fmt.Printf("All processes stopped\n")
}

func isPidRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if err.Error() == "os: process already finished" {
		return false
	}
	errno, ok := err.(syscall.Errno)
	if !ok {
		return false
	}
	switch errno {
	case syscall.ESRCH:
		return false
	case syscall.EPERM:
		return true
	}
	return false
}

func getRunningPidsWindows(pids []int) []int {
	if len(pids) == 0 {
		return nil
	}

	args := []string{"/NH"}
	for _, pid := range pids {
		args = append(args, "/FI", fmt.Sprintf("PID eq %d", pid))
	}

	cmd := exec.Command("tasklist", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var running []int
	outputStr := string(output)
	for _, pid := range pids {
		if strings.Contains(outputStr, fmt.Sprintf("%d", pid)) {
			running = append(running, pid)
		}
	}
	return running
}

func getRunningPids() []int {
	pidsMutex.Lock()
	defer pidsMutex.Unlock()

	if len(activePids) == 0 {
		return nil
	}

	if runtime.GOOS == "windows" {
		return getRunningPidsWindows(activePids)
	}

	var running []int
	for _, pid := range activePids {
		if isPidRunning(pid) {
			running = append(running, pid)
		}
	}
	return running
}
