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

package ops

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openziti/identity"
	"github.com/spf13/cobra"
)

type IdentityConfigFile struct {
	ZtAPI       string          `json:"ztAPI"`
	ID          identity.Config `json:"id"`
	ConfigTypes []string        `json:"configTypes"`
}

func NewUnwrapIdentityFileCommand(out io.Writer, errOut io.Writer) *cobra.Command {
	outCertFile := ""
	outKeyFile := ""
	outCaFile := ""

	cmd := &cobra.Command{
		Use:   "unwrap <identity_file>",
		Short: "unwrap a Ziti Identity file into its separate pieces (supports PEM only)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			identityFile := args[0]

			rootFileName := strings.TrimSuffix(identityFile, ".json")

			if outCertFile == "" {
				outCertFile = rootFileName + ".cert"
			}

			if outKeyFile == "" {
				outKeyFile = rootFileName + ".key"
			}

			if outCaFile == "" {
				outCaFile = rootFileName + ".ca"
			}

			identityJson, err := os.ReadFile(identityFile)

			if err != nil {
				return fmt.Errorf("error opening file %s: %w", args[0], err)
			}

			config := &IdentityConfigFile{}
			if err := json.Unmarshal(identityJson, config); err != nil {
				return fmt.Errorf("error unmarshaling identity config JSON: %w", err)
			}

			if strings.HasPrefix(config.ID.Cert, "pem:") {
				data := strings.TrimPrefix(config.ID.Cert, "pem:")
				if err := os.WriteFile(outCertFile, []byte(data), getFileMode(false)); err != nil {
					return fmt.Errorf("error writing certificate to file [%s]: %w", outCertFile, err)
				}
			} else {
				return fmt.Errorf("error writing certificate to file [%s]: missing pem prefix, type is unsupported", outCertFile)
			}

			if strings.HasPrefix(config.ID.Key, "pem:") {
				data := strings.TrimPrefix(config.ID.Key, "pem:")
				if err := os.WriteFile(outKeyFile, []byte(data), getFileMode(true)); err != nil {
					return fmt.Errorf("error writing private key to file [%s]: %w", outKeyFile, err)
				}
			} else {
				_, _ = fmt.Fprintf(errOut, "error writing private key to file [%s]: missing pem prefix, type is unsupported\n", outKeyFile)
			}

			if strings.HasPrefix(config.ID.CA, "pem:") {
				data := strings.TrimPrefix(config.ID.CA, "pem:")
				if err := os.WriteFile(outCaFile, []byte(data), getFileMode(false)); err != nil {
					return fmt.Errorf("error writing CAs to file [%s]: %w", outCaFile, err)
				}
			} else {
				return fmt.Errorf("error writing CAs to file [%s]: missing pem prefix, type is unsupported", outCaFile)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outCertFile, "cert", "", "", "output certificate file, defaults to ./<root>.cert")
	cmd.Flags().StringVarP(&outKeyFile, "key", "", "", "output private key file, defaults to ./<root>.key")
	cmd.Flags().StringVarP(&outCaFile, "ca", "", "", "output ca bundle file, defaults to ./<root>.ca")

	return cmd
}
