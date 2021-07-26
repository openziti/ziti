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
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"io"

	"github.com/pkg/errors"

	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	"github.com/spf13/cobra"
)

// newUpdatePostureCheckCmd creates the 'edge controller update posture-check' command
func newUpdatePostureCheckCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "posture-check",
	}

	cmd.AddCommand(newUpdatePostureCheckMacCmd(f, out, errOut))
	cmd.AddCommand(newUpdatePostureCheckDomainCmd(f, out, errOut))
	cmd.AddCommand(newUpdatePostureCheckProcessCmd(f, out, errOut))
	cmd.AddCommand(newUpdatePostureCheckOsCmd(f, out, errOut))
	cmd.AddCommand(newUpdatePostureCheckMfaCmd(f, out, errOut))

	return cmd
}

type updatePostureCheckOptions struct {
	edgeOptions
	name           string
	tags           map[string]string
	roleAttributes []string
}

type updatePostureCheckMacOptions struct {
	updatePostureCheckOptions
	addresses []string
}

type updatePostureCheckDomainOptions struct {
	updatePostureCheckOptions
	domains []string
}

type updatePostureCheckProcessOptions struct {
	updatePostureCheckOptions
	hashes []string
	signer string
	os     string
	path   string
}

type updatePostureCheckMfaOptions struct {
	updatePostureCheckOptions
	timeoutSeconds        int
	promptOnWake          bool
	promptOnUnlock        bool
	ignoreLegacyEndpoints bool
}

type updatePostureCheckOsOptions struct {
	updatePostureCheckOptions
	os []string
}

func newUpdatePostureCheckMacCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updatePostureCheckMacOptions{
		updatePostureCheckOptions: updatePostureCheckOptions{
			edgeOptions: edgeOptions{
				CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
			},
		},
		addresses: nil,
	}

	cmd := &cobra.Command{
		Use:   "mac <idOrName>",
		Short: "updates a MAC posture check",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdatePostureCheckMac(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"Set role attributes of the posture check. Use --role-attributes '' to set an empty list")
	cmd.Flags().StringSliceVarP(&options.addresses, "mac-addresses", "m", nil,
		"Set MAC addresses of the posture check")
	return cmd
}

// runUpdatePostureCheckMac update a new identity on the Ziti Edge Controller
func runUpdatePostureCheckMac(o *updatePostureCheckMacOptions) error {
	id, err := mapNameToID("posture-checks", o.Args[0], o.edgeOptions)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		setJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		setJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.Cmd.Flags().Changed("mac-addresses") {
		setJSONValue(entityData, o.addresses, "macAddresses")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	setJSONValue(entityData, PostureCheckTypeMAC, "typeId")

	_, err = patchEntityOfType(fmt.Sprintf("posture-checks/%v", id), entityData.String(), &o.edgeOptions)
	return err
}

func newUpdatePostureCheckDomainCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updatePostureCheckDomainOptions{
		updatePostureCheckOptions: updatePostureCheckOptions{
			edgeOptions: edgeOptions{
				CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
			},
		},
		domains: nil,
	}

	cmd := &cobra.Command{
		Use:   "domain <idOrName>",
		Short: "updates a domain posture check",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdatePostureCheckDomain(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"Set role attributes of the posture check. Use --role-attributes '' to set an empty list")
	cmd.Flags().StringSliceVarP(&options.domains, "domains", "d", nil,
		"Set the domains of the posture check")
	return cmd
}

func newUpdatePostureCheckMfaCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updatePostureCheckMfaOptions{
		updatePostureCheckOptions: updatePostureCheckOptions{
			edgeOptions: edgeOptions{
				CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
			},
		},
		timeoutSeconds:        -1,
		promptOnWake:          false,
		promptOnUnlock:        false,
		ignoreLegacyEndpoints: false,
	}

	cmd := &cobra.Command{
		Use:   "mfa <idOrName>",
		Short: "updates an MFA posture check",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdatePostureCheckMfa(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name ")
	cmd.Flags().IntVarP(&options.timeoutSeconds, "seconds", "s", -1, "Seconds an MFA posture check allows before an additional MFA code must be submitted")

	cmd.Flags().BoolVarP(&options.promptOnWake, "wake", "w", false, "Prompt for MFA code on endpoint wake")
	cmd.Flags().BoolVarP(&options.promptOnWake, "no-wake", "z", false, "Do not prompt for MFA code on endpoint wake")

	cmd.Flags().BoolVarP(&options.promptOnUnlock, "unlock", "u", false, "Prompt for MFA code on endpoint unlock")
	cmd.Flags().BoolVarP(&options.promptOnUnlock, "no-unlock", "q", false, "Do not prompt for MFA code on endpoint unlock")

	cmd.Flags().BoolVarP(&options.ignoreLegacyEndpoints, "ignore-legacy", "i", false, "Ignore prompts and timeout for endpoints that do not support MFA timeout/prompts")
	cmd.Flags().BoolVarP(&options.ignoreLegacyEndpoints, "no-ignore-legacy", "l", false, "Do not ignore prompts and timeout for endpoints that do not support MFA timeout/prompts")

	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "Set role attributes of the posture check. Use --role-attributes '' to set an empty list")
	return cmd
}

func runUpdatePostureCheckMfa(o *updatePostureCheckMfaOptions) error {
	id, err := mapNameToID("posture-checks", o.Args[0], o.edgeOptions)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		setJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("seconds") {
		setJSONValue(entityData, o.timeoutSeconds, "timeoutSeconds")
		change = true
	}

	if o.Cmd.Flags().Changed("wake") {
		setJSONValue(entityData, o.promptOnWake, "promptOnWake")
		change = true
	}

	if o.Cmd.Flags().Changed("no-wake") {
		setJSONValue(entityData, false, "promptOnWake")
		change = true
	}

	if o.Cmd.Flags().Changed("unlock") {
		setJSONValue(entityData, o.promptOnUnlock, "promptOnUnlock")
		change = true
	}

	if o.Cmd.Flags().Changed("no-unlock") {
		setJSONValue(entityData, false, "promptOnUnlock")
		change = true
	}

	if o.Cmd.Flags().Changed("ignore-legacy") {
		setJSONValue(entityData, o.ignoreLegacyEndpoints, "ignoreLegacyEndpoints")
		change = true
	}

	if o.Cmd.Flags().Changed("no-ignore-legacy") {
		setJSONValue(entityData, false, "ignoreLegacyEndpoints")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		setJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	setJSONValue(entityData, PostureCheckTypeMFA, "typeId")

	_, err = patchEntityOfType(fmt.Sprintf("posture-checks/%v", id), entityData.String(), &o.edgeOptions)
	return err
}

// runUpdatePostureCheckDomain update a new identity on the Ziti Edge Controller
func runUpdatePostureCheckDomain(o *updatePostureCheckDomainOptions) error {
	id, err := mapNameToID("posture-checks", o.Args[0], o.edgeOptions)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		setJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		setJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.Cmd.Flags().Changed("domains") {
		if len(o.domains) == 0 {
			return fmt.Errorf("must specify at least one domain, multiple values may be separated by commas")
		}

		setJSONValue(entityData, o.domains, "domains")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	setJSONValue(entityData, PostureCheckTypeDomain, "typeId")

	_, err = patchEntityOfType(fmt.Sprintf("posture-checks/%v", id), entityData.String(), &o.edgeOptions)
	return err
}

//process

func newUpdatePostureCheckProcessCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updatePostureCheckProcessOptions{
		updatePostureCheckOptions: updatePostureCheckOptions{
			edgeOptions: edgeOptions{
				CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
			},
		},
		hashes: nil,
		signer: "",
		os:     "",
		path:   "",
	}

	cmd := &cobra.Command{
		Use:   "process <idOrName>",
		Short: "updates a process posture check",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdatePostureCheckProcess(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"Set role attributes of the posture check. Use --role-attributes '' to set an empty list")
	cmd.Flags().StringVarP(&options.path, "path", "p", "", "set the path of the posture check")
	cmd.Flags().StringSliceVarP(&options.hashes, "hash-sigs", "s", nil, "set the valid hashes of the posture check")
	cmd.Flags().StringVarP(&options.signer, "signer-fingerprint", "f", "", "set the signer fingerprint of the posture check")
	cmd.Flags().StringVarP(&options.os, "os", "o", "", "set the OS of the posture check")
	return cmd
}

// runUpdatePostureCheckProcess update a new identity on the Ziti Edge Controller
func runUpdatePostureCheckProcess(o *updatePostureCheckProcessOptions) error {
	id, err := mapNameToID("posture-checks", o.Args[0], o.edgeOptions)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		setJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		setJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.Cmd.Flags().Changed("path") {
		setJSONValue(entityData, o.path, "process", "path")
		change = true
	}

	if o.Cmd.Flags().Changed("hash-sigs") {
		o.hashes, err = cleanSha512s(o.hashes)

		if err != nil {
			return err
		}

		setJSONValue(entityData, o.hashes, "process", "hashes")
		change = true
	}

	if o.Cmd.Flags().Changed("signer-fingerprint") {
		o.signer, err = cleanSha1(o.signer)

		if err != nil {
			return err
		}

		setJSONValue(entityData, o.signer, "process", "signerFingerprint")
		change = true
	}

	if o.Cmd.Flags().Changed("os") {
		os := normalizeOsType(o.os)

		if os == "" || os == OsAndroid || os == OsIOS {
			return fmt.Errorf("invalid os type [%s]: expected %s|%s|%s|%s", o.os, OsLinux, OsMacOs, OsWindows, OsWindowsServer)
		}

		setJSONValue(entityData, o.os, "process", "osType")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	setJSONValue(entityData, PostureCheckTypeProcess, "typeId")

	_, err = patchEntityOfType(fmt.Sprintf("posture-checks/%v", id), entityData.String(), &o.edgeOptions)
	return err
}

// os
func newUpdatePostureCheckOsCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &updatePostureCheckOsOptions{
		updatePostureCheckOptions: updatePostureCheckOptions{
			edgeOptions: edgeOptions{
				CommonOptions: common.CommonOptions{Factory: f, Out: out, Err: errOut},
			},
		},
		os: nil,
	}

	cmd := &cobra.Command{
		Use:   "os <idOrName>",
		Short: "updates a OS posture check",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args
			err := runUpdatePostureCheckOs(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	options.AddCommonFlags(cmd)
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringVarP(&options.name, "name", "n", "", "Set the name")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil,
		"Set role attributes of the posture check. Use --role-attributes '' to set an empty list")
	cmd.Flags().StringSliceVarP(&options.os, "os", "o", nil,
		"Set OS(es) of the posture check, should be in the format of '<os>:<version>:<version>:...', multiple may be specified via CSV or multiple flags")
	return cmd
}

// runUpdatePostureCheckOs update a new identity on the Ziti Edge Controller
func runUpdatePostureCheckOs(o *updatePostureCheckOsOptions) error {
	id, err := mapNameToID("posture-checks", o.Args[0], o.edgeOptions)
	if err != nil {
		return err
	}
	entityData := gabs.New()
	change := false

	if o.Cmd.Flags().Changed("name") {
		setJSONValue(entityData, o.name, "name")
		change = true
	}

	if o.Cmd.Flags().Changed("role-attributes") {
		setJSONValue(entityData, o.roleAttributes, "roleAttributes")
		change = true
	}

	if o.Cmd.Flags().Changed("os") {
		var oses []*osSpec
		for i, osStr := range o.os {
			if osSpec, err := parseOsSpec(osStr); err != nil {
				return fmt.Errorf("could not prase os at index [%d] got [%s]: %v", i, osStr, err)
			} else {
				oses = append(oses, osSpec)
			}
		}
		setJSONValue(entityData, oses, "operatingSystems")
		change = true
	}

	if !change {
		return errors.New("no change specified. must specify at least one attribute to change")
	}

	setJSONValue(entityData, PostureCheckTypeOS, "typeId")

	_, err = patchEntityOfType(fmt.Sprintf("posture-checks/%v", id), entityData.String(), &o.edgeOptions)
	return err
}
