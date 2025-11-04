//go:build apitests || cli_tests

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
package testutil

import (
	"context"
	"errors"
	"fmt"
	"net"
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
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/ziti/cmd"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/ops"
	"github.com/openziti/ziti/ziti/run"
	"github.com/openziti/ziti/ziti/util"
	"github.com/stretchr/testify/require"
)

var commonOpts = api.Options{
	CommonOptions: common.CommonOptions{
		Out: os.Stdout,
		Err: os.Stderr,
	},
}

type loginCreds struct {
	AdminCertFile string
	AdminCaFile   string
	AdminKeyFile  string
	AdminIdFile   string
	ApiSession    edge_apis.ApiSession
}

type Overlay struct {
	loginCreds
	NetworkBindingIdFile string // a ziti identity file to use when starting the controller which will bind a given service over an overlay
	NetworkDialingIdFile string // a ziti identity file used to dial mgmt services hosted/bound by a controller using a ziti overlay
	t                    *testing.T
	Ctx                  context.Context
	StartTimeout         time.Duration
	Name                 string
	extCmd               *exec.Cmd
	cmdDone              chan error
	pidsMutex            *sync.Mutex
	activePids           []int
	*run.QuickstartOpts
}

func (o *Overlay) ControllerHostPort() string {
	return fmt.Sprintf("https://%s:%d", o.ControllerAddress, o.ControllerPort)
}
func (o *Overlay) RouterHostPort() string {
	return fmt.Sprintf("https://%s:%d", o.RouterAddress, o.RouterPort)
}

func (o *Overlay) startArgs() []string {
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

func (o *Overlay) ReplaceConfig(newServerCertPath string) error {
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
          file: ` + o.NetworkBindingIdFile + `
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

func (o *Overlay) StartExternal(zitiPath string, done chan error) {
	args := append([]string{"edge", "quickstart"}, o.startArgs()...)
	fmt.Printf("%s overlay command: %s %s\n", o.Name, zitiPath, strings.Join(args, " "))
	o.extCmd = exec.CommandContext(
		o.Ctx,
		zitiPath,
		args...,
	)
	_ = os.Mkdir(o.Home, 0755)
	stdoutFile, createErr1 := os.Create(filepath.Join(o.Home, "ctrl-stdout.log"))
	if createErr1 != nil {
		done <- createErr1
	}
	stderrFile, createErr2 := os.Create(filepath.Join(o.Home, "ctrl-stderr.log"))
	if createErr2 != nil {
		done <- createErr2
	}
	o.extCmd.Stdout = stdoutFile
	o.extCmd.Stderr = stderrFile

	fmt.Printf("ctrl logs at: %s\n", filepath.Join(o.Home, "ctrl-stdout.log"))
	if startErr := o.extCmd.Start(); startErr != nil {
		done <- startErr
		return
	}

	fmt.Printf("started ziti quickstart (pid=%d)\n", o.extCmd.Process.Pid)
	o.trackPid(o.extCmd.Process.Pid)

	go func() {
		err := o.extCmd.Wait()
		if errors.Is(err, context.Canceled) || (err != nil && strings.Contains(err.Error(), "signal killed")) {
			err = nil
		}
		done <- err
	}()
}

func (o *Overlay) CreateAdminIdentity(t *testing.T, now, baseDir string) error {
	if lr, le := o.Login(); le != nil {
		return le
	} else {
		//set the valid token for reuse later:
		require.NotEmpty(t, lr)
		require.NotEmpty(t, lr.ApiSession)
		require.NotEmpty(t, lr.ApiSession.GetToken())
		o.ApiSession = lr.ApiSession
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
	o.AdminIdFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".json"
	t.Logf("identity file should exist at: %v", o.AdminIdFile)

	unwrapCmd := ops.NewUnwrapIdentityFileCommand(os.Stdout, os.Stderr)
	unwrapCmd.SetArgs([]string{o.AdminIdFile})
	unwrapCmdErr := unwrapCmd.Execute()
	if unwrapCmdErr != nil {
		t.Fatalf("unable to unwrap identity: %v", unwrapCmdErr)
	}
	o.AdminCertFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".cert"
	o.AdminCaFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".ca"
	o.AdminKeyFile = strings.TrimSuffix(adminJwtPath, ".jwt") + ".key"
	t.Logf("certfile should exist at: %v", o.AdminCertFile)
	t.Logf("caFile should exist at: %v", o.AdminCaFile)
	t.Logf("keyFile should exist at: %v", o.AdminKeyFile)
	return nil
}

func (o *Overlay) CreateOverlayIdentities(t *testing.T, now string) error {
	p := common.NewOptionsProvider(os.Stdout, os.Stderr)
	if _, le := o.Login(); le != nil {
		return le
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
	o.NetworkBindingIdFile = strings.TrimSuffix(controllerJwtPath, ".jwt") + ".json"
	t.Logf("networkBindingIdFile should exist at: %v", o.NetworkBindingIdFile)

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
	o.NetworkDialingIdFile = strings.TrimSuffix(clientJwtPath, ".jwt") + ".json"
	t.Logf("networkDialingIdFile should exist at: %v", o.NetworkDialingIdFile)

	cwd, _ := os.Getwd()
	yamlToImport, _ := filepath.Abs(cwd + "/login_test_import.yml")
	v2 := cmd.NewRootCommand(os.Stdin, os.Stdout, os.Stderr)
	v2.SetArgs(strings.Split("ops import --username "+o.Username+" --password "+o.Password+" "+yamlToImport, " "))
	if v2Err := v2.Execute(); v2Err != nil {
		t.Fatalf("unable to import zitified-login-test.yml: %v", v2Err)
	}
	return nil
}
func (o *Overlay) Stop() error {
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

		return fmt.Errorf("overlay %s did not exit after kill", o.Name)
	}
	return nil
}
func (o *Overlay) Login() (*edge.LoginOptions, error) {
	initialLogin := &edge.LoginOptions{
		Options:       commonOpts,
		Username:      o.Username,
		Password:      o.Password,
		ControllerUrl: o.ControllerHostPort(),
		Yes:           true,
		NetworkId:     o.NetworkDialingIdFile,
	}
	ile := initialLogin.Run()
	if ile == nil {
		util.ReloadConfig() //every login really needs to call reload to flush/overwrite the cached client
	}
	return initialLogin, ile
}

func (o *Overlay) closeFileHandles() {
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

func (o *Overlay) NewTestLoginOpts() edge.LoginOptions {
	return edge.LoginOptions{
		Options:       commonOpts,
		Username:      o.Username,
		Password:      o.Password,
		ControllerUrl: o.ControllerHostPort(),
		Yes:           true,
		IgnoreConfig:  false,
		NetworkId:     o.NetworkDialingIdFile,
	}
}

func (o *Overlay) trackPid(pid int) {
	o.pidsMutex.Lock()
	defer o.pidsMutex.Unlock()
	o.activePids = append(o.activePids, pid)
}

func (o *Overlay) CleanupPids() {
	o.pidsMutex.Lock()
	defer o.pidsMutex.Unlock()

	for _, pid := range o.activePids {
		if !o.isPidRunning(pid) {
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

func (o *Overlay) forceKillPids(pids []int) {
	if runtime.GOOS != "windows" {
		return
	}

	for _, pid := range pids {
		fmt.Printf("Force killing process (pid=%d)...\n", pid)
		c := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
		_ = c.Run()
	}
}

func (o *Overlay) EnsureAllPidsStopped(t *testing.T) {
	running := o.getRunningPids()
	if len(running) == 0 {
		fmt.Printf("All processes stopped\n")
		return
	}

	fmt.Printf("Processes still running: %v, waiting 10o...\n", running)
	time.Sleep(10 * time.Second)

	running = o.getRunningPids()
	if len(running) > 0 {
		fmt.Printf("Force killing remaining processes: %v\n", running)
		o.forceKillPids(running)

		// Poll for up to 60s for processes to exit
		start := time.Now()
		for time.Since(start) < 60*time.Second {
			running = o.getRunningPids()
			if len(running) == 0 {
				fmt.Printf("All processes stopped after force kill\n")
				return
			}
			fmt.Printf("Processes still running after %.0fs: %v\n", time.Since(start).Seconds(), running)
			time.Sleep(2 * time.Second)
		}

		running = o.getRunningPids()
		if len(running) > 0 {
			t.Fatalf("Processes still running after 60s wait: %v", running)
		}
	}

	fmt.Printf("All processes stopped\n")
}

func (o *Overlay) isPidRunning(pid int) bool {
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
	default:
		// ignored
	}
	return false
}

func (o *Overlay) getRunningPidsWindows(pids []int) []int {
	if len(pids) == 0 {
		return nil
	}

	args := []string{"/NH"}
	for _, pid := range pids {
		args = append(args, "/FI", fmt.Sprintf("PID eq %d", pid))
	}

	c := exec.Command("tasklist", args...)
	output, err := c.Output()
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

func (o *Overlay) getRunningPids() []int {
	o.pidsMutex.Lock()
	defer o.pidsMutex.Unlock()

	if len(o.activePids) == 0 {
		return nil
	}

	if runtime.GOOS == "windows" {
		return o.getRunningPidsWindows(o.activePids)
	}

	var running []int
	for _, pid := range o.activePids {
		if o.isPidRunning(pid) {
			running = append(running, pid)
		}
	}
	return running
}

func (o *Overlay) WaitForControllerReady(timeout time.Duration) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Println("Waiting for controller to start... " + o.ControllerHostPort())
			_, err := rest_util.GetControllerWellKnownCas(o.ControllerHostPort())
			if err == nil {
				return nil
			}
		case <-time.After(timeout):
			return fmt.Errorf("timeout waiting for controller to become ready at %s", o.ControllerHostPort())
		}
	}
}

func (o *Overlay) WaitForRouterReady(timeout time.Duration) error {
	if !o.Routerless {
		routerReady := make(chan error)
		o.WaitForRouter(timeout, routerReady)

		select {
		case err := <-routerReady:
			return err
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout waiting for router to be ready at: %s", o.RouterHostPort())
		}
	}
	return nil
}

func CreateOverlay(t *testing.T, ctx context.Context, startTimeout time.Duration, home string, name string, ha bool) Overlay {
	o := Overlay{
		Name:         name,
		t:            t,
		Ctx:          ctx,
		StartTimeout: startTimeout,
		cmdDone:      make(chan error, 1),
		QuickstartOpts: &run.QuickstartOpts{
			Home:              gopath.Join(home, name),
			ControllerAddress: "localhost", //helpers.GetCtrlAdvertisedAddress(),
			ControllerPort:    findAvailablePort(t),
			RouterAddress:     "localhost", //helpers.GetRouterAdvertisedAddress(),
			RouterPort:        findAvailablePort(t),
			Routerless:        false,
			TrustDomain:       name,
			InstanceID:        name,
			IsHA:              ha,
			Username:          "admin",
			Password:          "admin",
			ConfigureAndExit:  false,
		},
		pidsMutex: &sync.Mutex{},
	}

	return o
}

func findAvailablePort(t *testing.T) uint16 {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return (uint16)(listener.Addr().(*net.TCPAddr).Port)
}
