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

package host

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/tunnel/dns"
	"github.com/openziti/edge/tunnel/entities"
	"github.com/openziti/edge/tunnel/intercept"
	"github.com/pkg/errors"
)

type interceptor struct{}

func New() intercept.Interceptor {
	return &interceptor{}
}

func (p interceptor) Intercept(*entities.Service, dns.Resolver, intercept.AddressTracker) error {
	return errors.New("can not intercept services in host mode")
}

func (p interceptor) Stop() {
	pfxlog.Logger().Info("stopping host interceptor")
}

func (p interceptor) StopIntercepting(string, intercept.AddressTracker) error {
	return errors.New("StopIntercepting not implemented by host interceptor")
}
