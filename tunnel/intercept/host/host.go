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

package host

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/tunnel/dns"
	"github.com/openziti/ziti/tunnel/entities"
	"github.com/openziti/ziti/tunnel/intercept"
	"github.com/pkg/errors"
)

type interceptor struct{}

func New() intercept.Interceptor {
	return &interceptor{}
}

func (p interceptor) Intercept(svc *entities.Service, _ dns.Resolver, _ intercept.AddressTracker) error {
	// only return error if an intercept has been requested
	if svc.InterceptV1Config != nil {
		return errors.New("can not intercept services in host mode")
	}
	return nil
}

func (p interceptor) Stop() {
	pfxlog.Logger().Info("stopping host interceptor")
}

func (p interceptor) StopIntercepting(string, intercept.AddressTracker) error {
	// host mode interceptor can't intercept services, so there's nothing to do here
	return nil
}
