/*
	Copyright NetFoundry, Inc.

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

package edge_controller

import (
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
)

type createIdentityOptions struct {
	commonOptions
	isAdmin        bool
	roleAttributes []string
	jwtOutputFile  string
}

// newCreateIdentityCmd creates the 'edge controller create identity' command
func newCreateIdentityCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	newOptions := func() *createIdentityOptions {
		return &createIdentityOptions{
			commonOptions: commonOptions{
				CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
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
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "Role attributes of the new identity")
	cmd.Flags().StringVarP(&options.jwtOutputFile, "jwt-output-file", "o", "", "File to which to output the JWT used for enrolling the identity")
	options.AddCommonFlags(cmd)

	return cmd
}

func runCreateIdentity(idType string, o *createIdentityOptions) error {
	entityData := gabs.New()
	setJSONValue(entityData, o.Args[0], "name")
	setJSONValue(entityData, strings.Title(idType), "type")
	setJSONValue(entityData, true, "enrollment", "ott")
	setJSONValue(entityData, o.isAdmin, "isAdmin")
	setJSONValue(entityData, o.roleAttributes, "roleAttributes")

	result, err := createEntityOfType("identities", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	id := result.S("data", "id").Data().(string)

	if _, err = fmt.Fprintf(o.Out, "%v\n", id); err != nil {
		panic(err)
	}

	if o.jwtOutputFile != "" {
		if err := getIdentityJwt(o, id); err != nil {
			return err
		}
	}
	return err
}

func getIdentityJwt(o *createIdentityOptions, id string) error {

	newIdentity, err := DetailEntityOfType("identities", id, o.OutputJSONResponse, o.Out)
	if err != nil {
		return err
	}

	if newIdentity == nil {
		return fmt.Errorf("no error during identity creation, but identity with id %v not found... unable to extract JWT", id)
	}

	dataContainer := newIdentity.Path("enrollment.ott.jwt")
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
