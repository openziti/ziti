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
	"github.com/openziti/ziti/controller/db"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	optionInterceptAddress = "interceptAddress"
	hostCfgSuffix          = ".host.v1"
	interceptCfgSuffix     = ".intercept.v1"
	dialPolSuffix          = ".dial"
	bindPolSuffix          = ".bind"
	interceptAddressSuffix = ".ziti"
	dialRoleSuffix         = ".clients"
	bindRoleSuffix         = ".servers"
)

// SecureOptions the options for the secure command
type SecureOptions struct {
	common.CommonOptions
	api.EntityOptions

	InterceptAddress string
}

// newSecureCmd consolidates network configuration steps for securing a service.
func newSecureCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &SecureOptions{
		EntityOptions: api.NewEntityOptions(out, errOut),
		CommonOptions: common.CommonOptions{
			Out: out,
			Err: errOut,
		},
	}

	cmd := &cobra.Command{
		Use:   "secure <service_name> <protocol>:<address>:<port>",
		Short: "creates a service, configs, and policies for a resource",
		Long: fmt.Sprintf("creates\n"+
			"1. A host config with the name <servicename>%s \n"+
			"2. An intercept config with the name <servicename>%s\n"+
			"   - A default intercept address of <servicename>%s or customize with the --interceptAddress flag\n"+
			"3. A service with the provided name\n"+
			"4. A bind service policy with the name <servicename>%s\n"+
			"   - A role attribute of <servicename>%s is applied\n"+
			"5. A dial service policy with the name <servicename>%s\n"+
			"   - A role attribute of <servicename>%s is applied\n\n"+
			"Once successfully executed, apply the role attributes to your client and server identities.", hostCfgSuffix, interceptCfgSuffix, interceptAddressSuffix, bindPolSuffix, bindRoleSuffix, dialPolSuffix, dialRoleSuffix),
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runSecure(options)
			cmdhelper.CheckErr(err)
		},
	}

	cmd.Flags().StringVar(&options.InterceptAddress, optionInterceptAddress, "", "the custom intercept address for your service")
	options.CommonOptions.AddCommonFlags(cmd)

	return cmd
}

// runSecure implements the command to secure a resource
func runSecure(o *SecureOptions) (err error) {
	// Ensure there are at least two arguments (service name and the url details)
	if len(o.Args) < 2 {
		logrus.Fatal("Insufficient arguments: Service name and, at minimum, port are required")
	}

	svcName := o.Args[0]
	input := o.Args[1]

	// Parse the url argument
	protocol, address, port, err := parseInput(input)
	if err != nil {
		logrus.Fatal("Error:", err)
	}

	// Create a host config
	hostCfgName := svcName + hostCfgSuffix
	jsonStr := fmt.Sprintf(`{"forwardProtocol":true, "forwardPort":true, "allowedProtocols":["%s"], "address":"%s", "allowedPortRanges":[{"low":%d, "high":%d}]}`, protocol, address, port, port)

	cmd := newCreateConfigCmd(os.Stdout, os.Stderr)
	args := []string{hostCfgName, entities.HostConfigV1, jsonStr}
	cmd.SetArgs(args)

	// Run the command
	err = cmd.Execute()
	if err != nil {
		logrus.Fatal("Error:", err)
	}

	// Create a intercept config
	interceptAddress := svcName + interceptAddressSuffix
	if o.InterceptAddress != "" {
		interceptAddress = o.InterceptAddress
	}
	interceptCfgName := svcName + interceptCfgSuffix
	jsonStr = fmt.Sprintf(`{"protocols":["%s"], "addresses":["%s"], "portRanges":[{"low":%d, "high":%d}]}`, protocol, interceptAddress, port, port)

	cmd = newCreateConfigCmd(os.Stdout, os.Stderr)
	args = []string{interceptCfgName, entities.InterceptV1, jsonStr}
	cmd.SetArgs(args)

	// Run the command
	err = cmd.Execute()
	if err != nil {
		logrus.Fatal("Error:", err)
	}

	// Create service
	cmd = newCreateServiceCmd(os.Stdout, os.Stderr)
	args = []string{svcName, "--configs", hostCfgName + "," + interceptCfgName}
	cmd.SetArgs(args)

	// Run the command
	err = cmd.Execute()
	if err != nil {
		logrus.Fatal("Error:", err)
	}

	// Create service policies
	svcRole := "@" + svcName

	dialSvcPolName := svcName + dialPolSuffix
	dialIdRole := "#" + svcName + dialRoleSuffix
	cmd = newCreateServicePolicyCmd(os.Stdout, os.Stderr)
	args = []string{dialSvcPolName, db.PolicyTypeDialName, "--service-roles", svcRole, "--identity-roles", dialIdRole}
	cmd.SetArgs(args)

	// Run the command
	err = cmd.Execute()
	if err != nil {
		logrus.Fatal("Error:", err)
	}

	bindSvcPolName := svcName + bindPolSuffix
	bindIdRole := "#" + svcName + bindRoleSuffix
	cmd = newCreateServicePolicyCmd(os.Stdout, os.Stderr)
	args = []string{bindSvcPolName, db.PolicyTypeBindName, "--service-roles", svcRole, "--identity-roles", bindIdRole}
	cmd.SetArgs(args)

	// Run the command
	err = cmd.Execute()
	if err != nil {
		logrus.Fatal("Error:", err)
	}

	// Print summary of created entities
	_, err = fmt.Fprintf(os.Stdout, "\nSecure command completed successfully.\n"+
		" - Created a new host config: %s\n"+
		" - Created a new intercept config: %s\n"+
		"   - The intercept address is '%s:%d'\n"+
		" - Created a new service: %s\n"+
		" - Created a new bind service-policy: %s\n"+
		"   - The role attribute for the bind policy is '%s'\n"+
		" - Created a new dial service-policy: %s\n"+
		"   - The role attribute for the dial policy is '%s'\n",
		hostCfgName, interceptCfgName, interceptAddress, port, svcName, bindSvcPolName, bindIdRole, dialSvcPolName, dialIdRole)

	if err != nil {
		logrus.Fatalf("Failed to write `secure` summary: %v", err)
	}

	return
}

func parseInput(input string) (string, string, int, error) {
	parts := strings.Split(input, ":")

	// Check if there is at least one part (the port should be provided)
	if len(parts) < 1 {
		return "", "", 0, fmt.Errorf("could not find a port provided in input (%s)", input)
	}

	// Initialize the default values (port is always provided)
	defaultProtocol := "tcp\", \"udp"
	protocol := defaultProtocol
	address := "127.0.0.1"
	portStr := parts[len(parts)-1]

	// Check if the input contains two parts (address or protocol, and port)
	if len(parts) == 2 {
		// Check if the last part is a valid port
		if _, err := strconv.Atoi(portStr); err != nil {
			return "", "", 0, fmt.Errorf("port was not detected in input (%s)", portStr)
		}
		// Check if the first part is the protocol. If not, assume it's the address
		if parts[0] == "tcp" || parts[0] == "udp" {
			protocol = parts[0]
		} else {
			address = parts[0]
		}
	}

	// Check if the input contains three parts (protocol, address, port)
	if len(parts) == 3 {
		protocol = parts[0]
		address = parts[1]
	}

	// Validate protocol
	if protocol != "tcp" && protocol != "udp" && protocol != defaultProtocol {
		return "", "", 0, fmt.Errorf("invalid protocol detected, tcp and udp are supported")
	}

	// Parse the port
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", "", 0, fmt.Errorf("port was not detected in input (%s)", portStr)
	}

	return protocol, address, port, nil
}
