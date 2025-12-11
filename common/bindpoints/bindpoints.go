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
	"github.com/openziti/xweb/v3"
)

// BindPointListenerFactory implements the xweb.BindPointListenerFactory.
// It provides a factory that generates xweb.BindPoints based on the provided config section
type BindPointListenerFactory struct {
}

// New checks to see if this BindPoint is for overlay or underlay then calls the expected func.
// As of now there are only two types, OverlayBindPoint and UnderlayBindPoint
func (c *BindPointListenerFactory) New(conf map[interface{}]interface{}) (xweb.BindPoint, error) {
	if conf["identity"] != nil {
		return newOverlayBindPoint(conf)
	} else { // only two options right now. underlay and overlay...
		return newUnderlayBindPoint(conf, false)
	}
}

// LegacyBindPointListenerFactory implements the xweb.LegacyBindPointListenerFactory.
// It provides a factory that generates xweb.LegacyBindPoints based on the provided config section. It also
// allows for "legacy" validation, specifically with respect to web.bindPoints.addresses where 0.0.0.0 was previously
// considered a valid address.
type LegacyBindPointListenerFactory struct {
}

// New checks to see if this LegacyBindPoint is for overlay or underlay then calls the expected func.
// As of now there are only two types, OverlayLegacyBindPoint and UnderlayLegacyBindPoint
func (c *LegacyBindPointListenerFactory) New(conf map[interface{}]interface{}) (xweb.BindPoint, error) {
	if conf["identity"] != nil {
		return newOverlayBindPoint(conf)
	} else { // only two options right now. underlay and overlay...
		return newUnderlayBindPoint(conf, true)
	}
}
