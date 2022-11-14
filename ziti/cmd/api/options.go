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
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/openziti/ziti/ziti/cmd/common"
	"github.com/spf13/cobra"
	"io"
)

// Options are common options for edge controller commands
type Options struct {
	common.CommonOptions
	OutputJSONRequest  bool
	OutputJSONResponse bool
	OutputCSV          bool
}

func (options *Options) OutputResponseJson() bool {
	return options.OutputJSONResponse
}

func (options *Options) OutputRequestJson() bool {
	return options.OutputJSONRequest
}

func (options *Options) OutputWriter() io.Writer {
	return options.CommonOptions.Out
}

func (options *Options) ErrOutputWriter() io.Writer {
	return options.CommonOptions.Err
}

func (options *Options) AddCommonFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&common.CliIdentity, "cli-identity", "i", "", "Specify the saved identity you want the CLI to use when connect to the controller with")
	cmd.Flags().BoolVarP(&options.OutputJSONResponse, "output-json", "j", false, "Output the full JSON response from the Ziti Edge Controller")
	cmd.Flags().BoolVar(&options.OutputJSONRequest, "output-request-json", false, "Output the full JSON request to the Ziti Edge Controller")
	cmd.Flags().IntVarP(&options.Timeout, "timeout", "", 5, "Timeout for REST operations (specified in seconds)")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
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
