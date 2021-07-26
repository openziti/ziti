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

package common

import (
	"context"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/factory"
	"github.com/spf13/cobra"
	"io"
	"time"
)

// CommonOptions contains common options and helper methods
type CommonOptions struct {
	Factory        util.Factory
	Out            io.Writer
	Err            io.Writer
	Cmd            *cobra.Command
	Args           []string
	BatchMode      bool
	Verbose        bool
	Staging        bool
	ConfigIdentity string
	Timeout        int
}

func (options *CommonOptions) TimeoutContext() (context.Context, context.CancelFunc) {
	timeout := time.Duration(options.Timeout) * time.Second

	return context.WithTimeout(context.Background(), timeout)
}
