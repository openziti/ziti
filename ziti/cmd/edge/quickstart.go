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
	"fmt"
	edgeSubCmd "github.com/openziti/edge/controller/subcmd"
	"github.com/openziti/ziti/common/version"
	"github.com/openziti/ziti/ziti/cmd/create"
	"github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/pki"
	controller2 "github.com/openziti/ziti/ziti/controller"
	"github.com/openziti/ziti/ziti/router"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// newCreateCmd creates a command object for the "create" command
func newQuickStartCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "runs a Controller and Router in quickstart mode",
		Long:  "runs a Controller and Router in quickstart mode. A totally ephemeral network only valid while running.",
		Run: func(cmd *cobra.Command, args []string) {
			run(out, errOut)
		},
	}

	return cmd
}

func removeTempDir(tmpDir string) {
	fmt.Println("Removing temp directory at: " + tmpDir)
	_ = os.RemoveAll(tmpDir)
}

func run(out io.Writer, errOut io.Writer) {
	tmpDir, _ := os.MkdirTemp("", "quickstart")

	defer removeTempDir(tmpDir)

	ctrlYaml := tmpDir + "/ctrl.yaml"

	//ZITI_HOME=/tmp ziti create config controller | grep -v "#" | sed -E 's/^ *$//g' | sed '/^$/d'
	_ = os.Setenv("ZITI_HOME", tmpDir)
	_ = os.Setenv("ZITI_PKI_CTRL_CA", tmpDir+"/pki/root-ca/certs/root-ca.cert")
	_ = os.Setenv("ZITI_PKI_CTRL_KEY", tmpDir+"/pki/intermediate-ca/keys/server.key")
	_ = os.Setenv("ZITI_PKI_CTRL_CERT", tmpDir+"/pki/intermediate-ca/certs/client.cert")
	_ = os.Setenv("ZITI_PKI_SIGNER_CERT", tmpDir+"/pki/intermediate-ca/certs/intermediate-ca.cert")
	_ = os.Setenv("ZITI_PKI_SIGNER_KEY", tmpDir+"/pki/intermediate-ca/keys/intermediate-ca.key")
	_ = os.Setenv("ZITI_PKI_CTRL_SERVER_CERT", tmpDir+"/pki/intermediate-ca/certs/server.chain.pem")

	dbDir := tmpDir + "/db"
	_, _ = fmt.Fprintf(os.Stdout, "creating the tmp dir [%v] for the database.\n\n", dbDir)
	_ = os.MkdirAll(dbDir, 0o777)

	createMinimalPki(out, errOut)

	ctrl := create.NewCmdCreateConfigController()
	ctrl.SetArgs([]string{
		fmt.Sprintf("--output=%s", ctrlYaml),
	})
	_ = ctrl.Execute()

	initCmd := edgeSubCmd.NewEdgeInitializeCmd(version.GetCmdBuildInfo())
	initCmd.SetArgs([]string{
		fmt.Sprintf("--username=%s", "admin"),
		fmt.Sprintf("--password=%s", "admin"),
		fmt.Sprintf(ctrlYaml),
	})
	initErr := initCmd.Execute()
	if initErr != nil {
		logrus.Fatal(initErr)
	}

	go func() {
		runCtrl := controller2.NewRunCmd()
		runCtrl.SetArgs([]string{
			fmt.Sprintf(ctrlYaml),
		})
		runCtrlErr := runCtrl.Execute()
		if runCtrlErr != nil {
			logrus.Fatal(runCtrlErr)
		}
	}()

	fmt.Println("Controller running... Configuring and starting Router...")
	time.Sleep(5 * time.Second)

	//ziti edge login https://localhost:1280 -u admin -p admin -y
	loginCmd := NewLoginCmd(out, errOut)
	loginCmd.SetArgs([]string{
		fmt.Sprintf("localhost:1280"),
		fmt.Sprintf("--username=%s", "admin"),
		fmt.Sprintf("--password=%s", "admin"),
		fmt.Sprintf("-y"),
	})
	loginErr := loginCmd.Execute()
	if loginErr != nil {
		logrus.Fatal(loginErr)
	}

	//./ziti edge create edge-router ${ZITI_HOSTNAME}-edge-router -o ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt -t -a public
	createErCmd := NewCreateEdgeRouterCmd(out, errOut)
	erJwt := tmpDir + "/quickstart.er.jwt"
	createErCmd.SetArgs([]string{
		fmt.Sprintf("quickstart-router"),
		fmt.Sprintf("--jwt-output-file=%s", erJwt),
		fmt.Sprintf("--tunneler-enabled"),
		fmt.Sprintf("--role-attributes=%s", "public"),
	})
	createErErr := createErCmd.Execute()
	if createErErr != nil {
		logrus.Fatal(createErErr)
	}

	//./ziti create config router edge --routerName ${ZITI_HOSTNAME}-edge-router >${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml
	opts := &create.CreateConfigRouterOptions{}

	data := &create.ConfigTemplateValues{}
	data.PopulateConfigValues()
	create.SetZitiRouterIdentity(&data.Router, "quickstart-router")
	erCfg := create.NewCmdCreateConfigRouterEdge(opts, data)
	erYaml := tmpDir + "/quickstart.er.yaml"
	erCfg.SetArgs([]string{
		fmt.Sprintf("--routerName=%s", "quickstart-router"),
		fmt.Sprintf("--output=%s", erYaml),
	})
	_ = ctrl.Execute()
	erCfgErr := erCfg.Execute()
	if erCfgErr != nil {
		logrus.Fatal(erCfgErr)
	}

	//./ziti router enroll ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml --jwt ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.jwt
	erEnroll := router.NewEnrollGwCmd()
	erEnroll.SetArgs([]string{
		fmt.Sprintf(erYaml),
		fmt.Sprintf("--jwt=%s", erJwt),
	})
	erEnrollErr := erEnroll.Execute()
	if erEnrollErr != nil {
		logrus.Fatal(erEnrollErr)
	}

	go func() {
		//./ziti router run ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.yaml &> ${ZITI_HOME}/${ZITI_HOSTNAME}-edge-router.log &
		erRunCmd := router.NewRunCmd()
		erRunCmd.SetArgs([]string{
			fmt.Sprintf(erYaml),
		})
		erRunCmdErr := erRunCmd.Execute()
		if erRunCmdErr != nil {
			logrus.Fatal(erRunCmdErr)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)

	<-ch
	removeTempDir(tmpDir)
}

func createMinimalPki(out io.Writer, errOut io.Writer) {
	tmpDir := os.Getenv("ZITI_HOME")
	pkiDir := tmpDir + "/pki"
	fmt.Println("emitting a minimal PKI:")

	//ziti pki create ca --pki-root="$pkiDir" --ca-file="root-ca" --ca-name="root-ca"
	ca := pki.NewCmdPKICreateCA(out, errOut)
	ca.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", pkiDir),
		fmt.Sprintf("--ca-file=%s", "root-ca"),
		fmt.Sprintf("--ca-name=%s", "root-ca"),
	})
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	//ziti pki create intermediate --pki-root "$pkiDir" --ca-name "root-ca" --intermediate-name "intermediate-ca" --intermediate-file "intermediate-ca" --max-path-len "1"
	intermediate := pki.NewCmdPKICreateIntermediate(out, errOut)
	intermediate.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", pkiDir),
		fmt.Sprintf("--ca-name=%s", "root-ca"),
		fmt.Sprintf("--intermediate-name=%s", "intermediate-ca"),
		fmt.Sprintf("--intermediate-file=%s", "intermediate-ca"),
		fmt.Sprintf("--max-path-len=1"),
	})
	intErr := intermediate.Execute()
	if intErr != nil {
		logrus.Fatal(intErr)
	}

	//ziti pki create server --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --server-name "server" --server-file "server" --dns "localhost,${ZITI_HOSTNAME}"
	svr := pki.NewCmdPKICreateServer(out, errOut)
	svr.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", pkiDir),
		fmt.Sprintf("--ca-name=%s", "intermediate-ca"),
		fmt.Sprintf("--server-name=%s", "server"),
		fmt.Sprintf("--server-file=%s", "server"),
		fmt.Sprintf("--dns=%s,%s", "localhost", helpers.GetCtrlAdvertisedAddress()),
	})
	svrErr := svr.Execute()
	if svrErr != nil {
		logrus.Fatal(svrErr)
	}

	//ziti pki create client --pki-root="${ZITI_HOME}/pki" --ca-name "intermediate-ca" --client-name "client" --client-file "client" --key-file "server"
	client := pki.NewCmdPKICreateClient(out, errOut)
	client.SetArgs([]string{
		fmt.Sprintf("--pki-root=%s", pkiDir),
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
