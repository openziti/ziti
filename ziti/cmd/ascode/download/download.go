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
	"github.com/judedaryl/go-arrayutils"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/ziti/internal"
	"github.com/openziti/ziti/internal/rest/mgmt"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/constants"
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
	loginOpts        edge.LoginOptions
	client           *rest_management_api_client.ZitiEdgeManagement
	ofJson           bool
	ofYaml           bool
	file             *os.File
	filename         string
	configCache      *cache.Cache
	configTypeCache  *cache.Cache
	authPolicyCache  *cache.Cache
	externalJwtCache *cache.Cache
}

var output Output

func NewDownloadCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	d := &Download{}
	d.loginOpts = edge.LoginOptions{}

	cmd := &cobra.Command{
		Use:   "export [entity]",
		Short: "Export entities",
		Long: "Export all or selected entities.\n" +
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
	viper.SetEnvPrefix(constants.ZITI) // All env vars we seek will be prefixed with "ZITI_"

	// Environment variables can't have dashes in them, so bind them to their equivalent
	// keys with underscores, d.g. --favorite-color to STING_FAVORITE_COLOR
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	v.AutomaticEnv()

	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(&d.ofJson, "json", true, "Output in JSON")
	cmd.Flags().BoolVar(&d.ofYaml, "yaml", false, "Output in YAML")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")

	cmd.Flags().StringVarP(&d.filename, "output-file", "o", "", "Write output to local file")

	edge.AddLoginFlags(cmd, &d.loginOpts)
	d.loginOpts.Out = out
	d.loginOpts.Err = errOut

	return cmd
}

func (d *Download) Init(out io.Writer) error {

	logLvl := logrus.InfoLevel
	if d.loginOpts.Verbose {
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
		o, err := NewOutputToFile(d.loginOpts.Verbose, d.ofJson, d.ofYaml, d.filename, d.loginOpts.Err)
		if err != nil {
			return err
		}
		output = *o
	} else {
		o, err := NewOutputToWriter(d.loginOpts.Verbose, d.ofJson, d.ofYaml, out, d.loginOpts.Err)
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
		log.Debug("Processing Certificate Authorities")
		cas, err := d.GetCertificateAuthorities()
		if err != nil {
			return err
		}
		result["certificateAuthorities"] = cas
	}
	if all ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		log.Debug("Processing Identities")
		identities, err := d.GetIdentities()
		if err != nil {
			return err
		}
		result["identities"] = identities
	}

	if all ||
		slices.Contains(args, "edge-router") || slices.Contains(args, "edge-routers") ||
		slices.Contains(args, "er") || slices.Contains(args, "ers") {
		log.Debug("Processing Edge Routers")
		routers, err := d.GetEdgeRouters()
		if err != nil {
			return err
		}
		result["edgeRouters"] = routers
	}
	if all ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		log.Debug("Processing Services")
		services, err := d.GetServices()
		if err != nil {
			return err
		}
		result["services"] = services
	}
	if all ||
		slices.Contains(args, "config") || slices.Contains(args, "configs") {
		log.Debug("Processing Configs")
		configs, err := d.GetConfigs()
		if err != nil {
			return err
		}
		result["configs"] = configs
	}
	if all ||
		slices.Contains(args, "config-type") || slices.Contains(args, "config-types") {
		log.Debug("Processing Config Types")
		configTypes, err := d.GetConfigTypes()
		if err != nil {
			return err
		}
		result["configTypes"] = configTypes
	}
	if all ||
		slices.Contains(args, "service-policy") || slices.Contains(args, "service-policies") {
		log.Debug("Processing Service Policies")
		servicePolicies, err := d.GetServicePolicies()
		if err != nil {
			return err
		}
		result["servicePolicies"] = servicePolicies
	}
	if all ||
		slices.Contains(args, "edgerouter-policy") || slices.Contains(args, "edgerouter-policies") {
		log.Debug("Processing Router Policies")
		routerPolicies, err := d.GetRouterPolicies()
		if err != nil {
			return err
		}
		result["edgeRouterPolicies"] = routerPolicies
	}
	if all ||
		slices.Contains(args, "service-edgerouter-policy") || slices.Contains(args, "service-edgerouter-policies") {
		log.Debug("Processing Service EdgeRouter Policies")
		serviceRouterPolicies, err := d.GetServiceEdgeRouterPolicies()
		if err != nil {
			return err
		}
		result["serviceEdgeRouterPolicies"] = serviceRouterPolicies
	}
	if all ||
		slices.Contains(args, "external-jwt-signer") || slices.Contains(args, "external-jwt-signers") {
		log.Debug("Processing External JWT Signers")
		externalJwtSigners, err := d.GetExternalJwtSigners()
		if err != nil {
			return err
		}
		result["externalJwtSigners"] = externalJwtSigners
	}
	if all ||
		slices.Contains(args, "auth-policy") || slices.Contains(args, "auth-policies") {
		log.Debug("Processing Auth Policies")
		authPolicies, err := d.GetAuthPolicies()
		if err != nil {
			return err
		}
		result["authPolicies"] = authPolicies
	}
	if all ||
		slices.Contains(args, "posture-check") || slices.Contains(args, "posture-checks") {
		log.Debug("Processing Posture Checks")
		postureChecks, err := d.GetPostureChecks()
		if err != nil {
			return err
		}
		result["postureChecks"] = postureChecks
	}

	log.Debug("Download complete")

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
		_, _ = internal.FPrintFReusingLine(d.loginOpts.Err, "Reading %d/%d %s\r", offset, totalCount, entityName)
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

	_, _ = internal.FPrintFReusingLine(d.loginOpts.Err, "Read %d %s\r\n", len(result), entityName)

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
