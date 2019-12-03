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

package main

import (
	"github.com/netfoundry/ziti-edge/sdk/ziti"
	"fmt"
	"net"
	"net/http"
	"os"
)

type Greeter string

func (g Greeter) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	var result string
	if name := req.URL.Query().Get("name"); name != "" {
		result = fmt.Sprintf("Hello, %v, from %v\n", name, g)
	} else {
		result = "Who are you?\n"
	}
	if _, err := resp.Write([]byte(result)); err != nil {
		panic(err)
	}
}

func serve(listener net.Listener, serverType string) {
	if err := http.Serve(listener, Greeter(serverType)); err != nil {
		panic(err)
	}
}

func plain(listenAddr string) {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		panic(err)
	}
	serve(listener, "plain")
}

func withZiti(service string) {
	listener, err := ziti.NewContext().Listen(service)
	if err != nil {
		panic(err)
	}
	serve(listener, "ziti")
}

func main() {
	if len(os.Args) > 2 && os.Args[1] == "ziti" {
		withZiti(os.Args[2])
	} else {
		plain("localhost:8080")
	}
}
