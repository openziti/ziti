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
package util

import (
	"github.com/spf13/cobra"
)

func CancelableCobraCmd(ignoreErrors bool, runE func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// prevent Cobra help/usage on cancel
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		ctx := cmd.Context()

		errCh := make(chan error, 1)
		go func() {
			errCh <- runE(cmd, args)
		}()

		select {
		case <-ctx.Done():
			if ignoreErrors {
				return nil
			} else {
				return ctx.Err()
			}
		case err := <-errCh:
			return err
		}
	}
}
