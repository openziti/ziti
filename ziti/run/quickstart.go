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
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openziti/ziti/v2/ziti/cmd/edge"
	"github.com/openziti/ziti/v2/ziti/enroll"

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/controller/rest_client/cluster"
	"github.com/openziti/ziti/v2/ziti/cmd/agentcli"
	"github.com/openziti/ziti/v2/ziti/cmd/api"
	"github.com/openziti/ziti/v2/ziti/cmd/common"
	"github.com/openziti/ziti/v2/ziti/cmd/console"
	"github.com/openziti/ziti/v2/ziti/cmd/create"
	"github.com/openziti/ziti/v2/ziti/cmd/helpers"
	"github.com/openziti/ziti/v2/ziti/cmd/pki"
	"github.com/openziti/ziti/v2/ziti/constants"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

type QuickstartOpts struct {
	Username           string
	Password           string
	AlreadyInitialized bool
	Home               string
	ControllerAddress  string
	ControllerPort     uint16
	RouterAddress      string
	RouterPort         uint16
	out                io.Writer
	errOut             io.Writer
	cleanOnExit        bool
	TrustDomain        string
	InstanceID         string
	ClusterMember      string
	Routerless         bool
	ConfigureAndExit   bool
	ConfigFile         string
	Zac                bool
	ZacVersion         string
	ZacLocation        string
	Yes                bool

	// flags is this invocation's parsed flag set, used to tell a user-supplied flag from a default.
	flags *pflag.FlagSet
	// ignoredSetupFlags are flags supplied on a re-run that only take effect when the environment is
	// first created, so they had no effect this run.
	ignoredSetupFlags []string

	joinCommand bool
	verbose     bool
	nonVoter    bool
}

func addCommonQuickstartFlags(cmd *cobra.Command, options *QuickstartOpts) {
	currentCtrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	currentCtrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	currentRouterAddy := helpers.GetRouterAdvertisedAddress()
	currentRouterPort := helpers.GetZitiEdgeRouterPort()
	defaultCtrlPort, _ := strconv.ParseInt(constants.DefaultCtrlEdgeAdvertisedPort, 10, 16)
	defaultRouterPort, _ := strconv.ParseInt(constants.DefaultZitiEdgeRouterPort, 10, 16)

	cmd.Flags().StringVarP(&options.Username, "username", "u", "", "admin username, default: admin")
	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "admin password, default: admin")

	cmd.Flags().StringVar(&options.Home, "home", "", "permanent directory")

	cmd.Flags().StringVar(&options.ControllerAddress, "ctrl-address", "", "sets the advertised address for the control plane and API. current: "+currentCtrlAddy)
	cmd.Flags().Uint16Var(&options.ControllerPort, "ctrl-port", uint16(defaultCtrlPort), "sets the port to use for the control plane and API. current: "+currentCtrlPort)
	cmd.Flags().StringVar(&options.RouterAddress, "router-address", "", "sets the advertised address for the integrated router. current: "+currentRouterAddy)
	cmd.Flags().Uint16Var(&options.RouterPort, "router-port", uint16(defaultRouterPort), "sets the port to use for the integrated router. current: "+currentRouterPort)
	cmd.Flags().BoolVar(&options.Routerless, "no-router", false, "specifies the quickstart should not start a router")

	cmd.Flags().BoolVar(&options.verbose, "verbose", false, "Show additional output.")
	cmd.Flags().BoolVar(&options.ConfigureAndExit, "configure-and-exit", false, "Configures everything and then exits gracefully")

	cmd.Flags().BoolVar(&options.Zac, "zac", false, "download the Ziti Admin Console (ZAC) and configure the controller to serve it")
	cmd.Flags().StringVar(&options.ZacVersion, "zac-version", "latest", "ZAC version to download when --zac is set")
	cmd.Flags().StringVar(&options.ZacLocation, "zac-location", "", "directory to install ZAC into; defaults to <home>/console")
	cmd.Flags().BoolVarP(&options.Yes, "yes", "y", false, "answer yes to prompts (e.g. replacing installed ZAC assets with a different --zac-version)")
}

func addQuickstartHaFlags(cmd *cobra.Command, options *QuickstartOpts) {
	cmd.Flags().StringVar(&options.TrustDomain, "trust-domain", "", "the specified trust domain to be used in SPIFFE ids.")
	cmd.Flags().StringVar(&options.InstanceID, "instance-id", "", "specifies a unique instance id for use in ha mode.")
}

// NewQuickStartCmd creates a command object for the "create" command
func NewQuickStartCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
	options := &QuickstartOpts{}
	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "runs a Controller and Router in quickstart mode",
		Long:  "runs a Controller and Router in quickstart mode with a temporary directory; suitable for testing and development",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.out = out
			options.errOut = errOut
			options.flags = cmd.Flags()
			if options.TrustDomain == "" {
				options.TrustDomain = "quickstart"
			}
			if options.InstanceID == "" {
				options.InstanceID = "instance-1"
			}
			return options.run(context)
		},
	}
	addCommonQuickstartFlags(cmd, options)
	addQuickstartHaFlags(cmd, options)
	cmd.AddCommand(NewQuickStartJoinClusterCmd(out, errOut, context))
	cmd.AddCommand(NewQuickStartClusterCmd(out, errOut, context))
	return cmd
}

func NewQuickStartJoinClusterCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
	options := &QuickstartOpts{}
	cmd := &cobra.Command{
		Use:   "join",
		Short: "runs a Controller and Router in quickstart mode and joins an existing cluster",
		Long:  "runs a Controller and Router in quickstart mode and joins an existing cluster with a temporary directory; suitable for testing and development",
		RunE: func(cmd *cobra.Command, args []string) error {
			options.out = out
			options.errOut = errOut
			options.flags = cmd.Flags()
			return options.join(context)
		},
	}
	addCommonQuickstartFlags(cmd, options)
	addQuickstartHaFlags(cmd, options)
	cmd.Flags().StringVarP(&options.ClusterMember, "cluster-member", "m", "", "address of a cluster member. required. example tls:localhost:1280")
	cmd.Flags().BoolVar(&options.nonVoter, "non-voting", false, "used with ha mode. specifies the member is a non-voting member")
	cmd.Hidden = true
	return cmd
}

func (o *QuickstartOpts) cleanupHome() {
	if o.cleanOnExit && !o.ConfigureAndExit {
		fmt.Println("Removing temp directory at: " + o.Home)
		_ = os.RemoveAll(o.Home)
	} else {
		fmt.Println("Environment left intact at: " + o.Home)
	}
}

func (o *QuickstartOpts) join(ctx context.Context) error {
	if strings.TrimSpace(o.InstanceID) == "" {
		return fmt.Errorf("--instance-id is required when joining a cluster")
	}
	if strings.TrimSpace(o.Home) == "" {
		return fmt.Errorf("--home is required when joining a cluster; the root-ca is used to create the server's pki")
	}
	if o.ClusterMember == "" {
		return fmt.Errorf("--cluster-member is required")
	}
	if strings.TrimSpace(o.TrustDomain) == "" {
		return fmt.Errorf("--trust-domain is required when joining a cluster")
	}

	o.joinCommand = true
	return o.run(ctx)
}

func (o *QuickstartOpts) run(ctx context.Context) error {
	if o.verbose {
		pfxlog.GlobalInit(logrus.DebugLevel, pfxlog.DefaultOptions().Color())
	}

	//set env vars
	if o.Home == "" {
		tmpDir, _ := os.MkdirTemp("", "quickstart")
		o.Home = tmpDir
		o.cleanOnExit = true
		logrus.Infof("temporary --home '%s'", o.Home)
	} else {
		//normalize path
		if strings.HasPrefix(o.Home, "~") {
			usr, err := user.Current()
			if err != nil {
				return fmt.Errorf("could not find user's home directory")
			}
			home := usr.HomeDir
			// Replace only the first instance of ~ in case it appears later in the path
			o.Home = strings.Replace(o.Home, "~", home, 1)
		}
		logrus.Infof("permanent --home '%s' will not be removed on exit", o.Home)
	}
	if o.Username == "" {
		o.Username = "admin"
	}
	if o.Password == "" {
		o.Password = "admin"
	}

	if o.InstanceID == "" {
		o.InstanceID = uuid.New().String()
	}

	o.ConfigFile = path.Join(o.instHome(), "ctrl.yaml")
	routerName := "router-" + o.InstanceID

	//ZITI_HOME=/tmp ziti create config controller | grep -v "#" | sed -E 's/^ *$//g' | sed '/^$/d'
	_ = os.Setenv("ZITI_HOME", o.Home)
	pkiLoc := path.Join(o.Home, "pki")
	rootLoc := path.Join(pkiLoc, "root-ca")
	pkiIntermediateName := o.scopedName("intermediate-ca")
	pkiServerName := o.scopedNameOff("server")
	pkiClientName := o.scopedNameOff("client")
	intermediateLoc := path.Join(pkiLoc, pkiIntermediateName)
	_ = os.Setenv("ZITI_PKI_CTRL_CA", path.Join(rootLoc, "certs", "root-ca.cert"))
	_ = os.Setenv("ZITI_PKI_CTRL_KEY", path.Join(intermediateLoc, "keys", pkiServerName+".key"))
	_ = os.Setenv("ZITI_PKI_CTRL_SERVER_CERT", path.Join(intermediateLoc, "certs", pkiServerName+".chain.pem"))
	_ = os.Setenv("ZITI_PKI_CTRL_CERT", path.Join(intermediateLoc, "certs", pkiClientName+".chain.pem"))
	_ = os.Setenv("ZITI_PKI_SIGNER_CERT", path.Join(intermediateLoc, "certs", pkiIntermediateName+".cert"))
	_ = os.Setenv("ZITI_PKI_SIGNER_KEY", path.Join(intermediateLoc, "keys", pkiIntermediateName+".key"))

	routerNameFromEnv := os.Getenv(constants.ZitiEdgeRouterNameVarName)
	if routerNameFromEnv != "" {
		routerName = routerNameFromEnv
	}

	dbDir := path.Join(o.instHome(), "db")
	if _, statErr := os.Stat(dbDir); !os.IsNotExist(statErr) {
		o.AlreadyInitialized = true
	}

	if o.AlreadyInitialized {
		// The controller and router boot from the configs written when the environment was created.
		// Report any supplied flags that no longer take effect, then use the persisted address/port.
		o.noteIgnoredSetupFlags()
		o.applyConfiguredEndpoints(routerName)
	} else {
		if o.ControllerAddress != "" {
			_ = os.Setenv(constants.CtrlAdvertisedAddressVarName, o.ControllerAddress)
			_ = os.Setenv(constants.CtrlEdgeAdvertisedAddressVarName, o.ControllerAddress)
		}
		if o.ControllerPort > 0 {
			_ = os.Setenv(constants.CtrlAdvertisedPortVarName, strconv.Itoa(int(o.ControllerPort)))
			_ = os.Setenv(constants.CtrlEdgeAdvertisedPortVarName, strconv.Itoa(int(o.ControllerPort)))
		}
		if o.RouterAddress != "" {
			_ = os.Setenv(constants.ZitiEdgeRouterAdvertisedAddressVarName, o.RouterAddress)
		}
		if o.RouterPort > 0 {
			_ = os.Setenv(constants.ZitiEdgeRouterPortVarName, strconv.Itoa(int(o.RouterPort)))
			_ = os.Setenv(constants.ZitiEdgeRouterListenerBindPortVarName, strconv.Itoa(int(o.RouterPort)))
		}
	}

	// Install the console assets before the controller starts.
	if o.Zac {
		if o.ZacLocation == "" {
			o.ZacLocation = path.Join(o.Home, "console")
		}
		// Normalize separators to match the location the config generator writes when creating the config.
		o.ZacLocation = helpers.NormalizePath(o.ZacLocation)
		version, zacErr := console.EnsureAssets(o.out, o.ZacVersion, o.ZacLocation, o.Yes)
		if zacErr != nil {
			return fmt.Errorf("failed to install ZAC: %w", zacErr)
		}
		if version != "" {
			logrus.Infof("ZAC %s ready at '%s'", version, o.ZacLocation)
		}
		// Read by controller config generation, which emits the "spa" web binding when it writes the config.
		_ = os.Setenv(constants.CtrlConsoleLocationVarName, o.ZacLocation)
	}

	if !o.AlreadyInitialized {
		_ = os.MkdirAll(dbDir, 0o700)
		logrus.Debugf("made directory '%s'", dbDir)

		if err := o.CreateMinimalPki(); err != nil {
			return err
		}

		_ = os.Setenv("ZITI_HOME", o.instHome())
		ctrl := create.NewCmdCreateConfigController()
		ctrl.SetArgs([]string{
			fmt.Sprintf("--output=%s", o.ConfigFile),
		})
		if err := ctrl.Execute(); err != nil {
			return err
		}
	}

	// On a re-run the config already exists and was not regenerated. Reconcile the "spa" binding:
	// add it if the first run omitted --zac, or point it at the current --zac-location.
	if o.Zac && o.AlreadyInitialized {
		cfg := &console.ConfigureOptions{
			Out:        o.out,
			Err:        o.errOut,
			In:         os.Stdin,
			ConfigFile: o.ConfigFile,
			All:        true,
			Location:   o.ZacLocation,
			Path:       "zac",
			IndexFile:  "index.html",
			Yes:        true,
		}
		if err := cfg.Run(); err != nil {
			return fmt.Errorf("failed to configure console binding: %w", err)
		}
	}

	curCtx, cancel := context.WithCancel(ctx) //used to cancel controller and router when configure-and-exit is selected
	fmt.Println("Starting controller...")
	go func() {
		runCtrl := NewRunControllerCmd()
		runCtrl.SetArgs([]string{
			o.ConfigFile,
		})
		runCtrl.SetContext(curCtx)
		runCtrlErr := runCtrl.Execute()
		if runCtrlErr != nil {
			logrus.Errorf("controller exited with error: %v", runCtrlErr)
		}
	}()
	fmt.Println("Controller running...")

	o.ControllerAddress = helpers.GetCtrlEdgeAdvertisedAddress()

	portStr := helpers.GetCtrlEdgeAdvertisedPort()
	port, portErr := strconv.Atoi(portStr)
	if portErr != nil {
		cancel()
		return fmt.Errorf("invalid controller port: %s", portStr)
	}
	o.ControllerPort = uint16(port)

	c := make(chan error)
	timeout := 30 * time.Second

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go waitForController(ctx, o.ControllerHostPort(), c)

	select {
	case <-c:
		//completed normally
		logrus.Info("Controller online. Continuing...")
	case <-time.After(timeout):
		o.cleanupHome()
		cancel()
		return fmt.Errorf("timed out waiting for controller: %s", o.ControllerHostPort())
	}

	p := common.NewOptionsProvider(o.out, o.errOut)
	fmt.Println("waiting three seconds for controller to become ready...")

	// Target our own ops-agent by its socket path rather than --pid. With --pid the
	// agent client enumerates and contacts every gops socket on the host, which is
	// slow when stale sockets accumulate. --app-addr dials us directly.
	agentSock := "unix:" + path.Join(os.TempDir(), fmt.Sprintf("gops-agent.%d.sock", os.Getpid()))

	if o.AlreadyInitialized {
		logrus.Infof("instance %s already initialized; skipping cluster init/join and rejoining the existing cluster", o.InstanceID)
	} else if !o.joinCommand {
		maxRetries := 5
		for attempt := 1; attempt <= maxRetries; attempt++ {
			fmt.Printf("initializing controller at port: %d\n", o.ControllerPort)
			agentInitCmd := agentcli.NewAgentClusterInit(p)
			args := []string{
				o.Username,
				o.Password,
				o.Username,
				fmt.Sprintf("--app-addr=%s", agentSock),
				"--timeout=30s",
			}
			agentInitCmd.SetArgs(args)

			agentInitErr := agentInitCmd.Execute()
			if agentInitErr != nil {
				if attempt < maxRetries {
					fmt.Println("initialization failed. waiting two seconds and trying again")
					time.Sleep(2 * time.Second) // Wait before retrying
				} else {
					fmt.Println("Max retries reached. Failing.")
					cancel()
					return agentInitErr
				}
			} else {
				break
			}
		}
	} else {
		// Joining can fail transiently while the target elects a leader, so retry until it succeeds or the deadline elapses.
		joinDeadline := time.Now().Add(90 * time.Second)
		attempt := 0
		for {
			attempt++
			agentJoinCmd := agentcli.NewAgentClusterAdd(p)
			agentJoinCmd.SetArgs([]string{
				o.ClusterMember,
				fmt.Sprintf("--app-addr=%s", agentSock),
				fmt.Sprintf("--voter=%t", !o.nonVoter),
				"--timeout=30s",
			})

			joinErr := agentJoinCmd.Execute()
			if joinErr == nil {
				logrus.Infof("add command successful after %d attempt(s). continuing...", attempt)
				break
			}
			if time.Now().After(joinDeadline) {
				o.cleanupHome()
				cancel()
				return fmt.Errorf("failed to join cluster after %d attempt(s): %w", attempt, joinErr)
			}
			logrus.Warnf("join attempt %d failed: %v, retrying", attempt, joinErr)
			time.Sleep(2 * time.Second)
		}
	}

	erConfigFile := path.Join(o.instHome(), routerName+".yaml")
	err := o.configureRouter(ctx, routerName, erConfigFile, o.ControllerHostPort())
	if err != nil {
		cancel()
		return err
	}
	o.runRouter(erConfigFile)

	ch := make(chan os.Signal, 1)
	// os.Interrupt also catches a relayed Windows CTRL_BREAK which the Go runtime maps to SIGINT
	signal.Notify(ch, os.Interrupt, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)

	if !o.Routerless {
		r := make(chan error)
		timeout, _ = time.ParseDuration("30s")
		logrus.Infof("waiting for router at: %s:%d", o.RouterAddress, o.RouterPort)
		go o.WaitForRouter(timeout, r)
		select {
		case waitErr := <-r:
			if waitErr != nil {
				o.cleanupHome()
				cancel()
				return fmt.Errorf("router failed: %w", waitErr)
			}
		case <-time.After(timeout):
			o.cleanupHome()
			cancel()
			return fmt.Errorf("timed out waiting for router on port: %d", o.RouterPort)
		}
	}

	go func() {
		time.Sleep(3 * time.Second) // output this after a bit...
		nextInstId := incrementStringSuffix(o.InstanceID)
		fmt.Println()
		o.printDetails()
		fmt.Println("=======================================================================================")
		cont := "\\"
		if os.Getenv("PSModulePath") != "" {
			cont = "`"
		}
		fmt.Println("Quickly add another member to this cluster using: ")
		fmt.Printf("  ziti run quickstart join %s\n", cont)
		fmt.Printf("    --ctrl-port %d %s\n", o.ControllerPort+1, cont)
		fmt.Printf("    --router-port %d %s\n", o.RouterPort+1, cont)
		fmt.Printf("    --home \"%s\" %s\n", o.Home, cont)
		fmt.Printf("    --trust-domain=\"%s\" %s\n", o.TrustDomain, cont)
		fmt.Printf("    --cluster-member tls:%s:%d %s\n", o.ControllerAddress, o.ControllerPort, cont)
		fmt.Printf("    --instance-id \"%s\"\n", nextInstId)
		fmt.Println("=======================================================================================")
		fmt.Println()
	}()

	if !o.ConfigureAndExit {
		select {
		case <-ch:
			fmt.Println("Signal to shutdown received")
		case <-ctx.Done():
			fmt.Println("Cancellation request received")
		}
	} else {
		fmt.Println("configure-and-exit set to true, closing router and controller")
	}

	o.cleanupHome()
	cancel()
	return nil
}

func (o *QuickstartOpts) printDetails() {
	fmt.Println("=======================================================================================")
	fmt.Println("controller and router started.")
	fmt.Println("    controller located at  : " + helpers.GetCtrlAdvertisedAddress() + ":" + strconv.Itoa(int(o.ControllerPort)))
	fmt.Println("    router located at      : " + helpers.GetRouterAdvertisedAddress() + ":" + strconv.Itoa(int(o.RouterPort)))
	fmt.Println("    config dir located at  : " + o.Home)
	fmt.Println("    configured trust domain: " + o.TrustDomain)
	fmt.Printf("    instance pid           : %d\n", os.Getpid())
	if o.Zac {
		// The console is served on the edge listener, so use the edge address and port.
		fmt.Printf("    console (ZAC) at       : https://%s:%s/zac\n", helpers.GetCtrlEdgeAdvertisedAddress(), helpers.GetCtrlEdgeAdvertisedPort())
	}
	if len(o.ignoredSetupFlags) > 0 {
		fmt.Println()
		fmt.Println("    NOTE: --home already exists. these flags only take effect when first creating it, so they were ignored: " + strings.Join(o.ignoredSetupFlags, ", "))
	}
}

// noteIgnoredSetupFlags records, for the closing banner, the flags supplied on a re-run of an existing
// --home that only take effect when the environment is first created. Cobra flags always carry a value,
// so o.flags.Changed distinguishes a flag the user typed from one left at its default. username and
// password are excluded: a crash-resume login still uses them.
func (o *QuickstartOpts) noteIgnoredSetupFlags() {
	setupOnlyFlags := []string{"ctrl-address", "ctrl-port", "router-address", "router-port", "trust-domain"}
	o.ignoredSetupFlags = nil
	for _, name := range setupOnlyFlags {
		if o.flags != nil && o.flags.Changed(name) {
			o.ignoredSetupFlags = append(o.ignoredSetupFlags, "--"+name)
		}
	}
}

// applyConfiguredEndpoints sets the advertised address/port from the existing controller and router
// configs. Best-effort: on a read or parse error it logs and leaves the current values unchanged.
func (o *QuickstartOpts) applyConfiguredEndpoints(routerName string) {
	if host, port, ok := readAdvertisedFromCtrlConfig(o.ConfigFile); ok {
		o.ControllerAddress = host
		o.ControllerPort = port
		// The ctrl-plane and edge address/port are equal in quickstart, so the edge value sets both.
		_ = os.Setenv(constants.CtrlAdvertisedAddressVarName, host)
		_ = os.Setenv(constants.CtrlEdgeAdvertisedAddressVarName, host)
		_ = os.Setenv(constants.CtrlAdvertisedPortVarName, strconv.Itoa(int(port)))
		_ = os.Setenv(constants.CtrlEdgeAdvertisedPortVarName, strconv.Itoa(int(port)))
	} else {
		logrus.Warnf("could not read persisted controller address from '%s', using flag/default values", o.ConfigFile)
	}

	if o.Routerless {
		return
	}
	routerCfg := path.Join(o.instHome(), routerName+".yaml")
	if host, port, ok := readAdvertisedFromRouterConfig(routerCfg); ok {
		o.RouterAddress = host
		o.RouterPort = port
		_ = os.Setenv(constants.ZitiEdgeRouterAdvertisedAddressVarName, host)
		_ = os.Setenv(constants.ZitiEdgeRouterPortVarName, strconv.Itoa(int(port)))
		_ = os.Setenv(constants.ZitiEdgeRouterListenerBindPortVarName, strconv.Itoa(int(port)))
	} else {
		logrus.Warnf("could not read persisted router address from '%s', using flag/default values", routerCfg)
	}
}

// readAdvertisedFromCtrlConfig returns the advertised host and port from the first web listener
// bind point in a controller config file.
func readAdvertisedFromCtrlConfig(file string) (string, uint16, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", 0, false
	}
	var cfg struct {
		Web []struct {
			BindPoints []struct {
				Address string `yaml:"address"`
			} `yaml:"bindPoints"`
		} `yaml:"web"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", 0, false
	}
	for _, w := range cfg.Web {
		for _, bp := range w.BindPoints {
			if h, p, ok := splitHostPortU16(bp.Address); ok {
				return h, p, true
			}
		}
	}
	return "", 0, false
}

// readAdvertisedFromRouterConfig returns the advertised host and port of the edge listener in a
// router config file.
func readAdvertisedFromRouterConfig(file string) (string, uint16, bool) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", 0, false
	}
	var cfg struct {
		Listeners []struct {
			Binding string `yaml:"binding"`
			Options struct {
				Advertise string `yaml:"advertise"`
			} `yaml:"options"`
		} `yaml:"listeners"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", 0, false
	}
	for _, l := range cfg.Listeners {
		if l.Binding == "edge" {
			if h, p, ok := splitHostPortU16(l.Options.Advertise); ok {
				return h, p, true
			}
		}
	}
	return "", 0, false
}

// splitHostPortU16 parses a "host:port" string into its host and a uint16 port.
func splitHostPortU16(addr string) (string, uint16, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", 0, false
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, false
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 || p > 65535 {
		return "", 0, false
	}
	return host, uint16(p), true
}

func (o *QuickstartOpts) configureRouter(ctx context.Context, routerName string, configFile string, ctrlUrl string) error {
	if o.Routerless {
		return nil
	}

	erJwt := path.Join(o.Home, routerName+".jwt")

	if !o.AlreadyInitialized {
		if o.joinCommand {
			o.waitForLeader()
		}
		if loginErr := o.loginWithRetry(ctx, 60*time.Second); loginErr != nil {
			return loginErr
		}

		if err := o.configureOverlay(); err != nil {
			return err
		}

		time.Sleep(1 * time.Second)

		// ziti edge create edge-router ${ZITI_HOSTNAME}-edge-router -o ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt -t -a public
		createErCmd := edge.NewCreateEdgeRouterCmd(o.out, o.errOut)
		createErCmd.SetArgs([]string{
			routerName,
			fmt.Sprintf("--jwt-output-file=%s", erJwt),
			"--tunneler-enabled",
			fmt.Sprintf("--role-attributes=%s", "public"),
		})

		o.waitForLeader() //wait for a leader before doing anything
		createErErr := createErCmd.Execute()

		if createErErr != nil {
			return createErErr
		}
	}

	// Create router config YAML if it doesn't exist yet (may have been skipped on a prior crashed run)
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// ziti create config router edge --routerName ${ZITI_HOSTNAME}-edge-router >${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml
		opts := &create.CreateConfigRouterOptions{}

		data := &create.ConfigTemplateValues{}
		data.PopulateConfigValues()
		create.SetZitiRouterIdentity(&data.Router, routerName)
		erCfg := create.NewCmdCreateConfigRouterEdge(opts, data)
		erCfg.SetArgs([]string{
			fmt.Sprintf("--routerName=%s", routerName),
			fmt.Sprintf("--output=%s", configFile),
		})

		o.waitForLeader()
		erCfgErr := erCfg.Execute()
		if erCfgErr != nil {
			return erCfgErr
		}
	}

	// Enroll router if JWT file still exists (consumed on successful enrollment)
	if _, err := os.Stat(erJwt); err == nil {
		if o.AlreadyInitialized {
			if loginErr := o.loginWithRetry(ctx, 60*time.Second); loginErr != nil {
				return loginErr
			}
		}

		// ziti router enroll ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml --jwt ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt
		erEnroll := enroll.NewEnrollEdgeRouterCmd()
		erEnroll.SetArgs([]string{
			configFile,
			fmt.Sprintf("--jwt=%s", erJwt),
		})

		o.waitForLeader() //needed?
		erEnrollErr := erEnroll.Execute()
		if erEnrollErr != nil {
			return erEnrollErr
		}

		// Remove the JWT file now that enrollment succeeded, so subsequent
		// restarts don't attempt to re-enroll.
		if removeErr := os.Remove(erJwt); removeErr != nil {
			logrus.Warnf("failed to remove router JWT after enrollment: %v", removeErr)
		}
	}

	return nil
}

func (o *QuickstartOpts) runRouter(configFile string) {
	if o.Routerless {
		return
	}

	go func() {
		// ziti router run ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml &> ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.log &
		erRunCmd := NewRunRouterCmd()
		erRunCmd.SetArgs([]string{
			configFile,
		})

		o.waitForLeader() //needed?
		erRunCmdErr := erRunCmd.Execute()
		if erRunCmdErr != nil {
			logrus.Errorf("router exited with error: %v", erRunCmdErr)
		}
	}()
}

func (o *QuickstartOpts) CreateMinimalPki() error {
	where := path.Join(o.Home, "pki")
	fmt.Println("emitting a minimal PKI")

	sid := fmt.Sprintf("spiffe://%s/controller/%s", o.TrustDomain, o.InstanceID)

	//ziti pki create ca --pki-root="$pkiDir" --ca-file="root-ca" --ca-name="root-ca" --spiffe-id="whatever"
	if o.joinCommand {
		// indicates we are joining a cluster. don't emit a root-ca, expect it'll be there or error
	} else {
		rootCaPath := path.Join(where, "root-ca", "certs", "root-ca.cert")
		rootCa, statErr := os.Stat(rootCaPath)
		if statErr != nil {
			if !os.IsNotExist(statErr) {
				logrus.Warnf("could not check for root-ca: %v", statErr)
			}
		}
		if rootCa == nil {
			ca := pki.NewCmdPKICreateCA(o.out, o.errOut)
			rootCaArgs := []string{
				fmt.Sprintf("--pki-root=%s", where),
				fmt.Sprintf("--ca-file=%s", "root-ca"),
				fmt.Sprintf("--ca-name=%s", "root-ca"),
				fmt.Sprintf("--trust-domain=%s", o.TrustDomain),
			}

			ca.SetArgs(rootCaArgs)
			pkiErr := ca.Execute()
			if pkiErr != nil {
				return pkiErr
			}

		} else {
			logrus.Infof("%s exists and will be reused", rootCaPath)
		}
	}

	//ziti pki create intermediate --pki-root "$pkiDir" --ca-name "root-ca" --intermediate-name "intermediate-ca" --intermediate-file "intermediate-ca" --max-path-len "1" --spiffe-id="whatever"
	intermediate := pki.NewCmdPKICreateIntermediate(o.out, o.errOut)
	intermediate.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", "root-ca"),
		fmt.Sprintf("--intermediate-name=%s", o.scopedName("intermediate-ca")),
		fmt.Sprintf("--intermediate-file=%s", o.scopedName("intermediate-ca")),
		"--max-path-len=1",
	})
	intErr := intermediate.Execute()
	if intErr != nil {
		return intErr
	}

	//ziti pki create server --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --server-name "server" --server-file "server" --dns "localhost,${ZITI_HOSTNAME}" --spiffe-id="whatever"
	svr := pki.NewCmdPKICreateServer(o.out, o.errOut)
	var ips = "127.0.0.1,::1"
	ipOverride := os.Getenv("ZITI_CTRL_EDGE_IP_OVERRIDE")
	if ipOverride != "" {
		ips = ips + "," + ipOverride
	}
	args := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", o.scopedName("intermediate-ca")),
		fmt.Sprintf("--server-name=%s", o.InstanceID),
		fmt.Sprintf("--server-file=%s", o.scopedNameOff("server")),
		fmt.Sprintf("--dns=%s,%s", "localhost", helpers.GetCtrlAdvertisedAddress()),
		fmt.Sprintf("--ip=%s", ips),
		fmt.Sprintf("--spiffe-id=%s", sid),
	}

	svr.SetArgs(args)
	svrErr := svr.Execute()
	if svrErr != nil {
		return svrErr
	}

	//ziti pki create client --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --client-name "client" --client-file "client" --key-file "server" --spiffe-id="whatever"
	client := pki.NewCmdPKICreateClient(o.out, o.errOut)
	client.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", o.scopedName("intermediate-ca")),
		fmt.Sprintf("--client-name=%s", o.InstanceID),
		fmt.Sprintf("--client-file=%s", o.scopedNameOff("client")),
		fmt.Sprintf("--key-file=%s", o.scopedNameOff("server")),
		fmt.Sprintf("--spiffe-id=%s", sid),
	})
	clientErr := client.Execute()
	if clientErr != nil {
		return clientErr
	}
	return nil
}

func waitForController(ctx context.Context, ctrlUrl string, done chan error) {
	if !strings.HasPrefix(ctrlUrl, "https://") {
		ctrlUrl = "https://" + ctrlUrl
	}
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr}
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			done <- ctx.Err()
			return
		case <-ticker.C:
			fmt.Printf("waiting for controller: %s\n", ctrlUrl)
		default:
			r, e := client.Get(ctrlUrl)
			if e == nil && r != nil && r.StatusCode == 200 {
				done <- nil
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (o *QuickstartOpts) WaitForRouter(timeout time.Duration, done chan error) {
	for {
		select {
		case <-time.After(timeout):
			done <- fmt.Errorf("router not available after %s at %s:%d", timeout, o.RouterAddress, o.RouterPort)
			return
		default:
			addr := net.JoinHostPort(o.RouterAddress, strconv.Itoa(int(o.RouterPort)))
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				_ = conn.Close()
				fmt.Printf("Router is available on %s:%d\n", o.RouterAddress, o.RouterPort)
				done <- nil
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
}

func (o *QuickstartOpts) scopedNameOff(name string) string {
	return name
}
func (o *QuickstartOpts) scopedName(name string) string {
	if o.InstanceID != "" {
		return name + "-" + o.InstanceID
	} else {
		return name
	}
}

func (o *QuickstartOpts) instHome() string {
	return path.Join(o.Home, o.InstanceID)
}

func (o *QuickstartOpts) configureOverlay() error {
	if o.joinCommand {
		return nil
	}

	// Allow all identities to use any edge router with the "public" attribute
	// ziti edge create edge-router-policy all-endpoints-public-routers --edge-router-roles "#public" --identity-roles "#all"
	erpCmd := edge.NewCreateEdgeRouterPolicyCmd(o.out, o.errOut)
	erpCmd.SetArgs([]string{
		"all-endpoints-public-routers",
		fmt.Sprintf("--edge-router-roles=%s", "#public"),
		fmt.Sprintf("--identity-roles=%s", "#all"),
	})
	erpCmdErr := erpCmd.Execute()
	if erpCmdErr != nil {
		return erpCmdErr
	}

	// # Allow all edge-routers to access all services
	// ziti edge create service-edge-router-policy all-routers-all-services --edge-router-roles "#all" --service-roles "#all"
	serpCmd := edge.NewCreateServiceEdgeRouterPolicyCmd(o.out, o.errOut)
	serpCmd.SetArgs([]string{
		"all-routers-all-services",
		fmt.Sprintf("--edge-router-roles=%s", "#all"),
		fmt.Sprintf("--service-roles=%s", "#all"),
	})
	o.waitForLeader()
	serpCmdErr := serpCmd.Execute()
	if serpCmdErr != nil {
		return serpCmdErr
	}
	return nil
}

type raftListMembersAction struct {
	api.Options
}

// loginWithRetry runs `ziti edge login` and retries on failure until it succeeds
// or the timeout elapses. This handles the post-join window where the AddPeer
// raft command has committed cluster membership but the admin authenticator row
// has not yet replicated into the local FSM, which causes a single-shot login to
// fail with 401. Mirrors the bounded retry the bash quickstart performs.
func (o *QuickstartOpts) loginWithRetry(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	attempt := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		loginCmd := edge.NewLoginCmd(o.out, o.errOut)
		loginCmd.SetArgs([]string{
			o.ControllerHostPort(),
			fmt.Sprintf("--username=%s", o.Username),
			fmt.Sprintf("--password=%s", o.Password),
			"-y",
		})
		attempt++
		err := loginCmd.Execute()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("login failed after %s (%d attempts): %w", timeout, attempt, err)
		}
		fmt.Printf("login attempt %d failed: %v -- retrying...\n", attempt, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

func (o *QuickstartOpts) waitForLeader() bool {
	for {
		p := common.NewOptionsProvider(o.out, o.errOut)
		action := &raftListMembersAction{
			Options: api.Options{CommonOptions: p()},
		}

		client, err := util.NewFabricManagementClient(action)
		if err != nil {
			return false
		}
		members, err := client.Cluster.ClusterListMembers(&cluster.ClusterListMembersParams{
			Context: context.Background(),
		}, nil)

		if err != nil {
			return false
		}
		for _, m := range members.Payload.Data {
			if m.Leader != nil && *m.Leader {
				time.Sleep(500 * time.Millisecond) // this just gives time for the leader to 'settle' -- shouldn't be necessary
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func incrementStringSuffix(input string) string {
	// Regular expression to capture the numeric suffix
	re := regexp.MustCompile(`(\d+)$`)
	match := re.FindStringSubmatch(input)

	if len(match) == 0 {
		return uuid.New().String()
	}

	numStr := match[1]
	numLength := len(numStr)
	num, _ := strconv.Atoi(numStr)
	num++

	incremented := fmt.Sprintf("%0*d", numLength, num)

	return strings.TrimSuffix(input, numStr) + incremented
}

func (o *QuickstartOpts) ControllerHostPort() string {
	return net.JoinHostPort(o.ControllerAddress, strconv.Itoa(int(o.ControllerPort)))
}
func (o *QuickstartOpts) RouterHostPort() string {
	return net.JoinHostPort(o.RouterAddress, strconv.Itoa(int(o.RouterPort)))
}
