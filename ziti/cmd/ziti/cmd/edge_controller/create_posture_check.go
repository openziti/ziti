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
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/common"
	cmdutil "github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	cmdhelper "github.com/openziti/ziti/ziti/cmd/ziti/cmd/helpers"
	"github.com/spf13/cobra"
	"io"
	"regexp"
	"strings"
)

// newCreatePostureCheckCmd creates the 'edge controller create posture-check' command
func newCreatePostureCheckCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "posture-check",
	}

	cmd.AddCommand(newCreatePostureCheckMacCmd(f, out, errOut))
	cmd.AddCommand(newCreatePostureCheckDomainCmd(f, out, errOut))
	cmd.AddCommand(newCreatePostureCheckProcessCmd(f, out, errOut))
	cmd.AddCommand(newCreatePostureCheckOsCmd(f, out, errOut))

	return cmd
}

type createPostureCheckOptions struct {
	commonOptions
	name           string
	tags           map[string]string
	roleAttributes []string
}

type createPostureCheckMacOptions struct {
	createPostureCheckOptions
	addresses []string
}

type createPostureCheckDomainOptions struct {
	createPostureCheckOptions
	domains []string
}

type createPostureCheckOsOptions struct {
	createPostureCheckOptions
	os []string
}

type createPostureCheckProcessOptions struct {
	createPostureCheckOptions
	hash         []string
	signer       string
	normalizedOs string
	path         string
}

func (options *createPostureCheckOptions) addPostureFlags(cmd *cobra.Command) {
	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().StringToStringVarP(&options.tags, "tags", "t", nil, "Add tags to service definition")
	cmd.Flags().StringSliceVarP(&options.roleAttributes, "role-attributes", "a", nil, "Role attributes of the new service")
}

func newCreatePostureCheckMacCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createPostureCheckMacOptions{
		createPostureCheckOptions: createPostureCheckOptions{
			commonOptions: commonOptions{
				CommonOptions: common.CommonOptions{
					Factory: f,
					Out:     out,
					Err:     errOut,
				},
			},
			tags: make(map[string]string),
		},
		addresses: nil,
	}

	cmd := &cobra.Command{
		Use:   "mac <name>",
		Short: "creates a posture check for MAC addresses",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreatePostureCheckMac(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	options.createPostureCheckOptions.addPostureFlags(cmd)

	cmd.Flags().StringSliceVarP(&options.addresses, "mac-addresses", "m", nil,
		"Set MAC addresses of the posture check")

	return cmd
}

func newCreatePostureCheckDomainCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createPostureCheckDomainOptions{
		createPostureCheckOptions: createPostureCheckOptions{
			commonOptions: commonOptions{
				CommonOptions: common.CommonOptions{
					Factory: f,
					Out:     out,
					Err:     errOut,
				},
			},
			tags: make(map[string]string),
		},
		domains: nil,
	}

	cmd := &cobra.Command{
		Use:   "domain <name>",
		Short: "creates a posture check for Windows domains",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreatePostureCheckDomain(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	options.createPostureCheckOptions.addPostureFlags(cmd)

	cmd.Flags().StringSliceVarP(&options.domains, "domains", "d", nil,
		"Set the domains of the posture check, may be CSV or multiple flags")

	return cmd
}

func newCreatePostureCheckProcessCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {
	options := &createPostureCheckProcessOptions{
		createPostureCheckOptions: createPostureCheckOptions{
			commonOptions: commonOptions{
				CommonOptions: common.CommonOptions{
					Factory: f,
					Out:     out,
					Err:     errOut,
				},
			},
			tags: make(map[string]string),
		},
		hash:   nil,
		signer: "",
	}

	cmd := &cobra.Command{
		Use:   fmt.Sprintf("process <name> <os=%s|%s|%s|%s> <absolutePath>", OsLinux, OsMacOs, OsWindows, OsWindowsServer),
		Short: "creates a posture check for an OS specific process",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(3)(cmd, args); err != nil {
				return err
			}
			os := normalizeOsType(args[1])
			if os == "" || os == OsAndroid || os == OsIOS {
				return fmt.Errorf("invalid os type [%s]: expected %s|%s|%s|%s", args[1], OsLinux, OsMacOs, OsWindows, OsWindowsServer)
			}

			options.normalizedOs = os
			options.path = args[2]

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreatePostureCheckProcess(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	options.createPostureCheckOptions.addPostureFlags(cmd)

	cmd.Flags().StringSliceVarP(&options.hash, "hash-sig", "s", nil, "One or more sha512 hashes separated by commas of valid binaries")
	cmd.Flags().StringVarP(&options.signer, "signer-fingerprint", "f", "", "The sha1 hash of a signer certificate")

	return cmd
}

const (
	OsAndroid       = "Android"
	OsWindows       = "Windows"
	OsWindowsServer = "WindowsServer"
	OsMacOs         = "macOS"
	OsIOS           = "iOS"
	OsLinux         = "Linux"

	PostureCheckTypeDomain  = "DOMAIN"
	PostureCheckTypeProcess = "PROCESS"
	PostureCheckTypeMAC     = "MAC"
	PostureCheckTypeOS      = "OS"
)

// Returns the normalized Edge API value or empty string
func normalizeOsType(os string) string {
	os = strings.ToLower(os)
	switch os {
	case "android":
		return OsAndroid
	case "windows":
		return OsWindows
	case "macos":
		return OsMacOs
	case "ios":
		return OsIOS
	case "linux":
		return OsLinux
	case "windowsserver":
		return OsWindowsServer
	}

	return ""
}

func cleanHexString(in string) string {
	hexClean := regexp.MustCompile("[^a-f0-9]+")
	return hexClean.ReplaceAllString(strings.ToLower(in), "")
}

func cleanSha512(in []string) ([]string, error) {

	hashMap := map[string]struct{}{}
	for _, hash := range in {
		cleanHash := cleanHexString(hash)

		if len(cleanHash) < 64 {
			return nil, fmt.Errorf("sha512 hash must be 64 hex characters, given [%s], cleaned [%s] with a length of %d", hash, cleanHash, len(cleanHash))
		}

		hashMap[cleanHash] = struct{}{}
	}

	var hashes []string
	for hash := range hashMap {
		hashes = append(hashes, hash)
	}

	return hashes, nil
}

func cleanSha1(in string) (string, error) {
	cleanSig := cleanHexString(in)

	if len(cleanSig) < 20 {
		return "", fmt.Errorf("sha1 hash must be 20 hex characters, given [%s], cleaned [%s] with a length of %d", in, cleanSig, len(cleanSig))
	}

	return cleanSig, nil
}

func runCreatePostureCheckProcess(o *createPostureCheckProcessOptions) error {
	entityData := gabs.New()
	setPostureCheckEntityValues(entityData, &o.createPostureCheckOptions, PostureCheckTypeProcess)

	setJSONValue(entityData, o.normalizedOs, "process", "osType")
	setJSONValue(entityData, o.path, "process", "path")

	hashes, err := cleanSha512(o.hash)

	if err != nil {
		return err
	}

	if len(hashes) > 0 {
		setJSONValue(entityData, hashes, "process", "hashes")
	}

	if o.signer != "" {
		cleanSigner, err := cleanSha1(o.signer)

		if err != nil {
			return err
		}

		setJSONValue(entityData, cleanSigner, "process", "signerFingerprint")
	}

	result, err := createEntityOfType("posture-checks", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	checkId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", checkId); err != nil {
		panic(err)
	}

	return err
}

func setPostureCheckEntityValues(entity *gabs.Container, options *createPostureCheckOptions, typeId string) {
	setJSONValue(entity, options.Args[0], "name")
	setJSONValue(entity, options.roleAttributes, "roleAttributes")
	setJSONValue(entity, options.tags, "tags")
	setJSONValue(entity, typeId, "typeId")
}

func cleanMacAddresses(inAddresses []string) ([]string, error) {
	macClean := regexp.MustCompile("[^a-f0-9]+")

	addressMap := map[string]struct{}{}
	for _, address := range inAddresses {
		cleanAddress := macClean.ReplaceAllString(strings.ToLower(address), "")

		if len(cleanAddress) < 12 || len(cleanAddress) > 17 {
			return nil, fmt.Errorf("mac address must be 12-17 hex characters, given [%s], cleaned [%s] with a length of %d", address, cleanAddress, len(cleanAddress))
		}

		addressMap[cleanAddress] = struct{}{}
	}

	var addresses []string
	for address := range addressMap {
		addresses = append(addresses, address)
	}

	return addresses, nil
}

// runCreatePostureCheckMac implements the command to create a mac address posture check
func runCreatePostureCheckMac(o *createPostureCheckMacOptions) (err error) {
	entityData := gabs.New()
	setPostureCheckEntityValues(entityData, &o.createPostureCheckOptions, PostureCheckTypeMAC)

	addresses, err := cleanMacAddresses(o.addresses)

	if err != nil {
		return err
	}

	if len(addresses) == 0 {
		return fmt.Errorf("must specify at least one MAC Address, multiple values may be separated by commas")
	}

	setJSONValue(entityData, addresses, "macAddresses")

	result, err := createEntityOfType("posture-checks", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	checkId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", checkId); err != nil {
		panic(err)
	}

	return err
}

func cleanDomains(inDomains []string) []string {
	domainMap := map[string]struct{}{}
	for _, domain := range inDomains {
		cleanDomain := strings.TrimSpace(strings.ToLower(domain))

		if len(cleanDomain) > 0 {
			domainMap[cleanDomain] = struct{}{}
		}
	}

	var domains []string
	for domain := range domainMap {
		domains = append(domains, domain)
	}

	return domains
}

// runCreatePostureCheckDomain implements the command to create a windows domain posture check
func runCreatePostureCheckDomain(o *createPostureCheckDomainOptions) (err error) {
	entityData := gabs.New()
	setPostureCheckEntityValues(entityData, &o.createPostureCheckOptions, PostureCheckTypeDomain)

	domains := cleanDomains(o.domains)

	if len(domains) == 0 {
		return fmt.Errorf("must specify at least one domain, multiple values may be separated by commas or multiple flags")
	}

	setJSONValue(entityData, domains, "domains")

	result, err := createEntityOfType("posture-checks", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	checkId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", checkId); err != nil {
		panic(err)
	}

	return err
}

func newCreatePostureCheckOsCmd(f cmdutil.Factory, out io.Writer, errOut io.Writer) *cobra.Command {

	options := &createPostureCheckOsOptions{
		createPostureCheckOptions: createPostureCheckOptions{
			commonOptions: commonOptions{
				CommonOptions: common.CommonOptions{
					Factory: f,
					Out:     out,
					Err:     errOut,
				},
			},
			tags: make(map[string]string),
		},
		os: nil,
	}

	cmd := &cobra.Command{
		Use:   "os <name>",
		Short: "creates a posture check for specific operating systems",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MinimumNArgs(1)(cmd, args); err != nil {
				return err
			}

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			options.Cmd = cmd
			options.Args = args

			err := runCreatePostureCheckOs(options)
			cmdhelper.CheckErr(err)
		},
		SuggestFor: []string{},
	}

	// allow interspersing positional args and flags
	cmd.Flags().SetInterspersed(true)
	options.AddCommonFlags(cmd)
	options.addPostureFlags(cmd)
	cmd.Flags().StringSliceVarP(&options.os, "os", "o", nil,
		"Set OS(es) of the posture check, should be in the format of '<os>:<version>:<version>:...', multiple may be specified via CSV or multiple flags")

	return cmd
}

func runCreatePostureCheckOs(o *createPostureCheckOsOptions) error {
	var osSpecs []*osSpec
	for i, osStr := range o.os {
		if osSpec, err := parseOsSpec(osStr); err != nil {
			return fmt.Errorf("could not prase os at index [%d]: %v", i, err)
		} else {
			osSpecs = append(osSpecs, osSpec)
		}
	}

	entityData := gabs.New()
	setPostureCheckEntityValues(entityData, &o.createPostureCheckOptions, PostureCheckTypeOS)
	setJSONValue(entityData, osSpecs, "operatingSystems")

	result, err := createEntityOfType("posture-checks", entityData.String(), &o.commonOptions)

	if err != nil {
		panic(err)
	}

	checkId := result.S("data", "id").Data()

	if _, err = fmt.Fprintf(o.Out, "%v\n", checkId); err != nil {
		panic(err)
	}

	return nil
}

type osSpec struct {
	Type     string
	Versions []string
}

func parseOsSpec(osSpecStr string) (*osSpec, error) {
	splits := strings.Split(osSpecStr, ":")

	osType := normalizeOsType(splits[0])

	if osType == "" {
		return nil, fmt.Errorf("invalid os type [%s]", splits[0])
	}

	cleanVersions := []string{}
	//have versions
	if len(splits) > 1 {
		versions := splits[1:]

		for _, version := range versions {
			cleanVersion := strings.TrimSpace(version)
			cleanVersions = append(cleanVersions, cleanVersion)
		}
	}

	return &osSpec{
		Type:     osType,
		Versions: cleanVersions,
	}, nil
}
