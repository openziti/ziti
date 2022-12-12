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
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/openziti/ziti/ziti/util"
	"github.com/spf13/cobra"
	"io"
)

// newPolicyAdvisor creates a command object for the "controller policy-advisor" command
func newPolicyAdivsorCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy-advisor",
		Short: "runs sanity checks on various policy related entities managed by the Ziti Edge Controller",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			cmdhelper.CheckErr(err)
		},
	}

	cmd.AddCommand(newPolicyAdvisorIdentitiesCmd(out, errOut))
	cmd.AddCommand(newPolicyAdvisorServicesCmd(out, errOut))

	return cmd
}

type policyAdvisorOptions struct {
	api.Options
	quiet bool
}

// newPolicyAdvisorIdentitiesCmd creates the 'edge controller policy-advisor identities' command
func newPolicyAdvisorIdentitiesCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &policyAdvisorOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "identities <identity name or id>? <service name or id>?",
		Short: "checks policies/connectivity between identities and services",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runIdentitiesPolicyAdvisor(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	cmd.Flags().BoolVarP(&options.quiet, "quiet", "q", false, "Minimize output by hiding header")

	return cmd
}

// newPolicyAdvisorServicesCmd creates the 'edge controller policy-advisor services' command
func newPolicyAdvisorServicesCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	options := &policyAdvisorOptions{
		Options: api.Options{
			CommonOptions: common.CommonOptions{Out: out, Err: errOut},
		},
	}

	cmd := &cobra.Command{
		Use:   "services <service name or id>? <identity name or id>?",
		Short: "checks policies/connectivity between services and identities ",
		Args:  cobra.RangeArgs(0, 2),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runServicesPolicyAdvisor(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&options.quiet, "quiet", "q", false, "Minimize output by hiding header")
	options.AddCommonFlags(cmd)

	return cmd
}

// runIdentitiesPolicyAdvisor create a new policyAdvisor on the Ziti Edge Controller
func runIdentitiesPolicyAdvisor(o *policyAdvisorOptions) error {
	if len(o.Args) > 0 {
		identityId, err := mapNameToID("identities", o.Args[0], o.Options)
		if err != nil {
			return err
		}
		if len(o.Args) > 1 {
			serviceId, err := mapNameToID("services", o.Args[1], o.Options)
			if err != nil {
				return err
			}
			if err := outputHeader(o); err != nil {
				return err
			}
			return runPolicyAdvisorForIdentityAndService(identityId, serviceId, o)
		}

		if err := outputHeader(o); err != nil {
			return err
		}
		return runPolicyAdvisorForIdentity(identityId, o)
	}

	if err := outputHeader(o); err != nil {
		return err
	}
	return runPolicyAdvisorForIdentities(o)
}

// runServicesPolicyAdvisor create a new policyAdvisor on the Ziti Edge Controller
func runServicesPolicyAdvisor(o *policyAdvisorOptions) error {
	if len(o.Args) > 0 {
		serviceId, err := mapNameToID("services", o.Args[0], o.Options)
		if err != nil {
			return err
		}

		if len(o.Args) > 1 {
			identityId, err := mapNameToID("identities", o.Args[1], o.Options)
			if err != nil {
				return err
			}
			if err := outputHeader(o); err != nil {
				return err
			}
			return runPolicyAdvisorForIdentityAndService(identityId, serviceId, o)
		}

		if err := outputHeader(o); err != nil {
			return err
		}
		return runPolicyAdvisorForService(serviceId, o)
	}

	if err := outputHeader(o); err != nil {
		return err
	}
	return runPolicyAdvisorForServices(o)
}

func outputHeader(o *policyAdvisorOptions) error {
	if o.quiet {
		return nil
	}
	_, err := fmt.Fprintf(o.Out, "\n"+
		"Policy General Guidelines\n"+
		"  In order for an identity to dial or bind a service, the following must be true:\n"+
		"    - The identity must have access to the service via a service policy of the correct type (dial or bind)\n"+
		"    - The identity must have acces to at least one on-line edge router via an edge router policy\n"+
		"    - The service must have access to at least one on-line edge router via a service edge router policy\n"+
		"    - There must be at least one on-line edge router that both the identity and service have access to.\n"+
		"\n"+
		"Policy Advisor Output Guide:\n"+
		"  STATUS = The status of the identity -> service reachability. Will be OKAY or ERROR. \n"+
		"  ID = identity name\n"+
		"  ID ROUTERS = number of routers accessible to the identity via edge router policies.\n"+
		"    - See edge router polices for an identity: ziti edge controller list identity edge-router-policies <identity>\n"+
		"  SVC = service name\n"+
		"  SVC ROUTERS = number of routers accessible to the service via service edge router policies.\n"+
		"    - See service edge router policies for a service with: ziti edge controller list service service-edge-router-policies <service>\n"+
		"  ONLINE COMMON ROUTERS = number of routers the identity and service have in common which are online.\n"+
		"  COMMON ROUTERS = number of routers (online or offline) the identity and service have in common.\n"+
		"  DIAL_OK = indicates if the identity has permission to dial the service.\n"+
		"    - See service polices for a service  : ziti edge controller list service service-policies <service>\n"+
		"    - See service polices for an identity: ziti edge controller list identity service-policies <identity>\n"+
		"  BIND_OK = indicates if the identity has permission to bind the service.\n"+
		"  ERROR_LIST = if the status is ERROR, error details will be listed on the following lines\n"+
		"\n"+
		"Output format: STATUS: ID (ID ROUTERS) -> SVC (SVC ROUTERS) Common Routers: (ONLINE COMMON ROUTERS/COMMON ROUTERS) Dial: DIAL_OK Bind: BIND_OK. ERROR_LIST\n"+
		"-------------------------------------------------------------------------------\n")
	return err
}

func runPolicyAdvisorForIdentities(o *policyAdvisorOptions) error {
	skip := 0
	done := false
	for !done {
		filter := fmt.Sprintf(`true skip %v limit 2`, skip)
		children, _, err := filterEntitiesOfType("identities", filter, false, o.Out, o.Options.Timeout, o.Options.Verbose)
		if err != nil {
			return err
		}

		for _, child := range children {
			identityId, _ := child.S("id").Data().(string)
			if err := runPolicyAdvisorForIdentity(identityId, o); err != nil {
				return err
			}
		}
		skip += len(children)
		if len(children) == 0 {
			done = true
		}
	}

	if skip == 0 {
		_, err := fmt.Fprintln(o.Out, "No identities found")
		return err
	}

	return nil
}

func runPolicyAdvisorForServices(o *policyAdvisorOptions) error {
	skip := 0
	done := false
	for !done {
		filter := fmt.Sprintf(`true skip %v limit 2`, skip)
		children, _, err := filterEntitiesOfType("services", filter, false, o.Out, o.Options.Timeout, o.Options.Verbose)
		if err != nil {
			return err
		}

		for _, child := range children {
			serviceId, _ := child.S("id").Data().(string)
			if err := runPolicyAdvisorForService(serviceId, o); err != nil {
				return err
			}
		}
		skip += len(children)
		if len(children) == 0 {
			done = true
		}
	}

	if skip == 0 {
		_, err := fmt.Fprintln(o.Out, "No services found")
		return err
	}

	return nil
}

func runPolicyAdvisorForIdentity(identityId string, o *policyAdvisorOptions) error {
	skip := 0
	done := false
	for !done {
		filter := fmt.Sprintf(`true skip %v limit 2`, skip)
		children, _, err := filterSubEntitiesOfType("identities", "services", identityId, filter, &o.Options)
		if err != nil {
			return err
		}

		for _, child := range children {
			serviceId, _ := child.S("id").Data().(string)
			if err := runPolicyAdvisorForIdentityAndService(identityId, serviceId, o); err != nil {
				return err
			}
		}
		skip += len(children)
		if len(children) == 0 {
			done = true
		}
	}

	if skip == 0 {
		identityName, err := mapIdToName("identities", identityId, o.Options)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(o.Out, "ERROR: %v %v\n\n", identityName, "\n  - Identity does not have access to any services. Adjust service policies.")
		return err
	}

	return nil
}

func runPolicyAdvisorForService(serviceId string, o *policyAdvisorOptions) error {
	skip := 0
	done := false
	for !done {
		filter := fmt.Sprintf(`true skip %v limit 2`, skip)
		children, _, err := filterSubEntitiesOfType("services", "identities", serviceId, filter, &o.Options)
		if err != nil {
			return err
		}

		for _, child := range children {
			identityId, _ := child.S("id").Data().(string)
			if err := runPolicyAdvisorForIdentityAndService(identityId, serviceId, o); err != nil {
				return err
			}
		}
		skip += len(children)
		if len(children) == 0 {
			done = true
		}
	}

	if skip == 0 {
		serviceName, err := mapIdToName("services", serviceId, o.Options)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(o.Out, "ERROR: %v %v\n\n", serviceName, "\n  - Service is not accessible by any identities. Adjust service policies.")
		return err
	}

	return nil
}

func runPolicyAdvisorForIdentityAndService(identityId, serviceId string, o *policyAdvisorOptions) error {
	result, err := util.EdgeControllerList("identities/"+identityId+"/policy-advice/"+serviceId, nil, o.OutputJSONResponse, o.Out, o.Options.Timeout, o.Options.Verbose)
	if err != nil || o.OutputJSONResponse {
		return err
	}

	identityName, _ := result.S("data", "identity", "name").Data().(string)
	serviceName, _ := result.S("data", "service", "name").Data().(string)
	isBindAllowed, _ := result.S("data", "isBindAllowed").Data().(bool)
	isDialAllowed, _ := result.S("data", "isDialAllowed").Data().(bool)
	identityRouterCount, _ := result.S("data", "identityRouterCount").Data().(float64)
	serviceRouterCount, _ := result.S("data", "serviceRouterCount").Data().(float64)

	commonRouters, err := result.S("data", "commonRouters").Children()
	if err != nil && err != gabs.ErrNotObjOrArray {
		return err
	}

	commonCount := len(commonRouters)
	onlineCount := 0
	for _, commonRouter := range commonRouters {
		isOnline := commonRouter.S("isOnline").Data().(bool)
		if isOnline {
			onlineCount++
		}
	}

	status := "OKAY "
	detail := ""
	if !isBindAllowed && !isDialAllowed {
		status = "ERROR"
		detail += "\n  - No access to service. Adjust service policies."
	}

	if identityRouterCount < 1 {
		status = "ERROR"
		detail += "\n  - Identity has no edge routers assigned. Adjust edge router policies."
	}

	if serviceRouterCount < 1 {
		status = "ERROR"
		detail += "\n  - Service has no edge routers assigned. Adjust service edge router policies."
	}

	if identityRouterCount > 0 && serviceRouterCount > 0 {
		if commonCount < 1 {
			status = "ERROR"
			detail += "\n  - Identity and services have no edge routers in common. Adjust edge router policies and/or service edge router policies."
		} else if onlineCount < 1 {
			status = "ERROR"
			detail += "\n  - Common edge routers are all off-line. Bring routers back on-line or adjust edge router policies and/or service edge router policies to increase common router pool."
		}
	}

	dialStatus := "Y"
	if !isDialAllowed {
		dialStatus = "N"
	}

	bindStatus := "Y"
	if !isBindAllowed {
		bindStatus = "N"
	}

	_, err = fmt.Fprintf(o.Out, "%v: %v (%v) -> %v (%v) Common Routers: (%v/%v) Dial: %v Bind: %v %v\n\n",
		status, identityName, identityRouterCount, serviceName, serviceRouterCount, onlineCount, commonCount, dialStatus, bindStatus, detail)

	return err
}
