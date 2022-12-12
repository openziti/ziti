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

package common

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"time"
)

type Printer interface {
	Printf(format string, args ...interface{})
	Println(val interface{})
}

type OptionsProvider func() CommonOptions

func NewOptionsProvider(out, err io.Writer) OptionsProvider {
	return func() CommonOptions {
		return CommonOptions{
			Out: out,
			Err: err,
		}
	}
}

// CommonOptions contains common options and helper methods
type CommonOptions struct {
	Out            io.Writer
	Err            io.Writer
	Cmd            *cobra.Command
	Args           []string
	BatchMode      bool
	Verbose        bool
	Staging        bool
	ConfigIdentity string
	Timeout        int
	Identity       string
}

func (options *CommonOptions) AddCommonFlags(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&options.BatchMode, "batch-mode", "b", false, "In batch mode the command never prompts for user input")
	cmd.Flags().BoolVarP(&options.Verbose, "verbose", "", false, "Enable verbose logging")
	cmd.Flags().BoolVarP(&options.Staging, "staging", "", false, "Install/Upgrade components from the ziti-staging repo")
	cmd.Flags().StringVarP(&options.ConfigIdentity, "configIdentity", "i", "", "Which configIdentity to use")
	cmd.Flags().IntVarP(&options.Timeout, "timeout", "t", 5, "Timeout for REST operations (specified in seconds)")
	options.Cmd = cmd
}

func (options *CommonOptions) TimeoutContext() (context.Context, context.CancelFunc) {
	timeout := time.Duration(options.Timeout) * time.Second

	return context.WithTimeout(context.Background(), timeout)
}

func (options *CommonOptions) GetFilter() *string {
	if len(options.Args) > 0 {
		return &options.Args[0]
	}
	return nil
}

func (options *CommonOptions) GetContext() context.Context {
	ctx, _ := context.WithTimeout(context.Background(), time.Second*time.Duration(options.Timeout))
	return ctx
}

func (options *CommonOptions) Printf(format string, args ...interface{}) {
	if _, err := fmt.Fprintf(options.Out, format, args...); err != nil {
		panic(err)
	}
}

func (options *CommonOptions) Println(s interface{}) {
	if _, err := fmt.Fprintln(options.Out, s); err != nil {
		panic(err)
	}
}

var CliIdentity string
