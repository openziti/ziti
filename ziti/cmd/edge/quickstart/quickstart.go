// /*
//
//	Copyright NetFoundry Inc.
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.
//
// */
//
// package edge
//
// import (
//
//	"context"
//	"fmt"
//	testing "github.com/openziti/ziti/ziti/cmd/edge/quickstart"
//	"io"
//	"os"
//	"os/signal"
//	"os/user"
//	"path"
//	"regexp"
//	"strconv"
//	"strings"
//	"syscall"
//	"time"
//
//	"github.com/google/uuid"
//	"github.com/michaelquigley/pfxlog"
//	"github.com/openziti/ziti/common/version"
//	"github.com/openziti/ziti/controller/rest_client/raft"
//	edgeSubCmd "github.com/openziti/ziti/controller/subcmd"
//	"github.com/openziti/ziti/ziti/cmd/agentcli"
//	"github.com/openziti/ziti/ziti/cmd/api"
//	"github.com/openziti/ziti/ziti/cmd/common"
//	"github.com/openziti/ziti/ziti/cmd/create"
//	"github.com/openziti/ziti/ziti/cmd/helpers"
//	"github.com/openziti/ziti/ziti/cmd/pki"
//	"github.com/openziti/ziti/ziti/constants"
//	ctrlcmd "github.com/openziti/ziti/ziti/controller"
//	"github.com/openziti/ziti/ziti/router"
//	"github.com/openziti/ziti/ziti/util"
//	"github.com/sirupsen/logrus"
//	"github.com/spf13/cobra"
//
// )
//
//	type QuickstartOpts struct {
//		Username           string
//		Password           string
//		AlreadyInitialized bool
//		Home               string
//		ControllerAddress  string
//		ControllerPort     int16
//		RouterAddress      string
//		RouterPort         int16
//		out                io.Writer
//		errOut             io.Writer
//		cleanOnExit        bool
//		TrustDomain        string
//		isHA               bool
//		InstanceID         string
//		ClusterMember      string
//		joinCommand        bool
//		verbose            bool
//		nonVoter           bool
//		routerless         bool
//	}
//
//	func addCommonQuickstartFlags(cmd *cobra.Command, options *QuickstartOpts) {
//		currentCtrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
//		currentCtrlPort := helpers.GetCtrlEdgeAdvertisedPort()
//		currentRouterAddy := helpers.GetRouterAdvertisedAddress()
//		currentRouterPort := helpers.GetZitiEdgeRouterPort()
//		defaultCtrlPort, _ := strconv.ParseInt(constants.DefaultCtrlEdgeAdvertisedPort, 10, 16)
//		defaultRouterPort, _ := strconv.ParseInt(constants.DefaultZitiEdgeRouterPort, 10, 16)
//
//		cmd.Flags().StringVarP(&options.Username, "username", "u", "", "admin username, default: admin")
//		cmd.Flags().StringVarP(&options.Password, "password", "p", "", "admin password, default: admin")
//
//		cmd.Flags().StringVar(&options.Home, "home", "", "permanent directory")
//
//		cmd.Flags().StringVar(&options.ControllerAddress, "ctrl-address", "", "sets the advertised address for the control plane and API. current: "+currentCtrlAddy)
//		cmd.Flags().Int16Var(&options.ControllerPort, "ctrl-port", int16(defaultCtrlPort), "sets the port to use for the control plane and API. current: "+currentCtrlPort)
//		cmd.Flags().StringVar(&options.RouterAddress, "router-address", "", "sets the advertised address for the integrated router. current: "+currentRouterAddy)
//		cmd.Flags().Int16Var(&options.RouterPort, "router-port", int16(defaultRouterPort), "sets the port to use for the integrated router. current: "+currentRouterPort)
//		cmd.Flags().BoolVar(&options.routerless, "no-router", false, "specifies the quickstart should not start a router")
//
//		cmd.Flags().BoolVar(&options.verbose, "verbose", false, "Show additional output.")
//	}
//
//	func addQuickstartHaFlags(cmd *cobra.Command, options *QuickstartOpts) {
//		cmd.Flags().StringVar(&options.TrustDomain, "trust-domain", "", "the specified trust domain to be used in SPIFFE ids.")
//		cmd.Flags().StringVar(&options.InstanceID, "instance-id", "", "specifies a unique instance id for use in ha mode.")
//	}
//
// // NewQuickStartCmd creates a command object for the "create" command
//
//	func NewQuickStartCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
//		options := &QuickstartOpts{}
//		cmd := &cobra.Command{
//			Use:   "quickstart",
//			Short: "runs a Controller and Router in quickstart mode",
//			Long:  "runs a Controller and Router in quickstart mode with a temporary directory; suitable for testing and development",
//			Run: func(cmd *cobra.Command, args []string) {
//				options.out = out
//				options.errOut = errOut
//				options.TrustDomain = "quickstart"
//				options.InstanceID = "quickstart"
//				options.run(context)
//			},
//		}
//		addCommonQuickstartFlags(cmd, options)
//		cmd.AddCommand(NewQuickStartJoinClusterCmd(out, errOut, context))
//		cmd.AddCommand(NewQuickStartHaCmd(out, errOut, context))
//		return cmd
//	}
//
//	func NewQuickStartHaCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
//		options := &QuickstartOpts{}
//		cmd := &cobra.Command{
//			Use:   "ha",
//			Short: "runs a Controller and Router in quickstart HA mode and creates the first cluster member",
//			Long:  "runs a Controller and Router in quickstart HA mode and creates the first cluster member with a temporary directory; suitable for testing and development",
//			Run: func(cmd *cobra.Command, args []string) {
//				options.out = out
//				options.errOut = errOut
//				options.isHA = true
//				if options.TrustDomain == "" {
//					options.TrustDomain = uuid.New().String()
//					fmt.Println("Trust domain was not supplied. Using a random trust domain: " + options.TrustDomain)
//				}
//				options.run(context)
//			},
//		}
//		addCommonQuickstartFlags(cmd, options)
//		addQuickstartHaFlags(cmd, options)
//		cmd.Hidden = true
//		return cmd
//	}
//
//	func NewQuickStartJoinClusterCmd(out io.Writer, errOut io.Writer, context context.Context) *cobra.Command {
//		options := &QuickstartOpts{}
//		cmd := &cobra.Command{
//			Use:   "join",
//			Short: "runs a Controller and Router in quickstart mode and joins an existing cluster",
//			Long:  "runs a Controller and Router in quickstart mode and joins an existing cluster with a temporary directory; suitable for testing and development",
//			Run: func(cmd *cobra.Command, args []string) {
//				options.out = out
//				options.errOut = errOut
//				options.join(context)
//			},
//		}
//		addCommonQuickstartFlags(cmd, options)
//		addQuickstartHaFlags(cmd, options)
//		cmd.Flags().StringVarP(&options.ClusterMember, "cluster-member", "m", "", "address of a cluster member. required. example tls:localhost:1280")
//		cmd.Flags().BoolVar(&options.nonVoter, "non-voting", false, "used with ha mode. specifies the member is a non-voting member")
//		cmd.Hidden = true
//		return cmd
//	}
//
//	func (o *QuickstartOpts) cleanupHome() {
//		if o.cleanOnExit {
//			fmt.Println("Removing temp directory at: " + o.Home)
//			_ = os.RemoveAll(o.Home)
//		} else {
//			fmt.Println("Environment left intact at: " + o.Home)
//		}
//	}
//
//	func (o *QuickstartOpts) join(ctx context.Context) {
//		if strings.TrimSpace(o.InstanceID) == "" {
//			logrus.Fatalf("the instance-id is required when joining a cluster")
//		}
//		if strings.TrimSpace(o.Home) == "" {
//			logrus.Fatalf("the home directory must be specified when joining an existing cluster. the root-ca is used to create the server's pki")
//		}
//
//		if o.ClusterMember == "" {
//			logrus.Fatalf("--cluster-member is required")
//		}
//
//		o.isHA = true
//		o.joinCommand = true
//		o.run(ctx)
//	}
//
//	func (o *QuickstartOpts) run(ctx context.Context) {
//		quickstart.Run(ctx)
//	}
//
//	func (o *QuickstartOpts) printDetails() {
//		fmt.Println("=======================================================================================")
//		fmt.Println("controller and router started.")
//		fmt.Println("    controller located at  : " + helpers.GetCtrlAdvertisedAddress() + ":" + strconv.Itoa(int(o.ControllerPort)))
//		fmt.Println("    router located at      : " + helpers.GetRouterAdvertisedAddress() + ":" + strconv.Itoa(int(o.RouterPort)))
//		fmt.Println("    config dir located at  : " + o.Home)
//		fmt.Println("    configured trust domain: " + o.TrustDomain)
//		fmt.Printf("    instance pid           : %d\n", os.Getpid())
//	}
//
//	func (o *QuickstartOpts) configureRouter(routerName string, configFile string, ctrlUrl string) {
//		if o.routerless {
//			return
//		}
//
//		if !o.AlreadyInitialized {
//			loginCmd := NewLoginCmd(o.out, o.errOut)
//			loginCmd.SetArgs([]string{
//				ctrlUrl,
//				fmt.Sprintf("--username=%s", o.Username),
//				fmt.Sprintf("--password=%s", o.Password),
//				"-y",
//			})
//			if o.joinCommand {
//				o.waitForLeader()
//			}
//			loginErr := loginCmd.Execute()
//			if loginErr != nil {
//				logrus.Fatal(loginErr)
//			}
//
//			o.configureOverlay()
//
//			time.Sleep(1 * time.Second)
//
//			var erJwt string
//
//			// ziti edge create edge-router ${ZITI_HOSTNAME}-edge-router -o ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt -t -a public
//			createErCmd := NewCreateEdgeRouterCmd(o.out, o.errOut)
//			erJwt = path.Join(o.Home, routerName+".jwt")
//			createErCmd.SetArgs([]string{
//				routerName,
//				fmt.Sprintf("--jwt-output-file=%s", erJwt),
//				"--tunneler-enabled",
//				fmt.Sprintf("--role-attributes=%s", "public"),
//			})
//
//			o.waitForLeader() //wait for a leader before doing anything
//			createErErr := createErCmd.Execute()
//
//			if createErErr != nil {
//				logrus.Fatal(createErErr)
//			}
//
//			// ziti create config router edge --routerName ${ZITI_HOSTNAME}-edge-router >${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml
//			opts := &create.CreateConfigRouterOptions{}
//
//			data := &create.ConfigTemplateValues{}
//			data.PopulateConfigValues()
//			opts.IsHA = o.isHA
//			create.SetZitiRouterIdentity(&data.Router, routerName)
//			erCfg := create.NewCmdCreateConfigRouterEdge(opts, data)
//			erCfg.SetArgs([]string{
//				fmt.Sprintf("--routerName=%s", routerName),
//				fmt.Sprintf("--output=%s", configFile),
//			})
//
//			o.waitForLeader()
//			erCfgErr := erCfg.Execute()
//			if erCfgErr != nil {
//				logrus.Fatal(erCfgErr)
//			}
//
//			// ziti router enroll ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml --jwt ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt
//			erEnroll := router.NewEnrollGwCmd()
//			erEnroll.SetArgs([]string{
//				configFile,
//				fmt.Sprintf("--jwt=%s", erJwt),
//			})
//
//			o.waitForLeader() //needed?
//			erEnrollErr := erEnroll.Execute()
//			if erEnrollErr != nil {
//				logrus.Fatal(erEnrollErr)
//			}
//		}
//	}
//
//	func (o *QuickstartOpts) runRouter(configFile string) {
//		if o.routerless {
//			return
//		}
//
//		go func() {
//			// ziti router run ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml &> ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.log &
//			erRunCmd := router.NewRunCmd()
//			erRunCmd.SetArgs([]string{
//				configFile,
//			})
//
//			o.waitForLeader() //needed?
//			erRunCmdErr := erRunCmd.Execute()
//			if erRunCmdErr != nil {
//				logrus.Fatal(erRunCmdErr)
//			}
//		}()
//	}
//
//	func (o *QuickstartOpts) createMinimalPki() {
//		where := path.Join(o.Home, "pki")
//		fmt.Println("emitting a minimal PKI")
//
//		sid := fmt.Sprintf("spiffe://%s/controller/%s", o.TrustDomain, o.InstanceID)
//
//		//ziti pki create ca --pki-root="$pkiDir" --ca-file="root-ca" --ca-name="root-ca" --spiffe-id="whatever"
//		if o.joinCommand {
//			// indicates we are joining a cluster. don't emit a root-ca, expect it'll be there or error
//		} else {
//			rootCaPath := path.Join(where, "root-ca", "certs", "root-ca.cert")
//			rootCa, statErr := os.Stat(rootCaPath)
//			if statErr != nil {
//				if !os.IsNotExist(statErr) {
//					logrus.Warnf("could not check for root-ca: %v", statErr)
//				}
//			}
//			if rootCa == nil {
//				ca := pki.NewCmdPKICreateCA(o.out, o.errOut)
//				rootCaArgs := []string{
//					fmt.Sprintf("--pki-root=%s", where),
//					fmt.Sprintf("--ca-file=%s", "root-ca"),
//					fmt.Sprintf("--ca-name=%s", "root-ca"),
//					fmt.Sprintf("--trust-domain=%s", o.TrustDomain),
//				}
//
//				ca.SetArgs(rootCaArgs)
//				pkiErr := ca.Execute()
//				if pkiErr != nil {
//					logrus.Fatal(pkiErr)
//				}
//
//			} else {
//				logrus.Infof("%s exists and will be reused", rootCaPath)
//			}
//		}
//
//		//ziti pki create intermediate --pki-root "$pkiDir" --ca-name "root-ca" --intermediate-name "intermediate-ca" --intermediate-file "intermediate-ca" --max-path-len "1" --spiffe-id="whatever"
//		intermediate := pki.NewCmdPKICreateIntermediate(o.out, o.errOut)
//		intermediate.SetArgs([]string{
//			fmt.Sprintf("--pki-root=%s", where),
//			fmt.Sprintf("--ca-name=%s", "root-ca"),
//			fmt.Sprintf("--intermediate-name=%s", o.scopedName("intermediate-ca")),
//			fmt.Sprintf("--intermediate-file=%s", o.scopedName("intermediate-ca")),
//			"--max-path-len=1",
//		})
//		intErr := intermediate.Execute()
//		if intErr != nil {
//			logrus.Fatal(intErr)
//		}
//
//		//ziti pki create server --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --server-name "server" --server-file "server" --dns "localhost,${ZITI_HOSTNAME}" --spiffe-id="whatever"
//		svr := pki.NewCmdPKICreateServer(o.out, o.errOut)
//		var ips = "127.0.0.1,::1"
//		ipOverride := os.Getenv("ZITI_CTRL_EDGE_IP_OVERRIDE")
//		if ipOverride != "" {
//			ips = ips + "," + ipOverride
//		}
//		args := []string{
//			fmt.Sprintf("--pki-root=%s", where),
//			fmt.Sprintf("--ca-name=%s", o.scopedName("intermediate-ca")),
//			fmt.Sprintf("--server-name=%s", o.InstanceID),
//			fmt.Sprintf("--server-file=%s", o.scopedNameOff("server")),
//			fmt.Sprintf("--dns=%s,%s", "localhost", helpers.GetCtrlAdvertisedAddress()),
//			fmt.Sprintf("--ip=%s", ips),
//			fmt.Sprintf("--spiffe-id=%s", sid),
//		}
//
//		svr.SetArgs(args)
//		svrErr := svr.Execute()
//		if svrErr != nil {
//			logrus.Fatal(svrErr)
//		}
//
//		//ziti pki create client --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --client-name "client" --client-file "client" --key-file "server" --spiffe-id="whatever"
//		client := pki.NewCmdPKICreateClient(o.out, o.errOut)
//		client.SetArgs([]string{
//			fmt.Sprintf("--pki-root=%s", where),
//			fmt.Sprintf("--ca-name=%s", o.scopedName("intermediate-ca")),
//			fmt.Sprintf("--client-name=%s", o.InstanceID),
//			fmt.Sprintf("--client-file=%s", o.scopedNameOff("client")),
//			fmt.Sprintf("--key-file=%s", o.scopedNameOff("server")),
//			fmt.Sprintf("--spiffe-id=%s", sid),
//		})
//		clientErr := client.Execute()
//		if clientErr != nil {
//			logrus.Fatal(clientErr)
//		}
//	}
//
//	func (o *QuickstartOpts) scopedNameOff(name string) string {
//		return name
//	}
//
//	func (o *QuickstartOpts) scopedName(name string) string {
//		if o.InstanceID != "" {
//			return name + "-" + o.InstanceID
//		} else {
//			return name
//		}
//	}
//
//	func (o *QuickstartOpts) instHome() string {
//		if o.isHA {
//			return path.Join(o.Home, o.InstanceID)
//		}
//		return o.Home
//	}
//
//	func (o *QuickstartOpts) configureOverlay() {
//		if o.joinCommand {
//			return
//		}
//
//		// Allow all identities to use any edge router with the "public" attribute
//		// ziti edge create edge-router-policy all-endpoints-public-routers --edge-router-roles "#public" --identity-roles "#all"
//		erpCmd := NewCreateEdgeRouterPolicyCmd(o.out, o.errOut)
//		erpCmd.SetArgs([]string{
//			"all-endpoints-public-routers",
//			fmt.Sprintf("--edge-router-roles=%s", "#public"),
//			fmt.Sprintf("--identity-roles=%s", "#all"),
//		})
//		erpCmdErr := erpCmd.Execute()
//		if erpCmdErr != nil {
//			logrus.Fatal(erpCmdErr)
//		}
//
//		// # Allow all edge-routers to access all services
//		// ziti edge create service-edge-router-policy all-routers-all-services --edge-router-roles "#all" --service-roles "#all"
//		serpCmd := NewCreateServiceEdgeRouterPolicyCmd(o.out, o.errOut)
//		serpCmd.SetArgs([]string{
//			"all-routers-all-services",
//			fmt.Sprintf("--edge-router-roles=%s", "#all"),
//			fmt.Sprintf("--service-roles=%s", "#all"),
//		})
//		o.waitForLeader()
//		serpCmdErr := serpCmd.Execute()
//		if serpCmdErr != nil {
//			logrus.Fatal(serpCmdErr)
//		}
//	}
//
//	type raftListMembersAction struct {
//		api.Options
//	}
//
//	func (o *QuickstartOpts) waitForLeader() bool {
//		for {
//			p := common.NewOptionsProvider(o.out, o.errOut)
//			action := &raftListMembersAction{
//				Options: api.Options{CommonOptions: p()},
//			}
//
//			client, err := util.NewFabricManagementClient(action)
//			if err != nil {
//				return false
//			}
//			members, err := client.Raft.RaftListMembers(&raft.RaftListMembersParams{
//				Context: context.Background(),
//			})
//
//			if err != nil {
//				return false
//			}
//			for _, m := range members.Payload.Data {
//				if m.Leader != nil && *m.Leader {
//					time.Sleep(500 * time.Millisecond) // this just gives time for the leader to 'settle' -- shouldn't be necessary
//					return true
//				}
//			}
//			time.Sleep(50 * time.Millisecond)
//		}
//	}
//
//	func incrementStringSuffix(input string) string {
//		// Regular expression to capture the numeric suffix
//		re := regexp.MustCompile(`(\d+)$`)
//		match := re.FindStringSubmatch(input)
//
//		if len(match) == 0 {
//			return uuid.New().String()
//		}
//
//		numStr := match[1]
//		numLength := len(numStr)
//		num, _ := strconv.Atoi(numStr)
//		num++
//
//		incremented := fmt.Sprintf("%0*d", numLength, num)
//
//		return strings.TrimSuffix(input, numStr) + incremented
//	}
package quickstart
