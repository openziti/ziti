// +build apitests

/*
	Copyright NetFoundry, Inc.

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

package tests

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/netfoundry/ziti-sdk-golang/ziti"
	"github.com/pkg/errors"
	"net"
	"testing"
	"time"
)

func Test_Dataflow(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.teardown()
	ctx.startServer()
	ctx.requireAdminLogin()

	ctx.AdminSession.requireNewServicePolicy("Dial", s("#all"), s("#all"))
	ctx.AdminSession.requireNewServicePolicy("Bind", s("#all"), s("#all"))
	ctx.AdminSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	service := ctx.AdminSession.requireNewService(nil, nil)
	fmt.Printf("service id: %v\n", service.id)

	ctx.createEnrollAndStartEdgeRouter()
	hostIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	hostConfig := ctx.enrollIdentity(hostIdentity.id)
	hostContext := ziti.NewContextWithConfig(hostConfig)
	listener, err := hostContext.Listen(service.name)
	ctx.req.NoError(err)

	errorC := make(chan error)
	go helloServerWrapper(listener, errorC)

	clientIdentity := ctx.AdminSession.requireNewIdentityWithOtt(false)
	clientConfig := ctx.enrollIdentity(clientIdentity.id)
	clientContext := ziti.NewContextWithConfig(clientConfig)
	conn, err := clientContext.Dial(service.name)
	ctx.req.NoError(err)

	name := uuid.New().String()
	n, err := conn.Write([]byte(name))
	ctx.req.NoError(err)
	ctx.req.Equal(n, len([]byte(name)))

	expected := "hello, " + name
	buf := make([]byte, 1024)
	n, err = conn.Read(buf)
	ctx.req.NoError(err)
	ctx.req.Equal(n, len([]byte(expected)))
	ctx.req.Equal(expected, string(buf[:n]))

	n, err = conn.Write([]byte("quit"))
	ctx.req.NoError(err)
	ctx.req.Equal(n, len([]byte("quit")))

	expected = "ok"
	buf = make([]byte, 1024)
	n, err = conn.Read(buf)
	ctx.req.NoError(err)
	ctx.req.Equal(n, len([]byte(expected)))
	ctx.req.Equal(expected, string(buf[:n]))

	processing := false
	for processing {
		select {
		case err, processing = <-errorC:
			if processing {
				ctx.req.NoError(err)
			}
		case <-time.After(5 * time.Second):
			ctx.req.Fail("timed out after 5 seconds")
		}
	}
}

func helloServerWrapper(listener net.Listener, errorC chan error) {
	err := helloServer(listener)
	if err != nil {
		errorC <- err
	}
	close(errorC)
	fmt.Print("service exiting")
}

func helloServer(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}

	for {
		buf := make([]byte, 1024)
		bytes, err := conn.Read(buf)
		if err != nil {
			return err
		}
		name := string(buf[:bytes])
		fmt.Printf("received '%v' from client\n", name)
		result := "hello, " + name
		if name == "quit" {
			result = "ok"
		}

		fmt.Printf("returning '%v' to client\n", result)
		bytes, err = conn.Write([]byte(result))
		if err != nil {
			return err
		}
		if bytes != len([]byte(result)) {
			return errors.Errorf("server expected to write %v bytes, but only wrote %v", len([]byte(result)), bytes)
		}
		if name == "quit" {
			fmt.Print("quitting")
			return listener.Close()
		}
	}
}
