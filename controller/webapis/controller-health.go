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
	"fmt"
	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/openziti/xweb/v3"
	"github.com/openziti/ziti/controller/env"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"time"
)

var _ xweb.ApiHandlerFactory = &ControllerHealthCheckApiFactory{}

type ControllerHealthCheckApiFactory struct {
	appEnv        *env.AppEnv
	healthChecker gosundheit.Health
}

func (factory ControllerHealthCheckApiFactory) Validate(config *xweb.InstanceConfig) error {
	return nil
}

func NewControllerHealthCheckApiFactory(appEnv *env.AppEnv, healthChecker gosundheit.Health) *ControllerHealthCheckApiFactory {
	return &ControllerHealthCheckApiFactory{
		appEnv:        appEnv,
		healthChecker: healthChecker,
	}
}

func (factory ControllerHealthCheckApiFactory) Binding() string {
	return ControllerHealthCheckApiBinding
}

func (factory ControllerHealthCheckApiFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	healthCheckApiHandler, err := NewControllerHealthCheckApiHandler(factory.healthChecker, factory.appEnv, options)

	if err != nil {
		return nil, err
	}

	return healthCheckApiHandler, nil

}

func NewControllerHealthCheckApiHandler(healthChecker gosundheit.Health, appEnv *env.AppEnv, options map[interface{}]interface{}) (*ControllerHealthCheckApiHandler, error) {
	healthCheckApi := &ControllerHealthCheckApiHandler{
		healthChecker: healthChecker,
		appEnv:        appEnv,
		options:       options,
	}

	return healthCheckApi, nil

}

type ControllerHealthCheckApiHandler struct {
	options       map[interface{}]interface{}
	appEnv        *env.AppEnv
	healthChecker gosundheit.Health
}

func (self ControllerHealthCheckApiHandler) Binding() string {
	return ControllerHealthCheckApiBinding
}

func (self ControllerHealthCheckApiHandler) Options() map[interface{}]interface{} {
	return self.options
}

func (self ControllerHealthCheckApiHandler) RootPath() string {
	return "/health-checks"
}

func (self ControllerHealthCheckApiHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, self.RootPath())
}

func (self *ControllerHealthCheckApiHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	output := map[string]interface{}{}
	output["meta"] = map[string]interface{}{}

	data := map[string]interface{}{}
	output["data"] = data

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")

	results, healthy := self.healthChecker.Results()
	data["healthy"] = healthy
	var checks []map[string]interface{}
	shortFormat := request.URL.Query().Get("type") == "short"

	for id, result := range results {
		check := map[string]interface{}{}
		checks = append(checks, check)
		check["id"] = id
		check["healthy"] = result.IsHealthy()
		if !shortFormat {
			check["lastCheckDuration"] = fmt.Sprintf("%v", result.Duration)
			check["lastCheckTime"] = result.Timestamp.UTC().Format(time.RFC3339)

			if result.Error != nil {
				check["err"] = result.Error
				check["consecutiveFailures"] = result.ContiguousFailures
			}

			if result.TimeOfFirstFailure != nil {
				check["failingSince"] = result.TimeOfFirstFailure.UTC().Format(time.RFC3339)
			}
			if result.Details != "didn't run yet" {
				check["details"] = result.Details
			}
		}
	}
	data["checks"] = checks

	if strings.HasSuffix(request.URL.Path, "/controller/raft") {
		isRaftEnabled := self.appEnv.GetHostController().IsRaftEnabled()
		isLeader := self.appEnv.GetHostController().IsRaftLeader()

		if !isLeader && isRaftEnabled {
			// this is uses 429 to be consistent with Vault. 503 seems like it would be more
			// appropriate, but it just needs to be not 200, so using 429 for consistency
			w.WriteHeader(429)
		}

		raftData := map[string]interface{}{}
		raftData["isRaftEnabled"] = isRaftEnabled
		raftData["isLeader"] = isLeader
		output["raft"] = raftData
	}

	if err := encoder.Encode(output); err != nil {
		logrus.WithError(err).Error("failure encoding health check results")
	}
}

func (self ControllerHealthCheckApiHandler) IsDefault() bool {
	return false
}
