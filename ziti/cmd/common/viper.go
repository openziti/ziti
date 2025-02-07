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
	c "github.com/openziti/ziti/ziti/constants"
	"github.com/spf13/viper"
	"strings"
)

func NewViper() *viper.Viper {
	result := viper.New()
	result.SetEnvPrefix(c.ZITI) // All env vars we seek will be prefixed with "ZITI_"
	result.AutomaticEnv()
	replacer := strings.NewReplacer("-", "_") // We use underscores in env var names, but use dashes in flag names
	result.SetEnvKeyReplacer(replacer)
	return result
}
