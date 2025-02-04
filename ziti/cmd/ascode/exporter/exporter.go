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

package exporter

import (
	"encoding/json"
	"errors"
	"github.com/judedaryl/go-arrayutils"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/ziti/internal"
	ziticobra "github.com/openziti/ziti/internal/cobra"
	"github.com/openziti/ziti/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/constants"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"io"
	"os"
	"slices"
	"strings"
)

var log = pfxlog.Logger()

type Exporter struct {
	Out              io.Writer
	Err              io.Writer
	LoginOpts        edge.LoginOptions
	OfJson           bool
	OfYaml           bool
	Filename         string
	file             *os.File
	configCache      map[string]any
	configTypeCache  map[string]any
	authPolicyCache  map[string]any
	externalJwtCache map[string]any
	client           *rest_management_api_client.ZitiEdgeManagement
}

var output Output

func NewExportCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	exporter := &Exporter{
		Out: out,
		Err: errOut,
	}
	exporter.LoginOpts = edge.LoginOptions{}

	cmd := &cobra.Command{
		Use:   "export [entity]",
		Short: "Export entities",
		Long: "Export all or selected entities.\n" +
			"Valid entities are: [all|ca/certificate-authority|identity|edge-router|service|config|config-type|service-policy|edge-router-policy|service-edge-router-policy|external-jwt-signer|auth-policy|posture-check] (default all)",
		Args: cobra.MinimumNArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			err := exporter.Execute(args)
			if err != nil {
				log.Fatal(err)
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

	edge.AddLoginFlags(cmd, &exporter.LoginOpts)
	cmd.Flags().SetInterspersed(true)
	cmd.Flags().BoolVar(&exporter.OfJson, "json", true, "Output in JSON")
	cmd.Flags().BoolVar(&exporter.OfYaml, "yaml", false, "Output in YAML")
	cmd.Flags().StringVar(&exporter.LoginOpts.ControllerUrl, "controller-url", "", "The url of the controller")
	cmd.MarkFlagsMutuallyExclusive("json", "yaml")
	ziticobra.SetHelpTemplate(cmd)

	cmd.Flags().StringVarP(&exporter.Filename, "output-file", "o", "", "Write output to local file")

	return cmd
}

func (exporter *Exporter) Execute(input []string) error {

	logLvl := logrus.InfoLevel
	if exporter.LoginOpts.Verbose {
		logLvl = logrus.DebugLevel
	}

	pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
	internal.ConfigureLogFormat(logLvl)

	var err error
	exporter.client, err = exporter.LoginOpts.NewMgmtClient()
	if err != nil {
		return err
	}

	if exporter.Filename != "" {
		o, err := NewOutputToFile(exporter.LoginOpts.Verbose, exporter.OfJson, exporter.OfYaml, exporter.Filename, exporter.Err)
		if err != nil {
			return err
		}
		output = *o
	} else {
		o, err := NewOutputToWriter(exporter.LoginOpts.Verbose, exporter.OfJson, exporter.OfYaml, exporter.Out, exporter.Err)
		if err != nil {
			return err
		}
		output = *o
	}

	args := arrayutils.Map(input, strings.ToLower)

	exporter.authPolicyCache = map[string]any{}
	exporter.configCache = map[string]any{}
	exporter.configTypeCache = map[string]any{}
	exporter.externalJwtCache = map[string]any{}

	result := map[string]interface{}{}

	if exporter.IsCertificateAuthorityExportRequired(args) {
		log.Debug("Processing Certificate Authorities")
		cas, err := exporter.GetCertificateAuthorities()
		if err != nil {
			return err
		}
		result["certificateAuthorities"] = cas
	}
	if exporter.IsIdentityExportRequired(args) {
		log.Debug("Processing Identities")
		identities, err := exporter.GetIdentities()
		if err != nil {
			return err
		}
		result["identities"] = identities
	}

	if exporter.IsEdgeRouterExportRequired(args) {
		log.Debug("Processing Edge Routers")
		routers, err := exporter.GetEdgeRouters()
		if err != nil {
			return err
		}
		result["edgeRouters"] = routers
	}
	if exporter.IsServiceExportRequired(args) {
		log.Debug("Processing Services")
		services, err := exporter.GetServices()
		if err != nil {
			return err
		}
		result["services"] = services
	}
	if exporter.IsConfigExportRequired(args) {
		log.Debug("Processing Configs")
		configs, err := exporter.GetConfigs()
		if err != nil {
			return err
		}
		result["configs"] = configs
	}
	if exporter.IsConfigTypeExportRequired(args) {
		log.Debug("Processing Config Types")
		configTypes, err := exporter.GetConfigTypes()
		if err != nil {
			return err
		}
		result["configTypes"] = configTypes
	}
	if exporter.IsServicePolicyExportRequired(args) {
		log.Debug("Processing Service Policies")
		servicePolicies, err := exporter.GetServicePolicies()
		if err != nil {
			return err
		}
		result["servicePolicies"] = servicePolicies
	}
	if exporter.IsEdgeRouterExportRequired(args) {
		log.Debug("Processing Edge Router Policies")
		routerPolicies, err := exporter.GetEdgeRouterPolicies()
		if err != nil {
			return err
		}
		result["edgeRouterPolicies"] = routerPolicies
	}
	if exporter.IsServiceEdgeRouterPolicyExportRequired(args) {
		log.Debug("Processing Service Edge Router Policies")
		serviceRouterPolicies, err := exporter.GetServiceEdgeRouterPolicies()
		if err != nil {
			return err
		}
		result["serviceEdgeRouterPolicies"] = serviceRouterPolicies
	}
	if exporter.IsExtJwtSignerExportRequired(args) {
		log.Debug("Processing External JWT Signers")
		externalJwtSigners, err := exporter.GetExternalJwtSigners()
		if err != nil {
			return err
		}
		result["externalJwtSigners"] = externalJwtSigners
	}
	if exporter.IsAuthPolicyExportRequired(args) {
		log.Debug("Processing Auth Policies")
		authPolicies, err := exporter.GetAuthPolicies()
		if err != nil {
			return err
		}
		result["authPolicies"] = authPolicies
	}
	if exporter.IsPostureCheckExportRequired(args) {
		log.Debug("Processing Posture Checks")
		postureChecks, err := exporter.GetPostureChecks()
		if err != nil {
			return err
		}
		result["postureChecks"] = postureChecks
	}

	log.Debug("Export complete")

	err = output.Write(result)
	if err != nil {
		return err
	}
	if exporter.file != nil {
		err := exporter.file.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

type ClientCount func() (int64, error)
type ClientList func(offset *int64, limit *int64) ([]interface{}, error)
type EntityProcessor func(item interface{}) (map[string]interface{}, error)

func (exporter *Exporter) getEntities(entityName string, count ClientCount, list ClientList, processor EntityProcessor) ([]map[string]interface{}, error) {

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
		_, _ = internal.FPrintfReusingLine(exporter.Err, "Reading %d/%d %s", offset, totalCount, entityName)
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

	_, _ = internal.FPrintflnReusingLine(exporter.Err, "Read %d %s", len(result), entityName)

	return result, nil

}

func (exporter *Exporter) ToMap(input interface{}) (map[string]interface{}, error) {
	jsonData, _ := json.MarshalIndent(input, "", "")
	m := map[string]interface{}{}
	err := json.Unmarshal(jsonData, &m)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (exporter *Exporter) defaultRoleAttributes(m map[string]interface{}) {
	if m["roleAttributes"] == nil {
		m["roleAttributes"] = []string{}
	}
}

func (exporter *Exporter) Filter(m map[string]interface{}, properties []string) {

	// remove any properties that are not requested
	for k := range m {
		if slices.Contains(properties, k) {
			delete(m, k)
		}
	}
}
