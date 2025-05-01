/*
	Copyright 2019 NetFoundry Inc.

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

package xgress_geneve

import (
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/router/xgress_router"
	"github.com/pkg/errors"
)

type Factory struct{}

func (f Factory) CreateListener(optionsData xgress.OptionsData) (xgress_router.Listener, error) {
	return &listener{}, nil
}

func (f Factory) CreateDialer(optionsData xgress.OptionsData) (xgress_router.Dialer, error) {
	return nil, errors.New("dialer not supported")
}
