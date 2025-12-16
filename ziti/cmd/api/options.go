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

package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/Jeffail/gabs"
	ziticobra "github.com/openziti/ziti/internal/cobra"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const CommonFlagKey = "common"

// Options are common options for edge controller commands
type Options struct {
	common.CommonOptions
	OutputJSONRequest  bool
	OutputJSONResponse bool
	OutputCSV          bool
	OptionsMap         map[string]any
}

func (options *Options) OutputResponseJson() bool {
	return options.OutputJSONResponse
}

func (options *Options) OutputRequestJson() bool {
	return options.OutputJSONRequest
}

func (options *Options) OutputWriter() io.Writer {
	return options.Out
}

func (options *Options) ErrOutputWriter() io.Writer {
	return options.Err
}

func addCommonFlag(cmd *cobra.Command, flagName string) {
	_ = ziticobra.AddFlagAnnotation(cmd, flagName, CommonFlagKey, "true")
}

func (options *Options) AddCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&common.CliIdentity, "cli-identity", "i", "", "Specify the saved identity you want the CLI to use when connect to the controller with")
	addCommonFlag(cmd, "cli-identity")
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	addCommonFlag(cmd, "output-json")
	cmd.Flags().BoolVar(&options.OutputJSONRequest, "output-request-json", false, "Output the full JSON request to the Ziti Edge Controller")
	addCommonFlag(cmd, "output-request-json")
	cmd.Flags().IntVarP(&options.Timeout, "timeout", "", 5, "Timeout for REST operations (specified in seconds)")
	addCommonFlag(cmd, "timeout")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	addCommonFlag(cmd, "verbose")
}

func (options *Options) LogCreateResult(entityType string, result *gabs.Container, err error) error {
	if err != nil {
		return err
	}

	if !options.OutputJSONResponse {
		id := result.S("data", "id").Data()
		_, err = fmt.Fprintf(options.Out, "New %v %v created with id: %v\n", entityType, options.Args[0], id)
		return err
	}
	return nil
}

type EntityOptions struct {
	Options
	Tags     map[string]string
	TagsJson string
}

func (self *EntityOptions) AddCommonFlags(cmd *cobra.Command) {
	self.Options.AddCommonFlags(cmd)
	self.Tags = map[string]string{}
	cmd.Flags().VarP(newStringToStringValue(self.Tags), "tags", func() string {
		if cmd.Flags().ShorthandLookup("t") == nil {
			return "t"
		}
		return ""
	}(), "Add tags to entity definition")
	cmd.Flags().StringVar(&self.TagsJson, "tags-json", "", "Add tags defined in JSON to entity definition")
}

func (self *EntityOptions) GetTags() map[string]interface{} {
	result := map[string]interface{}{}
	if len(self.TagsJson) > 0 {
		if err := json.Unmarshal([]byte(self.TagsJson), &result); err != nil {
			logrus.Fatalf("invalid tags JSON: '%s'", self.TagsJson)
		}
	}
	for k, v := range self.Tags {
		result[k] = v
	}
	return result
}

func (self *EntityOptions) TagsProvided() bool {
	return self.Cmd.Flags().Changed("tags") || self.Cmd.Flags().Changed("tags-json")
}

func (self *EntityOptions) SetTags(container *gabs.Container) {
	tags := self.GetTags()
	SetJSONValue(container, tags, "tags")
}

func NewEntityOptions(out, errOut io.Writer) EntityOptions {
	return EntityOptions{
		Options: Options{
			CommonOptions: common.CommonOptions{
				Out: out,
				Err: errOut,
			},
		},
	}
}

func newStringToStringValue(val map[string]string) pflag.Value {
	return &stringToStringMap{
		values: val,
	}
}

// stringToStringMap replaces the cobra version with a similar version that allows using
// a zero-length string to set an empty map. The cobra version doesn't provide a way to
// set an empty string
type stringToStringMap struct {
	values map[string]string
}

func (self *stringToStringMap) String() string {
	buf := bytes.Buffer{}
	first := true
	for k, v := range self.values {
		if !first {
			buf.WriteString(",")
		} else {
			first = false
		}
		buf.WriteString(k)
		buf.WriteString("=")
		buf.WriteString(v)
	}
	return buf.String()
}

func (self *stringToStringMap) Set(s string) error {
	if s == "" {
		self.values = map[string]string{}
		return nil
	}

	r := csv.NewReader(strings.NewReader(s))

	ss, err := r.Read()
	if err != nil {
		return err
	}
	for _, pair := range ss {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return fmt.Errorf("%s must be formatted as key=value", pair)
		}
		self.values[key] = value
	}
	return nil
}

func (self *stringToStringMap) Type() string {
	return "stringToStringMap"
}
