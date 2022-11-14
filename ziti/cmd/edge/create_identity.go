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
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/helpers"
	"github.com/pkg/errors"
	"io"
	"io/ioutil"
	"math"
	"reflect"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/spf13/cobra"
)

type createIdentityOptions struct {
	api.Options
	isAdmin                  bool
	roleAttributes           []string
	jwtOutputFile            string
	username                 string
	defaultHostingPrecedence string
	defaultHostingCost       uint16
	serviceCosts             map[string]int
	servicePrecedences       map[string]string
	tags                     map[string]string
	appData                  map[string]string
	externalId               string
}

// newCreateIdentityCmd creates the 'edge controller create identity' command
func newCreateIdentityCmd(out io.Writer, errOut io.Writer) *cobra.Command {
	newOptions := func() *createIdentityOptions {
		return &createIdentityOptions{
			Options: api.Options{
				CommonOptions: common.CommonOptions{Out: out, Err: errOut},
			},
		}
	}

	cmd := &cobra.Command{
		Use:   "identity",
		Short: "creates a new identity managed by the Ziti Edge Controller",
		Long:  "creates a new identity managed by the Ziti Edge Controller",
	}

	cmd.AddCommand(newCreateIdentityOfTypeCmd("device", newOptions()))
	cmd.AddCommand(newCreateIdentityOfTypeCmd("user", newOptions()))
	cmd.AddCommand(newCreateIdentityOfTypeCmd("service", newOptions()))

	return cmd
}

func newCreateIdentityOfTypeCmd(idType string, options *createIdentityOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   idType + " <name>",
		Short: "creates a new " + idType + " identity managed by the Ziti Edge Controller",
		Long:  "creates a new " + idType + " identity managed by the Ziti Edge Controller",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runCreateIdentity(idType, options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVarP(&options.isAdmin, "admin", "A", false, "Give the new identity admin privileges")
	cmd.Flags().StringVar(&options.username, "updb", "", "username to give the identity, will create a UPDB enrollment")
	cmd.Flags().StringVar(&options.externalId, "external-id", "", "an external id to give to the identity")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "Role attributes of the new identity")
	cmd.Flags().StringVarP(&options.jwtOutputFile, "jwt-output-file", "o", "", "File to which to output the JWT used for enrolling the identity")
	cmd.Flags().StringVarP(&options.defaultHostingPrecedence, "default-hosting-precedence", "p", "", "Default precedence to use when hosting services using this identity [default,required,failed]")
	cmd.Flags().Uint16VarP(&options.defaultHostingCost, "default-hosting-cost", "c", 0, "Default cost to use when hosting services using this identity")
	cmd.Flags().StringToIntVar(&options.serviceCosts, "service-costs", map[string]int{}, "Per-service hosting costs")
	cmd.Flags().StringToStringVar(&options.servicePrecedences, "service-precedences", map[string]string{}, "Per-service hosting precedences")
	cmd.Flags().StringToStringVar(&options.tags, "tags", nil, "Custom management tags")
	cmd.Flags().StringToStringVar(&options.appData, "app-data", nil, "Custom application data")

	options.AddCommonFlags(cmd)

	return cmd
}

func runCreateIdentity(idType string, o *createIdentityOptions) error {
	entityData := gabs.New()
	api.SetJSONValue(entityData, o.Args[0], "name")
	api.SetJSONValue(entityData, strings.Title(idType), "type")

	o.username = strings.TrimSpace(o.username)
	if o.username != "" {
		api.SetJSONValue(entityData, o.username, "enrollment", "updb")
	} else {
		api.SetJSONValue(entityData, true, "enrollment", "ott")
	}
	api.SetJSONValue(entityData, o.isAdmin, "isAdmin")
	api.SetJSONValue(entityData, o.roleAttributes, "roleAttributes")
	api.SetJSONValue(entityData, o.tags, "tags")
	api.SetJSONValue(entityData, o.appData, "appData")

	if o.externalId != "" {
		api.SetJSONValue(entityData, o.externalId, "externalId")
	}

	if o.defaultHostingPrecedence != "" {
		prec, err := normalizeAndValidatePrecedence(o.defaultHostingPrecedence)
		if err != nil {
			return err
		}

		api.SetJSONValue(entityData, prec, "defaultHostingPrecedence")
	}

	api.SetJSONValue(entityData, o.defaultHostingCost, "defaultHostingCost")

	for k, v := range o.serviceCosts {
		if v < 0 || v > math.MaxUint16 {
			return errors.Errorf("hosting costs must be in the range %v-%v", 0, math.MaxUint16)
		}
		id, err := mapNameToID("services", k, o.Options)
		if err != nil {
			return err
		}
		delete(o.serviceCosts, k)
		o.serviceCosts[id] = v
	}
	api.SetJSONValue(entityData, o.serviceCosts, "serviceHostingCosts")

	for k, v := range o.servicePrecedences {
		id, err := mapNameToID("services", k, o.Options)
		if err != nil {
			return err
		}

		prec, err := normalizeAndValidatePrecedence(v)
		if err != nil {
			return err
		}

		delete(o.servicePrecedences, k)
		o.servicePrecedences[id] = prec
	}
	api.SetJSONValue(entityData, o.servicePrecedences, "serviceHostingPrecedences")

	result, err := CreateEntityOfType("identities", entityData.String(), &o.Options)
	if err := o.LogCreateResult("identity", result, err); err != nil {
		return err
	}

	if o.jwtOutputFile != "" {
		id := result.S("data", "id").Data().(string)
		if err := getIdentityJwt(o, id, o.Options.Timeout, o.Options.Verbose); err != nil {
			return err
		}
	}
	return err
}

func getIdentityJwt(o *createIdentityOptions, id string, timeout int, verbose bool) error {

	newIdentity, err := DetailEntityOfType("identities", id, o.OutputJSONResponse, o.Out, timeout, verbose)
	if err != nil {
		return err
	}

	if newIdentity == nil {
		return fmt.Errorf("no error during identity creation, but identity with id %v not found... unable to extract JWT", id)
	}

	var dataContainer *gabs.Container
	if o.username != "" {
		dataContainer = newIdentity.Path("enrollment.updb.jwt")
	} else {
		dataContainer = newIdentity.Path("enrollment.ott.jwt")
	}

	data := dataContainer.Data()
	jwt, ok := dataContainer.Data().(string)

	if !ok {
		return fmt.Errorf("could not read enrollment.ott.jwt as a string encountered %v", reflect.TypeOf(data))
	}

	if jwt == "" {
		return fmt.Errorf("enrollment JWT not present for new identity")
	}

	if err := ioutil.WriteFile(o.jwtOutputFile, []byte(jwt), 0600); err != nil {
		fmt.Printf("Failed to write JWT to file(%v)\n", o.jwtOutputFile)
		return err
	}

	if container := newIdentity.Path("enrollment.ott.expiresAt"); container != nil && container.Data() != nil {
		jwtExpiration := fmt.Sprintf("%v", container.Data())
		if jwtExpiration != "" {
			fmt.Printf("Enrollment expires at %v\n", jwtExpiration)
		}
	}

	return err
}
