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

package webapis

import (
	"encoding/json"
	"github.com/openziti/xweb/v2"
	"github.com/openziti/ziti/controller/env"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

var _ xweb.ApiHandlerFactory = &ControllerIsLeaderApiFactory{}

type ControllerIsLeaderApiFactory struct {
	appEnv *env.AppEnv
}

func (factory ControllerIsLeaderApiFactory) Validate(config *xweb.InstanceConfig) error {
	return nil
}

func NewControllerIsLeaderApiFactory(appEnv *env.AppEnv) *ControllerIsLeaderApiFactory {
	return &ControllerIsLeaderApiFactory{
		appEnv: appEnv,
	}
}

func (factory ControllerIsLeaderApiFactory) Binding() string {
	return ControllerIsLeaderApiBinding
}

func (factory ControllerIsLeaderApiFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	return &ControllerIsLeaderApiHandler{
		appEnv:  factory.appEnv,
		options: options,
	}, nil

}

type ControllerIsLeaderApiHandler struct {
	handler http.Handler
	options map[interface{}]interface{}
	appEnv  *env.AppEnv
}

func (self ControllerIsLeaderApiHandler) Binding() string {
	return ControllerIsLeaderApiBinding
}

func (self ControllerIsLeaderApiHandler) Options() map[interface{}]interface{} {
	return self.options
}

func (self ControllerIsLeaderApiHandler) RootPath() string {
	return "/sys/health"
}

func (self ControllerIsLeaderApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, self.RootPath())
}

func (self *ControllerIsLeaderApiHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	output := map[string]interface{}{}
	output["meta"] = map[string]interface{}{}

	data := map[string]interface{}{}
	output["data"] = data

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")

	isLeader := self.appEnv.GetHostController().IsRaftLeader()
	data["isLeader"] = isLeader
	if !isLeader {
		w.WriteHeader(429)
	}

	if err := encoder.Encode(output); err != nil {
		logrus.WithError(err).Error("failure encoding health check results")
	}
}

func (self ControllerIsLeaderApiHandler) IsDefault() bool {
	return false
}
