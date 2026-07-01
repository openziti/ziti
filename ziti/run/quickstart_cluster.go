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

package run

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/ziti/cmd/console"
	"github.com/openziti/ziti/v2/ziti/cmd/helpers"
	"github.com/openziti/ziti/v2/ziti/constants"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// QuickstartClusterOpts drives `quickstart cluster`: it launches one
// `ziti run quickstart` child process per node and joins them into one HA cluster.
// The parent owns the children's lifecycle. Ctrl-C stops every node, and a
// parent-created temp home is removed on a clean exit.
type QuickstartClusterOpts struct {
	Home              string
	Username          string
	Password          string
	ControllerAddress string
	RouterAddress     string
	CtrlPort          uint16
	RouterPort        uint16
	TrustDomain       string
	Size              int
	ShutdownGrace     time.Duration
	Zac               bool
	ZacVersion        string
	ZacLocation       string
	Yes               bool

	out         io.Writer
	errOut      io.Writer
	verbose     bool
	cleanOnExit bool
}

func NewQuickStartClusterCmd(out io.Writer, errOut io.Writer, ctx context.Context) *cobra.Command {
	options := &QuickstartClusterOpts{}
	defaultCtrlPort, _ := strconv.ParseInt(constants.DefaultCtrlEdgeAdvertisedPort, 10, 16)
	defaultRouterPort, _ := strconv.ParseInt(constants.DefaultZitiEdgeRouterPort, 10, 16)

	cmd := &cobra.Command{
		Use:   "cluster",
		Short: "runs a multi-node OpenZiti cluster, each node a quickstart, in child processes",
		Long: "runs a multi-node OpenZiti cluster by launching one quickstart child process per node and joining " +
			"them into a single raft cluster, suitable for testing and development. Pressing Ctrl-C stops every " +
			"node. If --home is omitted a temporary directory is created and removed on exit.",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.out = out
			options.errOut = errOut
			return options.run(ctx)
		},
	}

	cmd.Flags().StringVar(&options.Home, "home", "", "permanent directory to use, or a temp dir (removed on exit) if omitted")
	cmd.Flags().StringVarP(&options.Username, "username", "u", "", "admin username, default: admin")
	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "admin password, default: admin")
	cmd.Flags().StringVar(&options.ControllerAddress, "ctrl-address", "", "advertised controller address used by every node. current: "+helpers.GetCtrlEdgeAdvertisedAddress())
	cmd.Flags().StringVar(&options.RouterAddress, "router-address", "", "advertised router address used by every node")
	cmd.Flags().Uint16Var(&options.CtrlPort, "ctrl-port", uint16(defaultCtrlPort), "base controller port (node index N listens on base+N)")
	cmd.Flags().Uint16Var(&options.RouterPort, "router-port", uint16(defaultRouterPort), "base router port (node index N listens on base+N)")
	cmd.Flags().StringVar(&options.TrustDomain, "trust-domain", "quickstart", "trust domain used in SPIFFE ids")
	cmd.Flags().IntVar(&options.Size, "size", 3, "number of nodes in the cluster (3-9)")
	cmd.Flags().DurationVar(&options.ShutdownGrace, "shutdown-grace", 30*time.Second, "max time to wait for nodes to shut down cleanly before the parent gives up waiting")
	cmd.Flags().BoolVar(&options.Zac, "zac", false, "download the Ziti Admin Console (ZAC) and configure every node to serve it")
	cmd.Flags().StringVar(&options.ZacVersion, "zac-version", "latest", "ZAC version to download when --zac is set")
	cmd.Flags().StringVar(&options.ZacLocation, "zac-location", "", "directory to install ZAC into; defaults to <home>/console (shared by all nodes)")
	cmd.Flags().BoolVarP(&options.Yes, "yes", "y", false, "answer yes to prompts (e.g. replacing installed ZAC assets with a different --zac-version)")
	cmd.Flags().BoolVar(&options.verbose, "verbose", false, "show additional output")

	return cmd
}

type nodeExit struct {
	idx int
	err error
}

func (o *QuickstartClusterOpts) run(ctx context.Context) error {
	if o.verbose {
		pfxlog.GlobalInit(logrus.DebugLevel, pfxlog.DefaultOptions().Color())
	}
	if o.Size < 3 || o.Size > 9 {
		return fmt.Errorf("when using --size the value must be between 3 and 9, more cluster members cause slower mutations")
	}
	// Node i uses CtrlPort+i and RouterPort+i. Validate the derived ranges up front
	// so an out-of-range or overlapping port is a clear error rather than a
	// confusing bind failure partway through bring-up.
	ctrlMax := int(o.CtrlPort) + o.Size - 1
	routerMax := int(o.RouterPort) + o.Size - 1
	if ctrlMax > 65535 || routerMax > 65535 {
		return fmt.Errorf("--ctrl-port/--router-port plus %d nodes exceeds the maximum port 65535", o.Size)
	}
	if int(o.CtrlPort) <= routerMax && int(o.RouterPort) <= ctrlMax {
		return fmt.Errorf("the --ctrl-port range %d-%d and --router-port range %d-%d overlap for %d nodes. Pick port bases at least %d apart", o.CtrlPort, ctrlMax, o.RouterPort, routerMax, o.Size, o.Size)
	}
	if o.Username == "" {
		o.Username = "admin"
	}
	if o.Password == "" {
		o.Password = "admin"
	}
	if strings.TrimSpace(o.TrustDomain) == "" {
		o.TrustDomain = "quickstart"
	}

	if err := o.resolveHome(); err != nil {
		return err
	}

	// Install ZAC once in the parent. Nodes reuse the shared location.
	if o.Zac {
		if o.ZacLocation == "" {
			o.ZacLocation = filepath.Join(o.Home, "console")
		}
		// Normalize separators to match the location the config generator writes when creating the config.
		o.ZacLocation = helpers.NormalizePath(o.ZacLocation)
		version, zacErr := console.EnsureAssets(o.out, o.ZacVersion, o.ZacLocation, o.Yes)
		if zacErr != nil {
			return fmt.Errorf("failed to install ZAC: %w", zacErr)
		}
		if version != "" {
			logrus.Infof("ZAC %s ready at '%s' (shared by all nodes)", version, o.ZacLocation)
			// Pin children to the concrete version the parent installed, not the "latest" request.
			o.ZacVersion = version
		}
	}

	ctrlAddr := o.ControllerAddress
	if ctrlAddr == "" {
		ctrlAddr = helpers.GetCtrlEdgeAdvertisedAddress()
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine path to this executable: %w", err)
	}

	// Children run in their own process group (see configureChildProcAttr). The
	// parent catches the shutdown signal and relays a clean stop to each child
	// (CTRL_BREAK on Windows, SIGINT on POSIX). Nothing force-kills a child. If a
	// node will not stop, the temp home is left in place rather than removed under
	// a live process.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	var children []*exec.Cmd
	var logFiles []*os.File
	var monitors sync.WaitGroup
	exitCh := make(chan nodeExit, o.Size)

	cleanup := func() {
		for _, c := range children {
			relayStop(c)
		}
		done := make(chan struct{})
		go func() {
			monitors.Wait()
			close(done)
		}()
		cleanExit := true
		select {
		case <-done:
		case <-time.After(o.ShutdownGrace):
			cleanExit = false
			logrus.Warnf("not all nodes stopped within %s, leaving the environment in place rather than deleting it under a running node", o.ShutdownGrace)
		}
		for _, f := range logFiles {
			_ = f.Close()
		}
		switch {
		case !o.cleanOnExit:
			fmt.Println("environment left intact at: " + o.Home)
		case !cleanExit:
			fmt.Println("temp directory NOT removed because nodes are still running: " + o.Home)
		default:
			fmt.Println("removing temp directory at: " + o.Home)
			_ = os.RemoveAll(o.Home)
		}
	}

	readyChans := make([]chan struct{}, o.Size)

	startNode := func(idx int) error {
		cmd := exec.Command(self, o.childArgs(idx, ctrlAddr)...)
		// Give each node its own ziti CLI config/session dir. The CLI otherwise
		// shares one per-user location and the nodes' concurrent logins collide.
		instDir := filepath.Join(o.Home, fmt.Sprintf("instance-%d", idx+1))
		cliConfigDir := filepath.Join(instDir, "ziti-cli")
		cmd.Env = append(os.Environ(), "ZITI_CONFIG_DIR="+cliConfigDir)
		prefix := fmt.Sprintf("[instance-%d] ", idx+1)
		ready := make(chan struct{})
		readyChans[idx] = ready

		// Mirror each node's output to a per-node log file as well as the merged,
		// prefixed console stream. The console writer also watches for the line a
		// node prints once it is fully up (leader elected/joined, router running)
		// and closes ready when it sees it. The file gets the raw, unprefixed output.
		stdout := io.Writer(newReadyWriter(o.out, prefix, "Quickly add another member", func() { close(ready) }))
		stderr := io.Writer(newPrefixWriter(o.errOut, prefix))
		// The instance dir holds the node's db, pki, and CLI config dir, so it
		// must exist before the node starts.
		if mkErr := os.MkdirAll(instDir, 0o755); mkErr != nil {
			return fmt.Errorf("could not create instance directory %s for node %d: %w", instDir, idx+1, mkErr)
		}
		if logFile, ferr := os.OpenFile(o.nodeLogPath(idx), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); ferr == nil {
			logFiles = append(logFiles, logFile)
			stdout = io.MultiWriter(stdout, logFile)
			stderr = io.MultiWriter(stderr, logFile)
		} else {
			logrus.Warnf("could not open log file for node %d: %v", idx+1, ferr)
		}
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		configureChildProcAttr(cmd)
		if startErr := cmd.Start(); startErr != nil {
			return fmt.Errorf("failed to start node %d: %w", idx+1, startErr)
		}
		children = append(children, cmd)
		monitors.Add(1)
		go func() {
			defer monitors.Done()
			exitCh <- nodeExit{idx: idx, err: cmd.Wait()}
		}()
		return nil
	}

	if o.isFullRestart() {
		// Full restart: every node is already a raft member, so they start
		// together to re-form a quorum (a lone node can't elect a leader).
		logrus.Infof("existing cluster home detected at %s, starting all %d nodes together to re-form quorum", o.Home, o.Size)
		for i := 0; i < o.Size; i++ {
			fmt.Printf("starting cluster node %d of %d...\n", i+1, o.Size)
			if startErr := startNode(i); startErr != nil {
				cleanup()
				return startErr
			}
		}
		for i := 0; i < o.Size; i++ {
			if waitErr := o.waitForNode(sigCtx, i, readyChans[i], exitCh); waitErr != nil {
				cleanup()
				return waitErr
			}
			if sigCtx.Err() != nil {
				cleanup()
				return nil
			}
		}
	} else {
		// First bring-up, or growing an existing single node into a cluster: node 1
		// must be leader before the rest join, or they hit CLUSTER_NO_LEADER. Node 1
		// initializes if fresh, or restarts as leader if it exists, then the rest join.
		fmt.Printf("starting cluster node 1 of %d...\n", o.Size)
		if startErr := startNode(0); startErr != nil {
			cleanup()
			return startErr
		}
		if waitErr := o.waitForNode(sigCtx, 0, readyChans[0], exitCh); waitErr != nil {
			cleanup()
			return waitErr
		}
		if sigCtx.Err() != nil {
			cleanup()
			return nil
		}

		// Start the remaining nodes, each joining node 1, serialized on readiness.
		for i := 1; i < o.Size; i++ {
			fmt.Printf("starting cluster node %d of %d...\n", i+1, o.Size)
			if startErr := startNode(i); startErr != nil {
				cleanup()
				return startErr
			}
			if waitErr := o.waitForNode(sigCtx, i, readyChans[i], exitCh); waitErr != nil {
				cleanup()
				return waitErr
			}
			if sigCtx.Err() != nil {
				cleanup()
				return nil
			}
		}
	}

	// Let the nodes flush their startup output for a few seconds so the banner
	// is not scrolled away by trailing log lines.
	select {
	case <-sigCtx.Done():
		cleanup()
		return nil
	case <-time.After(3 * time.Second):
	}

	o.printDetails(ctrlAddr, children)

	select {
	case <-sigCtx.Done():
		fmt.Println("\nshutdown signal received, stopping cluster nodes...")
	case ne := <-exitCh:
		if ne.err != nil {
			fmt.Printf("\ncluster node %d exited unexpectedly (%v), stopping remaining nodes...\n", ne.idx+1, ne.err)
		} else {
			fmt.Printf("\ncluster node %d exited, stopping remaining nodes...\n", ne.idx+1)
		}
	}

	cleanup()
	return nil
}

// childArgs builds the argv for node idx, re-invoking this binary as a quickstart
// node. Node 0 initializes the cluster, later nodes join node 0.
func (o *QuickstartClusterOpts) childArgs(idx int, ctrlAddr string) []string {
	ctrlPort := o.CtrlPort + uint16(idx)
	routerPort := o.RouterPort + uint16(idx)
	args := []string{"run", "quickstart"}
	if idx > 0 {
		args = append(args, "join")
	}
	args = append(args,
		"--home", o.Home,
		"--instance-id", fmt.Sprintf("instance-%d", idx+1),
		"--ctrl-port", strconv.Itoa(int(ctrlPort)),
		"--router-port", strconv.Itoa(int(routerPort)),
		"--trust-domain", o.TrustDomain,
		"--username", o.Username,
		"--password", o.Password,
	)
	if o.ControllerAddress != "" {
		args = append(args, "--ctrl-address", o.ControllerAddress)
	}
	if o.RouterAddress != "" {
		args = append(args, "--router-address", o.RouterAddress)
	}
	if o.verbose {
		args = append(args, "--verbose")
	}
	if o.Zac {
		// Each node serves the parent-installed assets from the shared location.
		args = append(args, "--zac", "--zac-location", o.ZacLocation)
		if o.ZacVersion != "" {
			args = append(args, "--zac-version", o.ZacVersion)
		}
		if o.Yes {
			args = append(args, "--yes")
		}
	}
	if idx > 0 {
		args = append(args, "--cluster-member", fmt.Sprintf("tls:%s:%d", ctrlAddr, o.CtrlPort))
	}
	return args
}

// waitForNode blocks until node idx signals it is fully up (its ready channel is
// closed), the shutdown signal fires, the node exits early, or the timeout
// elapses.
func (o *QuickstartClusterOpts) waitForNode(ctx context.Context, idx int, ready <-chan struct{}, exitCh chan nodeExit) error {
	timeout := 180 * time.Second
	select {
	case <-ready:
		logrus.Infof("cluster node %d is up", idx+1)
		return nil
	case ne := <-exitCh:
		// A node died during bring-up. Abort and let cleanup() stop the rest.
		return fmt.Errorf("node %d exited before becoming ready: %v", ne.idx+1, ne.err)
	case <-ctx.Done():
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s waiting for node %d to become ready", timeout, idx+1)
	}
}

func (o *QuickstartClusterOpts) resolveHome() error {
	if o.Home == "" {
		tmpDir, err := os.MkdirTemp("", "quickstart-cluster")
		if err != nil {
			return fmt.Errorf("could not create temp directory: %w", err)
		}
		o.Home = tmpDir
		o.cleanOnExit = true
		logrus.Infof("temporary --home '%s' will be removed on exit", o.Home)
		return nil
	}
	// Expand a leading ~ only (bare, or before a path separator). A ~ elsewhere
	// is a literal character.
	if o.Home == "~" || strings.HasPrefix(o.Home, "~/") || strings.HasPrefix(o.Home, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not find user's home directory: %w", err)
		}
		o.Home = filepath.Join(home, strings.TrimLeft(o.Home[1:], `/\`))
	}
	logrus.Infof("permanent --home '%s' will not be removed on exit", o.Home)
	return nil
}

func (o *QuickstartClusterOpts) printDetails(ctrlAddr string, children []*exec.Cmd) {
	fmt.Println("=======================================================================================")
	fmt.Printf("cluster of %d nodes started.\n", o.Size)
	for i := 0; i < o.Size; i++ {
		pid := 0
		if i < len(children) && children[i].Process != nil {
			pid = children[i].Process.Pid
		}
		fmt.Printf("    node %d  controller: %s:%d  router: %s:%d  pid: %d\n",
			i+1, ctrlAddr, o.CtrlPort+uint16(i), o.routerAddrOrDefault(), o.RouterPort+uint16(i), pid)
	}
	if o.Zac {
		fmt.Printf("    console (ZAC)          : each node serves it on its controller port, e.g. https://%s:%d/zac\n", ctrlAddr, o.CtrlPort)
	}
	fmt.Println("    home directory         : " + o.Home)
	fmt.Println("    configured trust domain: " + o.TrustDomain)
	fmt.Println("    per-node logs:")
	for i := 0; i < o.Size; i++ {
		fmt.Printf("        node %d: %s\n", i+1, o.nodeLogPath(i))
	}

	exe := "ziti"
	if self, err := os.Executable(); err == nil {
		exe = self
	}
	// PowerShell continues lines with a backtick, POSIX shells with a backslash.
	cont := "\\"
	if os.Getenv("PSModulePath") != "" {
		cont = "`"
	}
	home := o.Home
	if strings.ContainsAny(home, " \t") {
		home = `"` + home + `"`
	}
	fmt.Println()
	fmt.Println("    to run a node individually (e.g. in separate terminals, or to restart one):")
	for i := 0; i < o.Size; i++ {
		fmt.Printf("        node %d:\n", i+1)
		fmt.Printf("            %s run quickstart %s\n", exe, cont)
		fmt.Printf("                --home %s %s\n", home, cont)
		fmt.Printf("                --instance-id instance-%d %s\n", i+1, cont)
		fmt.Printf("                --ctrl-port %d %s\n", o.CtrlPort+uint16(i), cont)
		fmt.Printf("                --router-port %d\n", o.RouterPort+uint16(i))
	}
	fmt.Println()
	fmt.Println("    press Ctrl-C here to stop all nodes.")
	fmt.Println("=======================================================================================")
}

// nodeLogPath is the per-node log file each node's output is mirrored to.
func (o *QuickstartClusterOpts) nodeLogPath(idx int) string {
	return filepath.Join(o.Home, fmt.Sprintf("instance-%d", idx+1), "quickstart.log")
}

// isFullRestart reports whether every node's data dir already exists in --home.
// If so, all nodes are existing raft members and start together to re-form
// quorum. If only some exist (growing a single node into a cluster, or a node
// that lost its data), node 1 comes up as leader first and the rest join.
func (o *QuickstartClusterOpts) isFullRestart() bool {
	if o.Home == "" {
		return false
	}
	for i := 0; i < o.Size; i++ {
		// mirrors the child's own "already initialized" check (<home>/<instance>/db)
		if _, err := os.Stat(filepath.Join(o.Home, fmt.Sprintf("instance-%d", i+1), "db")); err != nil {
			return false
		}
	}
	return true
}

func (o *QuickstartClusterOpts) routerAddrOrDefault() string {
	if o.RouterAddress != "" {
		return o.RouterAddress
	}
	return helpers.GetRouterAdvertisedAddress()
}

var prefixWriterMu sync.Mutex

// prefixWriter prefixes each complete line written to it, so interleaved output
// from multiple child nodes stays attributable. A package-level mutex keeps lines
// from different writers from interleaving mid-line. If sentinel is non-empty, the
// first line containing it fires onSentinel exactly once.
type prefixWriter struct {
	w          io.Writer
	prefix     string
	buf        bytes.Buffer
	sentinel   string
	onSentinel func()
	once       sync.Once
}

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix}
}

func newReadyWriter(w io.Writer, prefix, sentinel string, onSentinel func()) *prefixWriter {
	return &prefixWriter{w: w, prefix: prefix, sentinel: sentinel, onSentinel: onSentinel}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	n, sawSentinel, err := p.writeLocked(b)
	// Fire the readiness callback outside the lock: it runs external code (it
	// closes a channel).
	if sawSentinel {
		p.once.Do(p.onSentinel)
	}
	return n, err
}

func (p *prefixWriter) writeLocked(b []byte) (int, bool, error) {
	prefixWriterMu.Lock()
	defer prefixWriterMu.Unlock()

	n := len(b)
	p.buf.Write(b)
	sawSentinel := false
	for {
		line, err := p.buf.ReadBytes('\n')
		if err != nil {
			// no full line yet, keep the partial for next write
			p.buf.Reset()
			p.buf.Write(line)
			break
		}
		if _, werr := io.WriteString(p.w, p.prefix); werr != nil {
			return n, sawSentinel, werr
		}
		if _, werr := p.w.Write(line); werr != nil {
			return n, sawSentinel, werr
		}
		if p.sentinel != "" && p.onSentinel != nil && bytes.Contains(line, []byte(p.sentinel)) {
			sawSentinel = true
		}
	}
	return n, sawSentinel, nil
}
