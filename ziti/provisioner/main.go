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

package provisioner

import (
	"fmt"
)

// Provisioner settings
type Options struct {
	Provisioner string
	Environment string
}

// Up instantiates an environment
func Up(po *Options) error {
	if po.Provisioner == "vagrant" {
		return VagrantUp(po)
	}
	return fmt.Errorf("%s is not a valid provisioner choice", po.Provisioner)
}
