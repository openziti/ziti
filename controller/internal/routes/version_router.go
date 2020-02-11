/*
	Copyright 2019 Netfoundry, Inc.

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

package routes

import (
	"github.com/Jeffail/gabs"
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-edge/build"
	"github.com/netfoundry/ziti-edge/controller/env"
	"github.com/netfoundry/ziti-edge/controller/internal/permissions"
	"github.com/netfoundry/ziti-edge/controller/response"
	"runtime"
)

func init() {
	r := NewVersionRouter()
	env.AddRouter(r)
}

type VersionRouter struct {
	BasePath string
}

func NewVersionRouter() *VersionRouter {
	return &VersionRouter{
		BasePath: "/version",
	}
}

func (ir *VersionRouter) Register(ae *env.AppEnv) {

	listHandler := ae.WrapHandler(ir.List, permissions.Always())

	ae.RootRouter.HandleFunc(ir.BasePath, listHandler).Methods("GET")
	ae.RootRouter.HandleFunc(ir.BasePath+"/", listHandler).Methods("GET")
}

func (ir *VersionRouter) List(ae *env.AppEnv, rc *response.RequestContext) {
	data := gabs.New()
	buildInfo := build.GetBuildInfo()
	if _, err := data.SetP(buildInfo.GetVersion(), "version"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := data.SetP(buildInfo.GetRevision(), "revision"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := data.SetP(buildInfo.GetBuildDate(), "buildDate"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	if _, err := data.SetP(runtime.Version(), "runtimeVersion"); err != nil {
		pfxlog.Logger().WithField("cause", err).Panic("could not set value by path")
	}

	rc.RequestResponder.RespondWithOk(data.Data(), nil)
}
