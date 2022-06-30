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
	"github.com/fatih/color"
	"net"
	"net/http"
)

type plainEchoServer struct {
	Port     int
	listener net.Listener
}

func (s *plainEchoServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	input := r.URL.Query().Get("input")
	result := fmt.Sprintf("As you say, '%v', indeed!\n", input)
	c := color.New(color.FgBlue, color.Bold)
	c.Print("\nplain-http-echo-server: ")
	fmt.Printf("received input '%v'\n", input)
	if _, err := rw.Write([]byte(result)); err != nil {
		panic(err)
	}
}

func (s *plainEchoServer) run() (err error) {
	bindAddr := fmt.Sprintf("127.0.0.1:%v", s.Port)
	s.listener, err = net.Listen("tcp", bindAddr)
	if err != nil {
		return err
	}

	addr := s.listener.Addr().(*net.TCPAddr)
	s.Port = addr.Port

	c := color.New(color.FgBlue, color.Bold)
	c.Print("\nplain-http-echo-server: ")
	fmt.Printf("listening on %v\n", addr)
	go func() { _ = http.Serve(s.listener, http.HandlerFunc(s.ServeHTTP)) }()
	return nil
}

func (s *plainEchoServer) stop() error {
	return s.listener.Close()
}
