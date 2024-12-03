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

package download

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/judedaryl/go-arrayutils"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"github.com/openziti/ziti/ziti/cmd/edge"
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"os"
	"slices"
	"strings"
)

var log = pfxlog.Logger()

type Download struct {
	loginOpts edge.LoginOptions
	client    *rest_management_api_client.ZitiEdgeManagement

	verbose bool

	ofJson bool
	ofYaml bool

	file     *os.File
	filename string

	configCache      *cache.Cache
	configTypeCache  *cache.Cache
	authPolicyCache  *cache.Cache
	externalJwtCache *cache.Cache
}

var output Output

func NewDownload(loginOpts edge.LoginOptions, client *rest_management_api_client.ZitiEdgeManagement, ofJson bool, ofYaml bool, file *os.File, filename string) Download {
	d := Download{}
	d.loginOpts = loginOpts
	d.client = client
	d.ofJson = ofJson
	d.ofYaml = ofYaml
	d.file = file
	d.filename = filename
	return d
}

func NewDownloadCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	d := &Download{}
	downloadCmd := &cobra.Command{
		Use:   "export [entity]",
		Short: "Export Ziti entities",
		Long: "Export all or selected Ziti entities.\n" +
			"Valid entities are: [all|ca/certificate-authority|identity|edge-router|service|config|config-type|service-policy|edgerouter-policy|service-edgerouter-policy|external-jwt-signer|auth-policy|posture-check] (default all)",
		Args: cobra.MinimumNArgs(0),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			err := d.Init(out)
			if err != nil {
				panic(err)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			err := d.Execute(args)
			if err != nil {
				panic(err)
			}
		},
		Hidden: true,
	}

	v := viper.New()

	// When we bind flags to environment variables expect that the
	// environment variables are prefixed, d.g. a flag like --number
	// binds to an environment variable STING_NUMBER. This helps
	// avoid conflicts.
	viper.SetEnvPrefix(c.ZITI) // All env vars we seek will be prefixed with "ZITI_"

	// Environment variables can't have dashes in them, so bind them to their equivalent
	// keys with underscores, d.g. --favorite-color to STING_FAVORITE_COLOR
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	v.AutomaticEnv()

	downloadCmd.Flags().BoolVar(&d.ofJson, "json", true, "Output in JSON")
	downloadCmd.Flags().BoolVar(&d.ofYaml, "yaml", false, "Output in YAML")
	downloadCmd.MarkFlagsMutuallyExclusive("json", "yaml")

	downloadCmd.PersistentFlags().StringVarP(&d.filename, "output-file", "o", "", "Write output to local file")

	downloadCmd.PersistentFlags().BoolVarP(&d.verbose, "verbose", "v", false, "Enable verbose logging")

	edge.AddLoginFlags(downloadCmd, &d.loginOpts)
	d.loginOpts.Out = out
	d.loginOpts.Err = errOut

	return downloadCmd
}

func (d *Download) Init(out io.Writer) error {
	logLvl := logrus.InfoLevel
	if d.verbose {
		logLvl = logrus.DebugLevel
	}

	pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
	internal.ConfigureLogFormat(logLvl)

	client, err := mgmt.NewClient()
	if err != nil {
		loginErr := d.loginOpts.Run()
		if loginErr != nil {
			log.Fatal(err)
		}
		client, err = mgmt.NewClient()
		if err != nil {
			log.Fatal(err)
		}
	}
	d.client = client

	if d.filename != "" {
		o, err := NewOutputToFile(d.verbose, d.ofJson, d.ofYaml, d.filename)
		if err != nil {
			return err
		}
		output = *o
	} else {
		o, err := NewOutputToWriter(d.verbose, d.ofJson, d.ofYaml, out)
		if err != nil {
			return err
		}
		output = *o
	}

	return nil
}

func (d *Download) Execute(input []string) error {

	args := arrayutils.Map(input, strings.ToLower)

	d.authPolicyCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	d.configCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	d.configTypeCache = cache.New(cache.NoExpiration, cache.NoExpiration)
	d.externalJwtCache = cache.New(cache.NoExpiration, cache.NoExpiration)

	result := map[string]interface{}{}

	all := slices.Contains(args, "all") || len(args) == 0
	if all ||
		slices.Contains(args, "ca") || slices.Contains(args, "cas") ||
		slices.Contains(args, "certificate-authority") || slices.Contains(args, "certificate-authorities") {
		if d.verbose {
			log.Debug("Processing Certificate Authorities")
		}
		cas, err := d.GetCertificateAuthorities()
		if err != nil {
			return err
		}
		result["certificateAuthorities"] = cas
	}
	if all ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if d.verbose {
			log.Debug("Processing Identities")
		}
		identities, err := d.GetIdentities()
		if err != nil {
			return err
		}
		result["identities"] = identities
	}

	if all ||
		slices.Contains(args, "edge-router") || slices.Contains(args, "edge-routers") ||
		slices.Contains(args, "er") || slices.Contains(args, "ers") {
		if d.verbose {
			log.Debug("Processing Edge Routers")
		}
		routers, err := d.GetEdgeRouters()
		if err != nil {
			return err
		}
		result["routers"] = routers
	}
	if all ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if d.verbose {
			log.Debug("Processing Services")
		}
		services, err := d.GetServices()
		if err != nil {
			return err
		}
		result["services"] = services
	}
	if all ||
		slices.Contains(args, "config") || slices.Contains(args, "configs") {
		if d.verbose {
			log.Debug("Processing Configs")
		}
		configs, err := d.GetConfigs()
		if err != nil {
			return err
		}
		result["configs"] = configs
	}
	if all ||
		slices.Contains(args, "config-type") || slices.Contains(args, "config-types") {
		if d.verbose {
			log.Debug("Processing Config Types")
		}
		configTypes, err := d.GetConfigTypes()
		if err != nil {
			return err
		}
		result["configTypes"] = configTypes
	}
	if all ||
		slices.Contains(args, "service-policy") || slices.Contains(args, "service-policies") {
		if d.verbose {
			log.Debug("Processing Service Policies")
		}
		servicePolicies, err := d.GetServicePolicies()
		if err != nil {
			return err
		}
		result["servicePolicies"] = servicePolicies
	}
	if all ||
		slices.Contains(args, "edgerouter-policy") || slices.Contains(args, "edgerouter-policies") {
		if d.verbose {
			log.Debug("Processing Router Policies")
		}
		routerPolicies, err := d.GetRouterPolicies()
		if err != nil {
			return err
		}
		result["edgeRouterPolicies"] = routerPolicies
	}
	if all ||
		slices.Contains(args, "service-edgerouter-policy") || slices.Contains(args, "service-edgerouter-policies") {
		if d.verbose {
			log.Debug("Processing Service EdgeRouter Policies")
		}
		serviceRouterPolicies, err := d.GetServiceEdgeRouterPolicies()
		if err != nil {
			return err
		}
		result["serviceEdgeRouterPolicies"] = serviceRouterPolicies
	}
	if all ||
		slices.Contains(args, "external-jwt-signer") || slices.Contains(args, "external-jwt-signers") {
		if d.verbose {
			log.Debug("Processing External JWT Signers")
		}
		externalJwtSigners, err := d.GetExternalJwtSigners()
		if err != nil {
			return err
		}
		result["externalJwtSigners"] = externalJwtSigners
	}
	if all ||
		slices.Contains(args, "auth-policy") || slices.Contains(args, "auth-policies") {
		if d.verbose {
			log.Debug("Processing Auth Policies")
		}
		authPolicies, err := d.GetAuthPolicies()
		if err != nil {
			return err
		}
		result["authPolicies"] = authPolicies
	}
	if all ||
		slices.Contains(args, "posture-check") || slices.Contains(args, "posture-checks") {
		if d.verbose {
			log.Debug("Processing Posture Checks")
		}
		postureChecks, err := d.GetPostureChecks()
		if err != nil {
			return err
		}
		result["postureChecks"] = postureChecks
	}

	if d.verbose {
		log.Debug("Download complete")
	}

	err := output.Write(result)
	if err != nil {
		return err
	}
	if d.file != nil {
		err := d.file.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

type ClientCount func() (int64, error)
type ClientList func(offset *int64, limit *int64) ([]interface{}, error)
type EntityProcessor func(item interface{}) (map[string]interface{}, error)

func (d *Download) getEntities(entityName string, count ClientCount, list ClientList, processor EntityProcessor) ([]map[string]interface{}, error) {

	totalCount, countErr := count()
	if countErr != nil {
		return nil, errors.Join(errors.New("error reading total number of "+entityName), countErr)
	}

	result := []map[string]interface{}{}

	offset := int64(0)
	limit := int64(500)
	more := true
	for more {
		resp, err := list(&offset, &limit)
		_, _ = fmt.Fprintf(os.Stderr, "\u001B[2KReading %d/%d %s\r", offset, totalCount, entityName)
		if err != nil {
			return nil, errors.Join(errors.New("error reading "+entityName), err)
		}

		for _, item := range resp {
			m, err := processor(item)
			if err != nil {
				return nil, err
			}
			if m != nil {
				result = append(result, m)
			}
		}

		more = offset < totalCount
		offset += limit
	}

	_, _ = fmt.Fprintf(os.Stderr, "\u001B[2KRead %d %s\r\n", len(result), entityName)

	return result, nil

}

func (d *Download) ToMap(input interface{}) map[string]interface{} {
	jsonData, _ := json.MarshalIndent(input, "", "")
	m := map[string]interface{}{}
	err := json.Unmarshal(jsonData, &m)
	if err != nil {
		log.WithError(err).Error("error converting input to map")
		return map[string]interface{}{}
	}
	return m
}

func (d *Download) defaultRoleAttributes(m map[string]interface{}) {
	if m["roleAttributes"] == nil {
		m["roleAttributes"] = []string{}
	}
}

func (d *Download) Filter(m map[string]interface{}, properties []string) {

	// remove any properties that are not requested
	for k := range m {
		if slices.Contains(properties, k) {
			delete(m, k)
		}
	}
}
