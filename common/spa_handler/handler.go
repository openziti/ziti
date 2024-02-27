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

package spa_handler

import (
	"github.com/openziti/xweb/v2"
	log "github.com/sirupsen/logrus"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	Binding = "zac"
)

type ZitiAdminConsoleFactory struct {
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
	loc := options["location"]
	if loc == nil || loc == "" {
		log.Fatal("location must be supplied in zac options")
	}
	indexFile := options["indexFile"]
	if indexFile == nil || indexFile == "" {
		indexFile = "index.html"
	}
	zac := &SPAHTTPHandler{
		httpHandler: SpaHandler(loc.(string), "/"+Binding, indexFile.(string)),
	}

	return zac, nil
}

type SPAHTTPHandler struct {
	httpHandler http.Handler
}

func (self *SPAHTTPHandler) Binding() string {
	return Binding
}

func (self *SPAHTTPHandler) Options() map[interface{}]interface{} {
	return nil
}

func (self *SPAHTTPHandler) RootPath() string {
	return "/" + Binding
}

func (self *SPAHTTPHandler) IsHandler(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, self.RootPath()) || strings.HasPrefix(r.URL.Path, "/assets")
}

func (self *SPAHTTPHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	self.httpHandler.ServeHTTP(writer, request)
}

// Thanks to https://github.com/roberthodgen/spa-server
// Serve from a public directory with specific index
type spaHandler struct {
	content     string // The directory from which to serve
	contextRoot string // The context root to remove
	indexFile   string // The fallback/default file to serve
}

// Falls back to a supplied index (indexFile) when either condition is true:
// (1) Request (file) path is not found
// (2) Request path is a directory
// Otherwise serves the requested file.
func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Debugf("incoming r.URL.Path: %s", r.URL.Path)
	r.URL.Path = strings.TrimPrefix(r.URL.Path, h.contextRoot)
	p := filepath.Join(h.content, filepath.Clean(r.URL.Path))
	log.Debugf("outgoing r.URL.Path: %s", p)

	if info, err := os.Stat(p); err != nil {
		http.ServeFile(w, r, filepath.Join(h.content, h.indexFile))
		return
	} else if info.IsDir() {
		http.ServeFile(w, r, filepath.Join(h.content, h.indexFile))
		return
	}

	http.ServeFile(w, r, p)
}

// Returns a request handler (http.Handler) that serves a single
// page application from a given public directory (location).
func SpaHandler(location string, contextRoot string, indexFile string) http.Handler {
	return &spaHandler{location, contextRoot, indexFile}
}
