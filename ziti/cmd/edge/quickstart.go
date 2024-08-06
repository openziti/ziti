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

package edge

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/openziti/ziti/common/version"
	edgeSubCmd "github.com/openziti/ziti/controller/subcmd"
	"github.com/openziti/ziti/ziti/cmd/create"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/pki"
	"github.com/openziti/ziti/ziti/constants"
	controller2 "github.com/openziti/ziti/ziti/controller"
	"github.com/openziti/ziti/ziti/router"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type QuickstartOpts struct {
	Username           string
	Password           string
	AlreadyInitialized bool
	Home               string
	ControllerAddress  string
	ControllerPort     int16
	RouterAddress      string
	RouterPort         int16
	out                io.Writer
	errOut             io.Writer
	cleanOnExit        bool
}

// NewQuickStartCmd creates a command object for the "create" command
func NewQuickStartCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
	currentCtrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	currentCtrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	currentRouterAddy := helpers.GetRouterAdvertisedAddress()
	currentRouterPort := helpers.GetZitiEdgeRouterPort()
	defautlCtrlPort, _ := strconv.ParseInt(constants.DefaultCtrlEdgeAdvertisedPort, 10, 16)
	defautlRouterPort, _ := strconv.ParseInt(constants.DefaultZitiEdgeRouterPort, 10, 16)
	options := &QuickstartOpts{}
	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "runs a Controller and Router in quickstart mode",
		Long:  "runs a Controller and Router in quickstart mode with a temporary directory; suitable for testing and development",
		Run: func(cmd *cobra.Command, args []string) {
			options.out = out
			options.errOut = errOut
			options.run(context)
		},
	}
	cmd.Flags().StringVarP(&options.Username, "username", "u", "", "Admin username, default: admin")
	cmd.Flags().StringVarP(&options.Password, "password", "p", "", "Admin password, default: admin")

	cmd.Flags().StringVar(&options.Home, "home", "", "permanent directory")

	cmd.Flags().StringVar(&options.ControllerAddress, "ctrl-address", "", "Sets the advertised address for the control plane and API. current: "+currentCtrlAddy)
	cmd.Flags().Int16Var(&options.ControllerPort, "ctrl-port", int16(defautlCtrlPort), "Sets the port to use for the control plane and API. current: "+currentCtrlPort)
	cmd.Flags().StringVar(&options.RouterAddress, "router-address", "", "Sets the advertised address for the integrated router. current: "+currentRouterAddy)
	cmd.Flags().Int16Var(&options.RouterPort, "router-port", int16(defautlRouterPort), "Sets the port to use for the integrated router. current: "+currentRouterPort)
	return cmd
}

func (o *QuickstartOpts) cleanupHome() {
	if o.cleanOnExit {
		fmt.Println("Removing temp directory at: " + o.Home)
		_ = os.RemoveAll(o.Home)
	} else {
		fmt.Println("Environment left intact at: " + o.Home)
	}
}

func (o *QuickstartOpts) run(ctx context.Context) {
	//set env vars
	if o.Home == "" {
		tmpDir, _ := os.MkdirTemp("", "quickstart")
		o.Home = tmpDir
		o.cleanOnExit = true
	} else {
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

	ctrlYaml := o.Home + "/ctrl.yaml"

	//ZITI_HOME=/tmp ziti create config controller | grep -v "#" | sed -E 's/^ *$//g' | sed '/^$/d'
	_ = os.Setenv("ZITI_HOME", o.Home)
	_ = os.Setenv("ZITI_PKI_CTRL_CA", o.Home+"/pki/root-ca/certs/root-ca.cert")
	_ = os.Setenv("ZITI_PKI_CTRL_KEY", o.Home+"/pki/intermediate-ca/keys/server.key")
	_ = os.Setenv("ZITI_PKI_CTRL_CERT", o.Home+"/pki/intermediate-ca/certs/client.chain.pem")
	_ = os.Setenv("ZITI_PKI_SIGNER_CERT", o.Home+"/pki/intermediate-ca/certs/intermediate-ca.cert")
	_ = os.Setenv("ZITI_PKI_SIGNER_KEY", o.Home+"/pki/intermediate-ca/keys/intermediate-ca.key")
	_ = os.Setenv("ZITI_PKI_CTRL_SERVER_CERT", o.Home+"/pki/intermediate-ca/certs/server.chain.pem")

	routerName := "quickstart-router"
	routerNameFromEnv := os.Getenv(constants.ZitiEdgeRouterNameVarName)
	if routerNameFromEnv != "" {
		routerName = routerNameFromEnv
	}

	dbDir := o.Home + "/db"
	if _, err := os.Stat(dbDir); !os.IsNotExist(err) {
		o.AlreadyInitialized = true
	} else {
		_ = os.MkdirAll(dbDir, 0o777)
		logrus.Debugf("made directory '%s'", dbDir)

		o.createMinimalPki()

		ctrl := create.NewCmdCreateConfigController()
		ctrl.SetArgs([]string{
			fmt.Sprintf("--output=%s", ctrlYaml),
		})
		_ = ctrl.Execute()

		initCmd := edgeSubCmd.NewEdgeInitializeCmd(version.GetCmdBuildInfo())
		initCmd.SetArgs([]string{
			fmt.Sprintf("--username=%s", o.Username),
			fmt.Sprintf("--password=%s", o.Password),
			ctrlYaml,
		})
		initErr := initCmd.Execute()
		if initErr != nil {
			logrus.Fatal(initErr)
		}
	}

	go func() {
		runCtrl := controller2.NewRunCmd()
		runCtrl.SetArgs([]string{
			ctrlYaml,
		})
		runCtrlErr := runCtrl.Execute()
		if runCtrlErr != nil {
			logrus.Fatal(runCtrlErr)
		}
	}()

	fmt.Println("Controller running... Configuring and starting Router...")

	ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
	ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
	ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)

	c := make(chan struct{})
	defer close(c)
	timeout, _ := time.ParseDuration("30s")
	go waitForController(ctrlUrl, c)
	select {
	case <-c:
		//completed normally
		logrus.Info("Controller online. Continuing...")
	case <-time.After(timeout):
		fmt.Println("timed out waiting for controller:", ctrlUrl)
		o.cleanupHome()
		return
	}

	erYaml := o.Home + "/" + routerName + ".yaml"
	if !o.AlreadyInitialized {
		loginCmd := NewLoginCmd(o.out, o.errOut)
		loginCmd.SetArgs([]string{
			ctrlUrl,
			fmt.Sprintf("--username=%s", o.Username),
			fmt.Sprintf("--password=%s", o.Password),
			"-y",
		})
		loginErr := loginCmd.Execute()
		if loginErr != nil {
			logrus.Fatal(loginErr)
		}

		// Allow all identities to use any edge router with the "public" attribute
		// ziti edge create edge-router-policy all-endpoints-public-routers --edge-router-roles "#public" --identity-roles "#all"
		erpCmd := NewCreateEdgeRouterPolicyCmd(o.out, o.errOut)
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
		serpCmd := NewCreateServiceEdgeRouterPolicyCmd(o.out, o.errOut)
		serpCmd.SetArgs([]string{
			"all-routers-all-services",
			fmt.Sprintf("--edge-router-roles=%s", "#all"),
			fmt.Sprintf("--service-roles=%s", "#all"),
		})
		serpCmdErr := serpCmd.Execute()
		if serpCmdErr != nil {
			logrus.Fatal(serpCmdErr)
		}

		// ziti edge create edge-router ${ZITI_HOSTNAME}-edge-router -o ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt -t -a public
		createErCmd := NewCreateEdgeRouterCmd(o.out, o.errOut)
		erJwt := o.Home + "/" + routerName + ".jwt"
		createErCmd.SetArgs([]string{
			routerName,
			fmt.Sprintf("--jwt-output-file=%s", erJwt),
			"--tunneler-enabled",
			fmt.Sprintf("--role-attributes=%s", "public"),
		})
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
			fmt.Sprintf("--output=%s", erYaml),
		})
		erCfgErr := erCfg.Execute()
		if erCfgErr != nil {
			logrus.Fatal(erCfgErr)
		}

		// ziti router enroll ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml --jwt ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt
		erEnroll := router.NewEnrollGwCmd()
		erEnroll.SetArgs([]string{
			erYaml,
			fmt.Sprintf("--jwt=%s", erJwt),
		})
		erEnrollErr := erEnroll.Execute()
		if erEnrollErr != nil {
			logrus.Fatal(erEnrollErr)
		}
	}

	go func() {
		// ziti router run ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml &> ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.log &
		erRunCmd := router.NewRunCmd()
		erRunCmd.SetArgs([]string{
			erYaml,
		})
		erRunCmdErr := erRunCmd.Execute()
		if erRunCmdErr != nil {
			logrus.Fatal(erRunCmdErr)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ch:
		fmt.Println("Signal to shutdown received")
	case <-ctx.Done():
		fmt.Println("Cancellation request received")
	}
	o.cleanupHome()
}

func (o *QuickstartOpts) createMinimalPki() {
	where := o.Home + "/pki"
	fmt.Println("emitting a minimal PKI")

	//ziti pki create ca --pki-root="$pkiDir" --ca-file="root-ca" --ca-name="root-ca"
	ca := pki.NewCmdPKICreateCA(o.out, o.errOut)
	ca.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", "root-ca"),
		fmt.Sprintf("--ca-name=%s", "root-ca"),
	})
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	//ziti pki create intermediate --pki-root "$pkiDir" --ca-name "root-ca" --intermediate-name "intermediate-ca" --intermediate-file "intermediate-ca" --max-path-len "1"
	intermediate := pki.NewCmdPKICreateIntermediate(o.out, o.errOut)
	intermediate.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", "root-ca"),
		fmt.Sprintf("--intermediate-name=%s", "intermediate-ca"),
		fmt.Sprintf("--intermediate-file=%s", "intermediate-ca"),
		"--max-path-len=1",
	})
	intErr := intermediate.Execute()
	if intErr != nil {
		logrus.Fatal(intErr)
	}

	//ziti pki create server --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --server-name "server" --server-file "server" --dns "localhost,${ZITI_HOSTNAME}"
	svr := pki.NewCmdPKICreateServer(o.out, o.errOut)
	var ips = "127.0.0.1,::1"
	ip_override := os.Getenv("ZITI_CTRL_EDGE_IP_OVERRIDE")
	if ip_override != "" {
		ips = ips + "," + ip_override
	}
	svr.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", "intermediate-ca"),
		fmt.Sprintf("--server-name=%s", "server"),
		fmt.Sprintf("--server-file=%s", "server"),
		fmt.Sprintf("--dns=%s,%s", "localhost", helpers.GetCtrlAdvertisedAddress()),
		fmt.Sprintf("--ip=%s", ips),
	})
	svrErr := svr.Execute()
	if svrErr != nil {
		logrus.Fatal(svrErr)
	}

	//ziti pki create client --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --client-name "client" --client-file "client" --key-file "server"
	client := pki.NewCmdPKICreateClient(o.out, o.errOut)
	client.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", "intermediate-ca"),
		fmt.Sprintf("--client-name=%s", "client"),
		fmt.Sprintf("--client-file=%s", "client"),
		fmt.Sprintf("--key-file=%s", "server"),
	})
	clientErr := client.Execute()
	if clientErr != nil {
		logrus.Fatal(clientErr)
	}
}

func waitForController(ctrlUrl string, done chan struct{}) {
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
	done <- struct{}{}
}
