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
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/enroll"
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

	"github.com/google/uuid"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/controller/rest_client/cluster"
	"github.com/openziti/ziti/ziti/cmd/agentcli"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/create"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/pki"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/openziti/ziti/ziti/util"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
	joinCommand        bool
	verbose            bool
	nonVoter           bool
	routerless         bool
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
	cmd.Flags().BoolVar(&options.routerless, "no-router", false, "specifies the quickstart should not start a router")

	cmd.Flags().BoolVar(&options.verbose, "verbose", false, "Show additional output.")
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
			if options.TrustDomain == "" {
				options.TrustDomain = uuid.New().String()
				fmt.Println("Trust domain was not supplied. Using a random trust domain: " + options.TrustDomain)
			}
			err := options.run(context)
			if err != nil {
				logrus.Fatal(err)
			}
			return nil
		},
	}
	addCommonQuickstartFlags(cmd, options)
	cmd.AddCommand(NewQuickStartJoinClusterCmd(out, errOut, context))
	return cmd
}

func NewQuickStartJoinClusterCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
	options := &QuickstartOpts{}
	cmd := &cobra.Command{
		Use:   "join",
		Short: "runs a Controller and Router in quickstart mode and joins an existing cluster",
		Long:  "runs a Controller and Router in quickstart mode and joins an existing cluster with a temporary directory; suitable for testing and development",
		Run: func(cmd *cobra.Command, args []string) {
			options.out = out
			options.errOut = errOut
			err := options.join(context)
			if err != nil {
				logrus.Fatal(err)
			}
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
	if o.cleanOnExit && !o.joinCommand {
		fmt.Println("Removing temp directory at: " + o.Home)
		_ = os.RemoveAll(o.Home)
	} else {
		fmt.Println("Environment left intact at: " + o.Home)
	}
}

func (o *QuickstartOpts) join(ctx context.Context) error {
	if strings.TrimSpace(o.InstanceID) == "" {
		logrus.Fatalf("the instance-id is required when joining a cluster")
	}
	if strings.TrimSpace(o.Home) == "" {
		logrus.Fatalf("the home directory must be specified when joining an existing cluster. the root-ca is used to create the server's pki")
	}

	if o.ClusterMember == "" {
		logrus.Fatalf("--cluster-member is required")
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
	if o.Username == "" {
		o.Username = "admin"
	}
	if o.Password == "" {
		o.Password = "admin"
	}

	if o.InstanceID == "" {
		o.InstanceID = uuid.New().String()
	}

	ctrlYaml := path.Join(o.instHome(), "ctrl.yaml")
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
	if _, err := os.Stat(dbDir); !os.IsNotExist(err) {
		o.AlreadyInitialized = true
	} else {
		_ = os.MkdirAll(dbDir, 0o700)
		logrus.Debugf("made directory '%s'", dbDir)

		o.createMinimalPki()

		_ = os.Setenv("ZITI_HOME", o.instHome())
		ctrl := create.NewCmdCreateConfigController()
		args := []string{
			fmt.Sprintf("--output=%s", ctrlYaml),
		}
		ctrl.SetArgs(args)
		err = ctrl.Execute()
		if err != nil {
			logrus.Fatal(err)
		}
	}

	fmt.Println("Starting controller...")
	go func() {
		runCtrl := NewRunControllerCmd()
		runCtrl.SetArgs([]string{
			ctrlYaml,
		})
		runCtrlErr := runCtrl.Execute()
		if runCtrlErr != nil {
			logrus.Fatal(runCtrlErr)
		}
	}()
	fmt.Println("Controller running...")

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	c := make(chan error)
	timeout, _ := time.ParseDuration("30s")
	go waitForController(ctrlUrl, c)
	select {
	case <-c:
		//completed normally
		logrus.Info("Controller online. Continuing...")
	case <-time.After(timeout):
		o.cleanupHome()
		return fmt.Errorf("timed out waiting for controller: %s", ctrlUrl)
	}

	p := common.NewOptionsProvider(o.out, o.errOut)
	fmt.Println("waiting three seconds for controller to become ready...")

	if !o.joinCommand {
		maxRetries := 5
		for attempt := 1; attempt <= maxRetries; attempt++ {
			fmt.Printf("initializing controller at port: %d\n", o.ControllerPort)
			agentInitCmd := agentcli.NewAgentClusterInit(p)
			pid := os.Getpid()
			args := []string{
				o.Username,
				o.Password,
				o.Username,
				fmt.Sprintf("--pid=%d", pid),
			}
			agentInitCmd.SetArgs(args)

			agentInitErr := agentInitCmd.Execute()
			if agentInitErr != nil {
				if attempt < maxRetries {
					fmt.Println("initialization failed. waiting two seconds and trying again")
					time.Sleep(2 * time.Second) // Wait before retrying
				} else {
					fmt.Println("Max retries reached. Failing.")
					return agentInitErr
				}
			} else {
				break
			}
		}
	} else {
		agentJoinCmd := agentcli.NewAgentClusterAdd(p)

		args := []string{
			o.ClusterMember,
			fmt.Sprintf("--pid=%d", os.Getpid()),
			fmt.Sprintf("--voter=%t", !o.nonVoter),
		}
		agentJoinCmd.SetArgs(args)

		addChan := make(chan struct{})
		addTimeout := time.Second * 30
		go func() {
			o.waitForLeader()
			agentJoinErr := agentJoinCmd.Execute()
			if agentJoinErr != nil {
				logrus.Fatal(agentJoinErr)
			}
			close(addChan)
		}()

		select {
		case <-addChan:
			//completed normally
			logrus.Info("Add command successful. continuing...")
		case <-time.After(addTimeout):
			o.cleanupHome()
			return fmt.Errorf("timed out adding to cluster")
		}
	}

	erConfigFile := path.Join(o.instHome(), routerName+".yaml")
	err := o.configureRouter(routerName, erConfigFile, ctrlUrl)
	if err != nil {
		return err
	}
	o.runRouter(erConfigFile)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)

	if !o.routerless {
		r := make(chan struct{})
		timeout, _ = time.ParseDuration("30s")
		logrus.Infof("waiting for router at: %s:%d", o.RouterAddress, o.RouterPort)
		go waitForRouter(o.RouterAddress, o.RouterPort, r)
		select {
		case <-r:
			//completed normally
		case <-time.After(timeout):
			o.cleanupHome()
			return fmt.Errorf("timed out waiting for router on port: %d", o.RouterPort)
		}
	}

	go func() {
		time.Sleep(3 * time.Second) // output this after a bit... so that it probably is towards the end
		nextInstId := incrementStringSuffix(o.InstanceID)
		fmt.Println()
		o.printDetails()
		fmt.Println("=======================================================================================")
		fmt.Println("Quickly add another member to this cluster using: ")
		fmt.Printf("  ziti edge quickstart join \\\n")
		fmt.Printf("    --ctrl-port %d \\\n", o.ControllerPort+1)
		fmt.Printf("    --router-port %d \\\n", o.RouterPort+1)
		fmt.Printf("    --home \"%s\" \\\n", o.Home)
		fmt.Printf("    --trust-domain=\"%s\" \\\n", o.TrustDomain)
		fmt.Printf("    --cluster-member tls:%s:%s\\ \n", ctrlAddy, ctrlPort)
		fmt.Printf("    --instance-id \"%s\"\n", nextInstId)
		fmt.Println("=======================================================================================")
		fmt.Println()
	}()

	select {
	case <-ch:
		fmt.Println("Signal to shutdown received")
	case <-ctx.Done():
		fmt.Println("Cancellation request received")
	}

	o.cleanupHome()
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
}

func (o *QuickstartOpts) configureRouter(routerName string, configFile string, ctrlUrl string) error {
	if o.routerless {
		return nil
	}

	if !o.AlreadyInitialized {
		loginCmd := edge.NewLoginCmd(o.out, o.errOut)
		loginCmd.SetArgs([]string{
			ctrlUrl,
			fmt.Sprintf("--username=%s", o.Username),
			fmt.Sprintf("--password=%s", o.Password),
			"-y",
		})
		if o.joinCommand {
			o.waitForLeader()
		}
		loginErr := loginCmd.Execute()
		if loginErr != nil {
			logrus.Fatal(loginErr)
		}

		o.configureOverlay()

		time.Sleep(1 * time.Second)

		var erJwt string

		// ziti edge create edge-router ${ZITI_HOSTNAME}-edge-router -o ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt -t -a public
		createErCmd := edge.NewCreateEdgeRouterCmd(o.out, o.errOut)
		erJwt = path.Join(o.Home, routerName+".jwt")
		createErCmd.SetArgs([]string{
			routerName,
			fmt.Sprintf("--jwt-output-file=%s", erJwt),
			"--tunneler-enabled",
			fmt.Sprintf("--role-attributes=%s", "public"),
		})

		o.waitForLeader() //wait for a leader before doing anything
		createErErr := createErCmd.Execute()

		if createErErr != nil {
			logrus.Fatal(createErErr)
		}

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
			logrus.Fatal(erCfgErr)
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
	}
	return nil
}

func (o *QuickstartOpts) runRouter(configFile string) {
	if o.routerless {
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
			logrus.Fatal(erRunCmdErr)
		}
	}()
}

func (o *QuickstartOpts) createMinimalPki() {
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
				logrus.Fatal(pkiErr)
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
		logrus.Fatal(intErr)
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
		logrus.Fatal(svrErr)
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
		logrus.Fatal(clientErr)
	}
}

func waitForController(ctrlUrl string, done chan error) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	client := &http.Client{Transport: tr}
	for {
		r, e := client.Get(ctrlUrl)
		if e != nil || r == nil || r.StatusCode != 200 {
			time.Sleep(50 * time.Millisecond)
		} else {
			break
		}
	}
	done <- nil
}

func waitForRouter(address string, port uint16, done chan struct{}) {
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

func (o *QuickstartOpts) configureOverlay() {
	if o.joinCommand {
		return
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
		logrus.Fatal(erpCmdErr)
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
		logrus.Fatal(serpCmdErr)
	}
}

type raftListMembersAction struct {
	api.Options
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
		})

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
