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

package console

import (
	"embed"
	"github.com/openziti/fablab/kernel/model"
	"io/fs"
	"net/http"
)

//go:embed webroot
var webroot embed.FS

func Console() model.Action {
	return &console{}
}

func (consoleAction *console) Execute(m *model.Model) error {
	server := NewServer()
	go server.Listen()

	unprefixedWebroot, err := fs.Sub(webroot, "webroot")
	if err != nil {
		return err
	}
	http.Handle("/", http.FileServer(http.FS(unprefixedWebroot)))
	return http.ListenAndServe(":8080", nil)
}

type console struct{}
