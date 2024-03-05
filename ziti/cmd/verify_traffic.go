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

package cmd

import (
	_ "embed"
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/openziti/ziti/ziti/cmd/edge"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/cmd/templates"
	"github.com/openziti/ziti/ziti/cmd/testutil"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

var (
	verifyTrafficLong = templates.LongDesc(`
		Sends traffic over a ziti network to test functionality
`)

	// TODO: Finish this
	verifyTrafficExample = templates.Examples(`
		ziti verifytraffic...
	`)
)

const (
	optionIdentityName = "identity"
	flagErrorMsg       = "An error occurred setting flags for verifytraffic"
)

// VerifyTrafficOptions the options for the verifytraffic command
type VerifyTrafficOptions struct {
	common.CommonOptions
	edge.LoginOptions
	RouterName   string
	IdentityFile string
}

// NewVerifyTrafficCmd creates a command object
func NewVerifyTrafficCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &VerifyTrafficOptions{}
	cmd := &cobra.Command{
		Use:     "verifytraffic -i <identity-file-path>",
		Short:   "verify basic network functionality",
		Aliases: []string{"vn"},
		Long:    verifyTrafficLong,
		Example: verifyTrafficExample,
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := run(options)
			cmdhelper.CheckErr(err)
		},
	}

	cmd.Flags().StringVarP(&options.IdentityFile, optionIdentityName, "", "", "the path to the identity file used to verify traffic")
	options.CommonOptions.AddCommonFlags(cmd)
	err := cmd.MarkFlagRequired(optionIdentityName)
	if err != nil {
		logrus.Fatal(flagErrorMsg)
	}

	return cmd
}

// run implements the command
func run(o *VerifyTrafficOptions) error {

	protocol := "tcp"
	address := "localhost"
	port := 8080

	// Start up a basic http server
	go httpServer(fmt.Sprintf("%s:%d", address, port))

	params := fmt.Sprintf("%s:%s:%d", protocol, address, port)
	cmd := edge.NewSecureCmd(o.CommonOptions.Out, o.CommonOptions.Err)
	serviceName := testutil.GenerateRandomName("verifytraffic")
	endpoint := testutil.GenerateRandomName("verifytrafficaddress")

	cmd.SetArgs([]string{
		serviceName,
		params,
		fmt.Sprintf("--interceptAddress=%s", endpoint),
	})

	// Run Secure command
	err := cmd.Execute()
	if err != nil {
		logrus.Fatal(err)
	}

	// Modify the attributes of the identity and hosting router

	helloUrl := fmt.Sprintf("https://%s:%d", serviceName, port)
	log.Infof("created url: %s", helloUrl)
	wd, _ := os.Getwd()
	httpClient := testutil.CreateZitifiedHttpClient(wd + "/" + o.IdentityFile)

	timeoutSeconds := 10
	startTime := time.Now()
	success := false
	var resp *http.Response
	var e error
	// Loop until success or the timeout is reached
	for time.Since(startTime).Seconds() < float64(timeoutSeconds) {
		fmt.Println("Waiting...")
		resp, e = httpClient.Get(helloUrl)
		if e == nil && resp.StatusCode == 200 {
			success = true
			break
		}
		time.Sleep(time.Millisecond * 100)
	}

	if success {
		body, _ := io.ReadAll(resp.Body)
		fmt.Println(string(body))
	} else {
		fmt.Println(e)
	}

	return nil
}

func serve(listener net.Listener, serverType string) {
	if err := http.Serve(listener, Greeter(serverType)); err != nil {
		panic(err)
	}
}

func httpServer(listenAddr string) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		panic(err)
	}
	fmt.Printf("listening for non-ziti requests on %v\n", listenAddr)
	serve(listener, "plain-internet")
}

type Greeter string

func (g Greeter) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	var result string
	if name := req.URL.Query().Get("name"); name != "" {
		result = fmt.Sprintf("Hello, %v, from %v\n", name, g)
		fmt.Printf("Saying hello to %v, coming in from %v\n", name, g)
	} else {
		result = "Who are you?\n"
		fmt.Println("Asking for introduction")
	}
	if _, err := resp.Write([]byte(result)); err != nil {
		panic(err)
	}
}
