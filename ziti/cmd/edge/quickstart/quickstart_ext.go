// package quickstart
//
// import (
//
//	"context"
//	"crypto/tls"
//	"fmt"
//	"github.com/michaelquigley/pfxlog"
//	"github.com/openziti/ziti/common/version"
//	edgeSubCmd "github.com/openziti/ziti/controller/subcmd"
//	"github.com/openziti/ziti/ziti/cmd/agentcli"
//	"github.com/openziti/ziti/ziti/cmd/common"
//	"github.com/openziti/ziti/ziti/cmd/create"
//	"github.com/openziti/ziti/ziti/cmd/edge"
//	"github.com/openziti/ziti/ziti/cmd/helpers"
//	"github.com/openziti/ziti/ziti/constants"
//	ctrlcmd "github.com/openziti/ziti/ziti/controller"
//	"github.com/sirupsen/logrus"
//	"net"
//	"net/http"
//	"os"
//	"os/signal"
//	"path"
//	"strconv"
//	"strings"
//	"syscall"
//	"time"
//
// )
//
//	type QuickstartTestEnv struct {
//		controllerStarted chan struct{}
//		routerStarted     chan struct{}
//		complete          chan bool
//	}
//
//	func (q QuickstartTestEnv) Start(ctx context.Context, onCancel context.CancelFunc) {
//		_ = os.Setenv("ZITI_CTRL_EDGE_ADVERTISED_ADDRESS", "localhost") //force localhost
//		_ = os.Setenv("ZITI_ROUTER_NAME", "quickstart-router")
//
//		qs := edge.NewQuickStartCmd(os.Stdout, os.Stderr, ctx)
//		go func() {
//			err := qs.Execute()
//			if err != nil {
//				pfxlog.Logger().Fatal(err)
//			}
//			q.complete <- true
//		}()
//
//		ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
//		ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
//		ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)
//
//		go WaitForController(ctrlUrl, q.controllerStarted)
//		timeout, _ := time.ParseDuration("60s")
//		select {
//		case <-q.controllerStarted:
//			//completed normally
//			pfxlog.Logger().Info("controller online")
//		case <-time.After(timeout):
//			onCancel()
//			panic("timed out waiting for controller")
//		}
//	}
//
// func (q QuickstartTestEnv) ShutDown() {
//
// }
//
//	func WaitForController(ctrlUrl string, done chan struct{}) {
//		tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
//		client := &http.Client{Transport: tr}
//		for {
//			r, e := client.Get(ctrlUrl)
//			if e != nil || r == nil || r.StatusCode != 200 {
//				time.Sleep(50 * time.Millisecond)
//			} else {
//				break
//			}
//		}
//		done <- struct{}{}
//	}
//
//	func WaitForRouter(address string, port int16, done chan struct{}) {
//		for {
//			addr := fmt.Sprintf("%s:%d", address, port)
//			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
//			if err == nil {
//				_ = conn.Close()
//				fmt.Printf("Router is available on %s:%d\n", address, port)
//				close(done)
//				return
//			}
//			time.Sleep(25 * time.Millisecond)
//		}
//	}
//
// func Run(ctx context.Context) {
//
//		if o.verbose {
//			pfxlog.GlobalInit(logrus.DebugLevel, pfxlog.DefaultOptions().Color())
//		}
//
//		//set env vars
//		if o.Home == "" {
//			tmpDir, _ := os.MkdirTemp("", "quickstart")
//			o.Home = tmpDir
//			o.cleanOnExit = true
//			logrus.Infof("temporary --home '%s'", o.Home)
//		} else {
//			//normalize path
//			if strings.HasPrefix(o.Home, "~") {
//				usr, err := user.Current()
//				if err != nil {
//					logrus.Fatalf("Could not find user's home directory?")
//					return
//				}
//				home := usr.HomeDir
//				// Replace only the first instance of ~ in case it appears later in the path
//				o.Home = strings.Replace(o.Home, "~", home, 1)
//			}
//			logrus.Infof("permanent --home '%s' will not be removed on exit", o.Home)
//		}
//		if o.ControllerAddress != "" {
//			_ = os.Setenv(constants.CtrlAdvertisedAddressVarName, o.ControllerAddress)
//			_ = os.Setenv(constants.CtrlEdgeAdvertisedAddressVarName, o.ControllerAddress)
//		}
//		if o.ControllerPort > 0 {
//			_ = os.Setenv(constants.CtrlAdvertisedPortVarName, strconv.Itoa(int(o.ControllerPort)))
//			_ = os.Setenv(constants.CtrlEdgeAdvertisedPortVarName, strconv.Itoa(int(o.ControllerPort)))
//		}
//		if o.RouterAddress != "" {
//			_ = os.Setenv(constants.ZitiEdgeRouterAdvertisedAddressVarName, o.RouterAddress)
//		}
//		if o.RouterPort > 0 {
//			_ = os.Setenv(constants.ZitiEdgeRouterPortVarName, strconv.Itoa(int(o.RouterPort)))
//			_ = os.Setenv(constants.ZitiEdgeRouterListenerBindPortVarName, strconv.Itoa(int(o.RouterPort)))
//		}
//		if o.Username == "" {
//			o.Username = "admin"
//		}
//		if o.Password == "" {
//			o.Password = "admin"
//		}
//
//		if o.InstanceID == "" {
//			o.InstanceID = uuid.New().String()
//		}
//
//		ctrlYaml := path.Join(o.instHome(), "ctrl.yaml")
//		routerName := "router-" + o.InstanceID
//
//		//ZITI_HOME=/tmp ziti create config controller | grep -v "#" | sed -E 's/^ *$//g' | sed '/^$/d'
//		_ = os.Setenv("ZITI_HOME", o.Home)
//		pkiLoc := path.Join(o.Home, "pki")
//		rootLoc := path.Join(pkiLoc, "root-ca")
//		pkiIntermediateName := o.scopedName("intermediate-ca")
//		pkiServerName := o.scopedNameOff("server")
//		pkiClientName := o.scopedNameOff("client")
//		intermediateLoc := path.Join(pkiLoc, pkiIntermediateName)
//		_ = os.Setenv("ZITI_PKI_CTRL_CA", path.Join(rootLoc, "certs", "root-ca.cert"))
//		_ = os.Setenv("ZITI_PKI_CTRL_KEY", path.Join(intermediateLoc, "keys", pkiServerName+".key"))
//		_ = os.Setenv("ZITI_PKI_CTRL_SERVER_CERT", path.Join(intermediateLoc, "certs", pkiServerName+".chain.pem"))
//		_ = os.Setenv("ZITI_PKI_CTRL_CERT", path.Join(intermediateLoc, "certs", pkiClientName+".chain.pem"))
//		_ = os.Setenv("ZITI_PKI_SIGNER_CERT", path.Join(intermediateLoc, "certs", pkiIntermediateName+".cert"))
//		_ = os.Setenv("ZITI_PKI_SIGNER_KEY", path.Join(intermediateLoc, "keys", pkiIntermediateName+".key"))
//
//		routerNameFromEnv := os.Getenv(constants.ZitiEdgeRouterNameVarName)
//		if routerNameFromEnv != "" {
//			routerName = routerNameFromEnv
//		}
//
//		dbDir := path.Join(o.instHome(), "db")
//		if _, err := os.Stat(dbDir); !os.IsNotExist(err) {
//			o.AlreadyInitialized = true
//		} else {
//			_ = os.MkdirAll(dbDir, 0o700)
//			logrus.Debugf("made directory '%s'", dbDir)
//
//			o.createMinimalPki()
//
//			_ = os.Setenv("ZITI_HOME", o.instHome())
//			ctrl := create.NewCmdCreateConfigController()
//			args := []string{
//				fmt.Sprintf("--output=%s", ctrlYaml),
//			}
//			if o.isHA {
//				args = append(args, fmt.Sprintf("--minCluster=%d", 1))
//			}
//			ctrl.SetArgs(args)
//			err = ctrl.Execute()
//			if err != nil {
//				logrus.Fatal(err)
//			}
//
//			if !o.isHA {
//				initCmd := edgeSubCmd.NewEdgeInitializeCmd(version.GetCmdBuildInfo())
//				initCmd.SetArgs([]string{
//					fmt.Sprintf("--username=%s", o.Username),
//					fmt.Sprintf("--password=%s", o.Password),
//					ctrlYaml,
//				})
//				initErr := initCmd.Execute()
//				if initErr != nil {
//					logrus.Fatal(initErr)
//				}
//			}
//		}
//
//		fmt.Println("Starting controller...")
//		go func() {
//			runCtrl := ctrlcmd.NewRunCmd()
//			runCtrl.SetArgs([]string{
//				ctrlYaml,
//			})
//			runCtrlErr := runCtrl.Execute()
//			if runCtrlErr != nil {
//				logrus.Fatal(runCtrlErr)
//			}
//		}()
//		fmt.Println("Controller running...")
//
//		ctrlAddy := helpers.GetCtrlEdgeAdvertisedAddress()
//		ctrlPort := helpers.GetCtrlEdgeAdvertisedPort()
//		ctrlUrl := fmt.Sprintf("https://%s:%s", ctrlAddy, ctrlPort)
//
//		c := make(chan struct{})
//		timeout, _ := time.ParseDuration("30s")
//		go quickstart.WaitForController(ctrlUrl, c)
//		select {
//		case <-c:
//			//completed normally
//			logrus.Info("Controller online. Continuing...")
//		case <-time.After(timeout):
//			fmt.Println("timed out waiting for controller:", ctrlUrl)
//			o.cleanupHome()
//			return
//		}
//
//		if o.isHA {
//			p := common.NewOptionsProvider(o.out, o.errOut)
//			if !o.joinCommand {
//				fmt.Println("waiting three seconds for controller to become ready...")
//				time.Sleep(3 * time.Second)
//				agentInitCmd := agentcli.NewAgentCtrlInit(p)
//				pid := os.Getpid()
//				args := []string{
//					o.Username,
//					o.Password,
//					o.Username,
//					fmt.Sprintf("--pid=%d", pid),
//				}
//				agentInitCmd.SetArgs(args)
//
//				agentInitErr := agentInitCmd.Execute()
//				if agentInitErr != nil {
//					logrus.Fatal(agentInitErr)
//				}
//			} else {
//				agentJoinCmd := agentcli.NewAgentClusterAdd(p)
//
//				args := []string{
//					o.ClusterMember,
//					fmt.Sprintf("--pid=%d", os.Getpid()),
//					fmt.Sprintf("--voter=%t", !o.nonVoter),
//				}
//				agentJoinCmd.SetArgs(args)
//
//				fmt.Println("waiting three seconds for controller to become ready...")
//				time.Sleep(3 * time.Second)
//
//				addChan := make(chan struct{})
//				addTimeout := time.Second * 30
//				go func() {
//					o.waitForLeader()
//					agentJoinErr := agentJoinCmd.Execute()
//					if agentJoinErr != nil {
//						logrus.Fatal(agentJoinErr)
//					}
//					close(addChan)
//				}()
//
//				select {
//				case <-addChan:
//					//completed normally
//					logrus.Info("Add command successful. continuing...")
//				case <-time.After(addTimeout):
//					fmt.Println("timed out adding to cluster")
//					o.cleanupHome()
//					return
//				}
//			}
//		}
//
//		erConfigFile := path.Join(o.instHome(), routerName+".yaml")
//		o.configureRouter(routerName, erConfigFile, ctrlUrl)
//		o.runRouter(erConfigFile)
//
//		ch := make(chan os.Signal, 1)
//		signal.Notify(ch, os.Interrupt, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)
//
//		if !o.routerless {
//			r := make(chan struct{})
//			timeout, _ = time.ParseDuration("30s")
//			go waitForRouter(o.RouterAddress, o.RouterPort, r)
//			select {
//			case <-r:
//				//completed normally
//			case <-time.After(timeout):
//				fmt.Println("timed out waiting for router:", ctrlUrl)
//				o.cleanupHome()
//				return
//			}
//		}
//
//		if o.isHA {
//			go func() {
//				time.Sleep(3 * time.Second) // output this after a bit...
//				nextInstId := incrementStringSuffix(o.InstanceID)
//				fmt.Println()
//				o.printDetails()
//				fmt.Println("=======================================================================================")
//				fmt.Println("Quickly add another member to this cluster using: ")
//				fmt.Printf("  ziti edge quickstart join \\\n")
//				fmt.Printf("    --ctrl-port %d \\\n", o.ControllerPort+1)
//				fmt.Printf("    --router-port %d \\\n", o.RouterPort+1)
//				fmt.Printf("    --home \"%s\" \\\n", o.Home)
//				fmt.Printf("    --trust-domain=\"%s\" \\\n", o.TrustDomain)
//				fmt.Printf("    --cluster-member tls:%s:%s\\ \n", ctrlAddy, ctrlPort)
//				fmt.Printf("    --instance-id \"%s\"\n", nextInstId)
//				fmt.Println("=======================================================================================")
//				fmt.Println()
//			}()
//		} else {
//			fmt.Println()
//			o.printDetails()
//			fmt.Println("=======================================================================================")
//		}
//
//		select {
//		case <-ch:
//			fmt.Println("Signal to shutdown received")
//		case <-ctx.Done():
//			fmt.Println("Cancellation request received")
//		}
//		o.cleanupHome()
//	}
package quickstart
