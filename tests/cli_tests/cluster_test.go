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
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/openziti/edge-api/rest_util"
	"github.com/stretchr/testify/require"
)

// Test_Quickstart_Cluster brings up a 3-node cluster via `ziti run quickstart cluster` then:
//   - asserts every node's controller comes up and accepts an admin login. Only
//     node 1 initializes the admin, so a successful login on nodes 2 and 3 proves
//     raft replicated the admin identity and the cluster formed.
//   - delivers a graceful stop to the parent (SIGINT on POSIX, a CTRL_BREAK
//     console event on Windows) and asserts the auto-created temp home is removed.
//     If the stop signal cannot be delivered (e.g. no console attached), it
//     force-kills and skips the shutdown assertions.
func Test_Quickstart_Cluster(t *testing.T) {
	zitiPath := os.Getenv("ZITI_CLI_TEST_ZITI_BIN")
	if zitiPath == "" {
		t.Skip("ZITI_CLI_TEST_ZITI_BIN not set")
	}
	if _, statErr := os.Stat(zitiPath); statErr != nil {
		t.Fatalf("ziti binary not found at %s: %v", zitiPath, statErr)
	}

	const size = 3
	// One contiguous block split in half so the ctrl range (base..base+size-1)
	// and router range (base+size..base+2*size-1) never overlap.
	base := findConsecutivePorts(t, size*2)
	ctrlBase := base
	routerBase := base + size
	cfgDir := filepath.Join(t.TempDir(), "cli-config")
	logPath := filepath.Join(t.TempDir(), "cluster.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	defer func() { _ = logFile.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No --home on purpose: the cluster creates a temp dir and removes it on a
	// clean shutdown. Its path is read from the process output.
	args := []string{
		"run", "quickstart", "cluster",
		"--size", strconv.Itoa(size),
		"--ctrl-address", "localhost",
		"--router-address", "localhost",
		fmt.Sprintf("--ctrl-port=%d", ctrlBase),
		fmt.Sprintf("--router-port=%d", routerBase),
	}
	t.Logf("starting: %s %v", zitiPath, args)
	clusterCmd := exec.CommandContext(ctx, zitiPath, args...)
	clusterCmd.Env = append(os.Environ(), "PFXLOG_NO_JSON=true", "ZITI_CONFIG_DIR="+cfgDir)
	clusterCmd.SysProcAttr = newProcessGroupAttr()
	clusterCmd.Stdout = logFile
	clusterCmd.Stderr = logFile
	require.NoError(t, clusterCmd.Start())

	defer func() {
		if clusterCmd.Process == nil {
			return
		}
		// Backstop in case the test returns before the clean shutdown below.
		if gracefulStop(clusterCmd) != nil {
			forceKillTree(clusterCmd)
		}
		waited := make(chan struct{})
		go func() {
			_, _ = clusterCmd.Process.Wait()
			close(waited)
		}()
		select {
		case <-waited:
		case <-time.After(30 * time.Second):
		}
	}()

	// Discover the auto-created temp home from the process output.
	reHome := regexp.MustCompile(`temporary --home '([^']+)'`)
	var tempHome string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) && tempHome == "" {
		data, _ := os.ReadFile(logPath)
		if m := reHome.FindStringSubmatch(string(data)); m != nil {
			tempHome = m[1]
		} else {
			time.Sleep(250 * time.Millisecond)
		}
	}
	require.NotEmpty(t, tempHome, "did not observe an auto-created temp home in cluster output (see %s)", logPath)
	t.Logf("cluster temp home: %s", tempHome)

	// Every node must come up and accept an admin login. Nodes 2 and 3 never run
	// init, so a successful admin login there proves cluster replication.
	for i := 0; i < size; i++ {
		ctrlUrl := fmt.Sprintf("https://localhost:%d", int(ctrlBase)+i)
		require.NoErrorf(t, waitClusterNodeReady(ctrlUrl, "admin", "admin", 180*time.Second),
			"node %d (%s) never became ready; see %s", i+1, ctrlUrl, logPath)
		t.Logf("node %d ready and admin login succeeded at %s", i+1, ctrlUrl)
	}

	// Drive the cluster through the same ziti CLI that launched it: log into each
	// node, dump and verify raft membership, then prove a model write replicates.
	cliCfg := filepath.Join(t.TempDir(), "test-cli")
	runZiti := func(arg ...string) (stdout, stderr string, err error) {
		c := exec.Command(zitiPath, arg...)
		c.Env = append(os.Environ(), "ZITI_CONFIG_DIR="+cliCfg)
		var so, se bytes.Buffer
		c.Stdout, c.Stderr = &so, &se
		err = c.Run()
		return so.String(), se.String(), err
	}
	login := func(node int) error {
		url := fmt.Sprintf("https://localhost:%d", int(ctrlBase)+node)
		if _, se, err := runZiti("edge", "login", url, "-u", "admin", "-p", "admin", "-y"); err != nil {
			return fmt.Errorf("%w: %s", err, se)
		}
		return nil
	}

	// log in to every node through the CLI
	for i := 0; i < size; i++ {
		require.NoErrorf(t, login(i), "ziti edge login to node %d", i+1)
	}

	// dump the raft membership for a human to eyeball
	membersOut, membersErr, listErr := runZiti("ops", "cluster", "list")
	require.NoErrorf(t, listErr, "ziti ops cluster list: %s", membersErr)
	t.Logf("ziti ops cluster list:\n%s", membersOut)

	// verify it: size members, exactly one leader, all connected voters
	require.Eventuallyf(t, func() bool {
		so, _, e := runZiti("ops", "cluster", "list", "-j")
		if e != nil {
			return false
		}
		var resp struct {
			Data []struct {
				Connected bool `json:"connected"`
				Leader    bool `json:"leader"`
				Voter     bool `json:"voter"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(so), &resp) != nil || len(resp.Data) != size {
			return false
		}
		leaders, connectedVoters := 0, 0
		for _, m := range resp.Data {
			if m.Leader {
				leaders++
			}
			if m.Connected && m.Voter {
				connectedVoters++
			}
		}
		return leaders == 1 && connectedVoters == size
	}, 30*time.Second, time.Second, "cluster should converge to %d connected voters with one leader", size)
	t.Logf("raft membership verified: %d connected voting members, one leader", size)

	// prove replication: create an identity on node 1, see it appear on node 2
	idName := fmt.Sprintf("repl-check-%d", time.Now().UnixNano())
	require.NoError(t, login(0))
	if _, createErr, err := runZiti("edge", "create", "identity", idName); err != nil {
		require.NoErrorf(t, err, "create identity on node 1: %s", createErr)
	}
	require.NoError(t, login(1))
	require.Eventuallyf(t, func() bool {
		so, _, e := runZiti("edge", "list", "identities", fmt.Sprintf("name=%q", idName), "-j")
		if e != nil {
			return false
		}
		var resp struct {
			Data []struct {
				Name string `json:"name"`
			} `json:"data"`
		}
		return json.Unmarshal([]byte(so), &resp) == nil && len(resp.Data) == 1 && resp.Data[0].Name == idName
	}, 30*time.Second, time.Second, "identity %q created on node 1 should replicate to node 2", idName)
	t.Logf("identity %q created on node 1 is visible on node 2 (raft replication confirmed)", idName)

	// Clean shutdown: deliver a graceful stop to the parent. It relays to the
	// children, then removes the temp home.
	if stopErr := gracefulStop(clusterCmd); stopErr != nil {
		forceKillTree(clusterCmd)
		_, _ = clusterCmd.Process.Wait()
		t.Skipf("graceful stop signal not deliverable in this environment (%v); verified 3-node bring-up, skipping shutdown assertions", stopErr)
	}
	exited := make(chan error, 1)
	go func() { exited <- clusterCmd.Wait() }()
	select {
	case <-exited:
	case <-time.After(90 * time.Second):
		t.Fatalf("cluster did not exit within 90s of the stop signal; see %s", logPath)
	}

	_, statErr := os.Stat(tempHome)
	require.Truef(t, os.IsNotExist(statErr),
		"temp home %s should have been removed after a clean shutdown (stat err: %v)", tempHome, statErr)
}

// waitClusterNodeReady polls until the controller at ctrlUrl serves its CA bundle
// and accepts an admin UPDB login, or the timeout elapses.
func waitClusterNodeReady(ctrlUrl, user, pass string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		caCerts, err := rest_util.GetControllerWellKnownCas(ctrlUrl)
		if err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		pool := x509.NewCertPool()
		for _, ca := range caCerts {
			pool.AddCert(ca)
		}
		if _, err := rest_util.NewEdgeManagementClientWithUpdb(user, pass, ctrlUrl, pool); err != nil {
			lastErr = err
			time.Sleep(time.Second)
			continue
		}
		return nil
	}
	return fmt.Errorf("not ready within %s: %w", timeout, lastErr)
}

// findConsecutivePorts returns a base port p such that p..p+n-1 are all bindable.
// Note: there is an inherent TOCTOU race between closing these listeners here and
// the child processes binding them. This is an accepted limitation of testing
// with external processes. Under heavy concurrent load a bind can still lose.
func findConsecutivePorts(t *testing.T, n int) uint16 {
	t.Helper()
	for attempt := 0; attempt < 100; attempt++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			continue
		}
		base := l.Addr().(*net.TCPAddr).Port
		_ = l.Close()
		if base == 0 || base+n-1 > 65535 {
			continue
		}
		held := make([]net.Listener, 0, n)
		ok := true
		for i := 0; i < n; i++ {
			li, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", base+i))
			if e != nil {
				ok = false
				break
			}
			held = append(held, li)
		}
		for _, li := range held {
			_ = li.Close()
		}
		if ok {
			return uint16(base)
		}
	}
	t.Fatalf("could not find %d consecutive free ports", n)
	return 0
}
