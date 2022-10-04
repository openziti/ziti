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

package tutorial

import (
	"fmt"
	"net"
	"net/http"

	"github.com/fatih/color"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
)

type zitiEchoServer struct {
	identityJson string
	listener     net.Listener
}

func (s *zitiEchoServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	result := fmt.Sprintf("As you say, '%v', indeed!\n", input)
	c := color.New(color.FgGreen, color.Bold)
	c.Print("\nziti-http-echo-server: ")
	fmt.Printf("received input '%v'\n", input)
	if _, err := rw.Write([]byte(result)); err != nil {
		panic(err)
	}
}

func (s *zitiEchoServer) run() (err error) {
	config, err := config.NewFromFile(s.identityJson)
	if err != nil {
		return err
	}

	zitiContext := ziti.NewContextWithConfig(config)
	if s.listener, err = zitiContext.Listen("echo"); err != nil {
		return err
	}

	c := color.New(color.FgGreen, color.Bold)
	c.Print("\nziti-http-echo-server: ")
	fmt.Println("listening for connections from echo server")
	go func() { _ = http.Serve(s.listener, http.HandlerFunc(s.ServeHTTP)) }()
	return nil
}

func (s *zitiEchoServer) stop() error {
	return s.listener.Close()
}
