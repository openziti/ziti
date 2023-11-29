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

package zac

import (
	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/openziti/xweb/v2"
	"net/http"
	"strings"
)

const (
	Binding = "zac"
)

type ZitiAdminConsoleFactory struct {
	healthChecker gosundheit.Health
}

var _ xweb.ApiHandlerFactory = &ZitiAdminConsoleFactory{}

func NewZitiAdminConsoleFactory() *ZitiAdminConsoleFactory {
	return &ZitiAdminConsoleFactory{}
}

func (factory ZitiAdminConsoleFactory) Validate(*xweb.InstanceConfig) error {
	return nil
}

func (factory ZitiAdminConsoleFactory) Binding() string {
	return Binding
}

func (factory ZitiAdminConsoleFactory) New(_ *xweb.ServerConfig, options map[interface{}]interface{}) (xweb.ApiHandler, error) {
	loc := "./"
	if options["location"] != "" {
		loc = options["location"].(string)
	}
	zac := &ZitiAdminConsoleHandler{
		httpHandler: http.FileServer(http.Dir(loc)),
	}

	return zac, nil
}

type ZitiAdminConsoleHandler struct {
	options     map[interface{}]interface{}
	httpHandler http.Handler
}

func (self *ZitiAdminConsoleHandler) Binding() string {
	return Binding
}

func (self *ZitiAdminConsoleHandler) Options() map[interface{}]interface{} {
	return nil
}

func (self *ZitiAdminConsoleHandler) RootPath() string {
	return "/" + Binding
}

func (self *ZitiAdminConsoleHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, self.RootPath()) || strings.HasPrefix(r.URL.Path, "/assets")
}

func (self *ZitiAdminConsoleHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasPrefix(request.URL.Path, self.RootPath()) {
		request.URL.Path = self.RootPath() + "/" + request.URL.Path
	}
	self.httpHandler.ServeHTTP(writer, request)
}
