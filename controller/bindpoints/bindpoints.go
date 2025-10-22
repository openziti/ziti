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

package bindpoints

import (
	"github.com/openziti/xweb/v2"
)

// BindPointListenerFactory implements the xweb.BindPointListenerFactory.
// It provides a factory that generates xweb.BindPoints
type BindPointListenerFactory struct {
}

func (c *BindPointListenerFactory) New(conf map[interface{}]interface{}) (xweb.BindPoint, error) {
	if conf["identity"] != nil {
		return newOverlayBindPoint(conf)
	} else { // only two options right now. underlay and overlay...
		return newUnderlayBindPoint(conf)
	}
}
