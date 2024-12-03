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

package upload

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
	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

type Upload struct {
	loginOpts edge.LoginOptions
	client    *rest_management_api_client.ZitiEdgeManagement
	reader    Reader

	verbose bool

	ofJson bool
	ofYaml bool

	configCache        *cache.Cache
	serviceCache       *cache.Cache
	edgeRouterCache    *cache.Cache
	authPolicyCache    *cache.Cache
	extJwtSignersCache *cache.Cache
	identityCache      *cache.Cache
}

const envPrefix = "ZITI"

var upload Upload
var cmd *cobra.Command

/*
	func NewUpload() Upload {
		u := Upload{}
		return u
	}
*/
var log = pfxlog.Logger()

func NewUploadCmd(out io.Writer, errOut io.Writer) *cobra.Command {

	u := &Upload{}
	uploadCmd := &cobra.Command{
		Use:   "import filename [entity]",
		Short: "Import ziti entities",
		Long: "Import all or selected ziti entities from the specified file.\n" +
			"Valid entities are: [all|ca/certificate-authority|identity|edge-router|service|config|config-type|service-policy|edgerouter-policy|service-edgerouter-policy|external-jwt-signer|auth-policy|posture-check] (default all)",
		Args: cobra.MinimumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			u.Init()
		},
		Run: func(cmd *cobra.Command, args []string) {
			data, err := u.reader.read(args[0])
			if err != nil {
				panic(errors.Join(errors.New("unable to read input"), err))
			}
			m := map[string][]interface{}{}

			if u.ofYaml {
				err = yaml.Unmarshal(data, &m)
				if err != nil {
					panic(errors.Join(errors.New("unable to parse input data as yaml"), err))
				}
			} else {
				err = json.Unmarshal(data, &m)
				if err != nil {
					panic(errors.Join(errors.New("unable to parse input data as json"), err))
				}
			}

			result, executeErr := u.Execute(m, args[1:])
			if executeErr != nil {
				panic(executeErr)
			}
			if u.verbose {
				log.
					WithField("results", result).
					Debug("Finished")
			}
		},
	}

	v := viper.New()

	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	uploadCmd.Flags().BoolVar(&u.ofJson, "json", true, "Input parsed as JSON")
	uploadCmd.Flags().BoolVar(&u.ofYaml, "yaml", false, "Input parsed as YAML")
	uploadCmd.MarkFlagsMutuallyExclusive("json", "yaml")

	uploadCmd.PersistentFlags().BoolVarP(&u.verbose, "verbose", "v", false, "Enable verbose logging")
	uploadCmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Determine the naming convention of the flags when represented in the config file
		configName := f.Name
		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(configName) {
			val := v.Get(configName)
			uploadCmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}
	})

	edge.AddLoginFlags(uploadCmd, &u.loginOpts)
	u.loginOpts.Out = out
	u.loginOpts.Err = errOut

	cmd = uploadCmd
	upload = *u
	return uploadCmd
}

func (u *Upload) Init() {
	logLvl := logrus.InfoLevel
	if u.verbose {
		logLvl = logrus.DebugLevel
	}

	pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
	internal.ConfigureLogFormat(logLvl)

	client, err := mgmt.NewClient()
	if err != nil {
		loginErr := u.loginOpts.Run()
		if loginErr != nil {
			log.Fatal(err)
		}
		client, err = mgmt.NewClient()
		if err != nil {
			log.Fatal(err)
		}
	}
	u.client = client
	u.reader = FileReader{}
}

func (u *Upload) Execute(data map[string][]interface{}, inputArgs []string) (map[string]any, error) {

	args := arrayutils.Map(inputArgs, strings.ToLower)

	u.configCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.serviceCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.edgeRouterCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.authPolicyCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.extJwtSignersCache = cache.New(time.Duration(-1), time.Duration(-1))
	u.identityCache = cache.New(time.Duration(-1), time.Duration(-1))

	result := map[string]any{}
	all := slices.Contains(args, "all") || len(args) == 0

	cas := map[string]string{}
	if all ||
		slices.Contains(args, "ca") || slices.Contains(args, "cas") ||
		slices.Contains(args, "certificate-authority") || slices.Contains(args, "certificate-authorities") {
		if u.verbose {
			log.
				Debug("Processing CertificateAuthorities")
		}
		var err error
		cas, err = u.ProcessCertificateAuthorities(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("certificateAuthorities", cas).
				Debug("CertificateAuthorities created")
		}
	}
	result["certificateAuthorities"] = cas

	externalJwtSigners := map[string]string{}
	if all ||
		slices.Contains(args, "external-jwt-signer") || slices.Contains(args, "external-jwt-signers") ||
		slices.Contains(args, "auth-policy") || slices.Contains(args, "auth-policies") ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if u.verbose {
			log.
				Debug("Processing ExtJWTSigners")
		}
		var err error
		externalJwtSigners, err = u.ProcessExternalJwtSigners(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("externalJwtSigners", externalJwtSigners).
				Debug("ExtJWTSigners created")
		}
	}
	result["externalJwtSigners"] = externalJwtSigners

	authPolicies := map[string]string{}
	if all ||
		slices.Contains(args, "auth-policy") || slices.Contains(args, "auth-policies") ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if u.verbose {
			log.
				Debug("Processing AuthPolicies")
		}
		var err error
		authPolicies, err = u.ProcessAuthPolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("authPolicies", authPolicies).
				Debug("AuthPolicies created")
		}
	}
	result["authPolicies"] = authPolicies

	identities := map[string]string{}
	if all ||
		slices.Contains(args, "identity") || slices.Contains(args, "identities") {
		if u.verbose {
			log.
				Debug("Processing Identities")
		}
		var err error
		identities, err = u.ProcessIdentities(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("identities", identities).
				Debug("Identities created")
		}
	}
	result["identities"] = identities

	configTypes := map[string]string{}
	if all ||
		slices.Contains(args, "config-type") || slices.Contains(args, "config-types") ||
		slices.Contains(args, "config") || slices.Contains(args, "configs") ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if u.verbose {
			log.
				Debug("Processing ConfigTypes")
		}
		var err error
		configTypes, err = u.ProcessConfigTypes(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("configTypes", configTypes).
				Debug("ConfigTypes created")
		}
	}
	result["configTypes"] = configTypes

	configs := map[string]string{}
	if all ||
		slices.Contains(args, "config") || slices.Contains(args, "configs") ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if u.verbose {
			log.
				Debug("Processing Configs")
		}
		var err error
		configs, err = u.ProcessConfigs(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("configs", configs).
				Debug("Configs created")
		}
	}
	result["configs"] = configs

	services := map[string]string{}
	if all ||
		slices.Contains(args, "service") || slices.Contains(args, "services") {
		if u.verbose {
			log.
				Debug("Processing Services")
		}
		var err error
		services, err = u.ProcessServices(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("services", services).
				Debug("Services created")
		}
	}
	result["services"] = services

	postureChecks := map[string]string{}
	if all ||
		slices.Contains(args, "posture-check") || slices.Contains(args, "posture-checks") {
		if u.verbose {
			log.
				Debug("Processing PostureChecks")
		}
		var err error
		postureChecks, err = u.ProcessPostureChecks(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("postureChecks", postureChecks).
				Debug("PostureChecks created")
		}
	}
	result["postureChecks"] = postureChecks

	routers := map[string]string{}
	if all ||
		slices.Contains(args, "edge-router") || slices.Contains(args, "edge-routers") ||
		slices.Contains(args, "ers") || slices.Contains(args, "ers") {
		if u.verbose {
			log.
				Debug("Processing EdgeRouters")
		}
		var err error
		routers, err = u.ProcessEdgeRouters(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("edgeRouters", routers).
				Debug("EdgeRouters created")
		}
	}
	result["edgeRouters"] = routers

	serviceEdgeRouterPolicies := map[string]string{}
	if all ||
		slices.Contains(args, "service-edgerouter-policy") || slices.Contains(args, "service-edgerouter-policies") {
		if u.verbose {
			log.
				Debug("Processing ServiceRouterPolicies")
		}
		var err error
		serviceEdgeRouterPolicies, err = u.ProcessServiceEdgeRouterPolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("serviceEdgeRouterPolicies", serviceEdgeRouterPolicies).
				Debug("ServiceEdgeRouterPolicies created")
		}
	}
	result["serviceEdgeRouterPolicies"] = serviceEdgeRouterPolicies

	servicePolicies := map[string]string{}
	if all ||
		slices.Contains(args, "service-policy") || slices.Contains(args, "service-policies") {
		if u.verbose {
			log.
				Debug("Processing ServicePolicies")
		}
		var err error
		servicePolicies, err = u.ProcessServicePolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("servicePolicies", servicePolicies).
				Debug("ServicePolicies created")
		}
	}
	result["servicePolicies"] = servicePolicies

	routerPolicies := map[string]string{}
	if all ||
		slices.Contains(args, "edgerouter-policy") || slices.Contains(args, "edgerouter-policies") {
		if u.verbose {
			log.
				Debug("Processing EdgeRouterPolicies")
		}
		var err error
		routerPolicies, err = u.ProcessEdgeRouterPolicies(data)
		if err != nil {
			return nil, err
		}
		if u.verbose {
			log.
				WithField("routerPolicies", routerPolicies).
				Debug("EdgeRouterPolicies created")
		}
	}
	result["edgeRouterPolicies"] = routerPolicies

	if u.verbose {
		log.
			Info("Upload complete")
	}

	return result, nil
}

func FromMap[T interface{}](input interface{}, v T) *T {
	jsonData, _ := json.MarshalIndent(input, "", "  ")
	create := new(T)
	err := json.Unmarshal(jsonData, &create)
	if err != nil {
		log.
			WithField("err", err).
			Error("error converting input to object")
		return nil
	}
	return create
}

type Reader interface {
	read(input any) ([]byte, error)
}

type FileReader struct {
}

func (i FileReader) read(input any) ([]byte, error) {
	file, err := os.ReadFile(input.(string))
	if err != nil {
		return nil, err
	}

	return file, nil
}
