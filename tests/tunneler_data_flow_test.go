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
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/pkg/errors"
	"io"
	"net"
	"testing"
	"time"
)

func Test_TunnelerDataflow(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	ctx.AdminSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)
	ctx.AdminSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)
	ctx.AdminSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	hostConfig := ctx.newConfig("cea49285-6c07-42cf-9f52-09a9b115c783", map[string]interface{}{
		"hostname": "localhost",
		"port":     8687,
		"protocol": "tcp",
	})
	hostConfig.Name = "tunnel-host"
	ctx.AdminSession.requireCreateEntity(hostConfig)

	service := ctx.AdminSession.testContext.newService(nil, s(hostConfig.Id))
	service.Name = "tunnel-test"
	ctx.AdminSession.requireCreateEntity(service)

	ctx.CreateEnrollAndStartTunnelerEdgeRouter()
	l, err := net.Listen("tcp", "localhost:8687")
	ctx.Req.NoError(err)

	time.Sleep(time.Second)

	errC := make(chan error, 10)
	go acceptConnections(l, errC)

	conn, err := net.Dial("tcp", "localhost:8686")
	ctx.Req.NoError(err)

	_, err = conn.Write([]byte("hello"))
	ctx.Req.NoError(err)
	fmt.Println("client sent: 'hello'")

	var read []string
	for {
		buf := make([]byte, 1024)
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil && errors.Is(err, io.EOF) {
			if n > 0 {
				fmt.Printf("server responded: '%v'\n", string(buf[:n]))
				read = append(read, string(buf[:n]))
			}
			break
		}
		ctx.Req.NoError(err)
		fmt.Printf("server responded: '%v'\n", string(buf[:n]))
		read = append(read, string(buf[:n]))
	}

	ctx.Req.Equal(1, len(read))
	if len(read) > 0 { // goland complaining about potential nil slice
		ctx.Req.Equal("goodbye", read[0])
	}

	var errs []error
	done := false
	for !done {
		select {
		case err, ok := <-errC:
			if !ok {
				done = true
				break
			}
			fmt.Printf("error: %v\n", err)
			errs = append(errs, err)
		case <-time.After(1 * time.Second):
			ctx.Req.Fail("timed out waiting for errors")
		}
	}
	ctx.Req.Equal(0, len(errs))
}

func acceptConnections(l net.Listener, errC chan error) {
	defer func() { _ = l.Close() }()
	defer close(errC)

	conn, err := l.Accept()
	if err != nil {
		return
	}

	_ = conn.SetDeadline(time.Now().Add(time.Second))
	handleServerConn(conn, errC)
}

func handleServerConn(conn net.Conn, errC chan error) {
	var read []string

	defer func() {
		if len(read) != 1 {
			errC <- errors.Errorf("server expected on read result, got %+v", read)
		} else if read[0] != "hello" {
			errC <- errors.Errorf("server expected on read result of \"hello\", got %+v", read)
		}
	}()

	buf := make([]byte, len([]byte("hello")))
	n, err := conn.Read(buf)
	if err != nil {
		errC <- err
		return
	}

	fmt.Printf("client said: '%v'\n", string(buf[:n]))
	read = append(read, string(buf[:n]))

	if _, err = conn.Write([]byte("goodbye")); err != nil {
		errC <- err
		return
	}
	fmt.Println("server sent: goodbye")

	if err = conn.Close(); err != nil {
		errC <- err
	}
}

func Test_TunnelerDataflowHalfClose(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminLogin()

	ctx.AdminSession.requireNewServicePolicy("Dial", s("#all"), s("#all"), nil)
	ctx.AdminSession.requireNewServicePolicy("Bind", s("#all"), s("#all"), nil)
	ctx.AdminSession.requireNewEdgeRouterPolicy(s("#all"), s("#all"))
	ctx.AdminSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#all"))

	hostConfig := ctx.newConfig("cea49285-6c07-42cf-9f52-09a9b115c783", map[string]interface{}{
		"hostname": "localhost",
		"port":     8689,
		"protocol": "tcp",
	})
	hostConfig.Name = "tunnel-host"
	ctx.AdminSession.requireCreateEntity(hostConfig)

	service := ctx.AdminSession.testContext.newService(nil, s(hostConfig.Id))
	service.Name = "tunnel-test"
	ctx.AdminSession.requireCreateEntity(service)

	ctx.CreateEnrollAndStartTunnelerEdgeRouter()

	l, err := net.Listen("tcp", "localhost:8689")
	ctx.Req.NoError(err)

	time.Sleep(time.Second)

	errC := make(chan error, 10)
	go acceptConnectionsHalfClose(l, errC)

	conn, err := net.Dial("tcp", "localhost:8686")
	ctx.Req.NoError(err)

	_, err = conn.Write([]byte("hello"))
	ctx.Req.NoError(err)
	fmt.Println("client sent: 'hello'")

	ctx.Req.NoError(conn.(edge.CloseWriter).CloseWrite())

	var read []string
	for {
		buf := make([]byte, 1024)
		_ = conn.SetDeadline(time.Now().Add(time.Second))
		n, err := conn.Read(buf)
		if err != nil && errors.Is(err, io.EOF) {
			if n > 0 {
				fmt.Printf("server responded: '%v'\n", string(buf[:n]))
				read = append(read, string(buf[:n]))
			}
			break
		}
		ctx.Req.NoError(err)
		fmt.Printf("server responded: '%v'\n", string(buf[:n]))
		read = append(read, string(buf[:n]))
	}

	ctx.Req.Equal(1, len(read))
	if len(read) > 0 { // goland complaining about potential nil slice
		ctx.Req.Equal("goodbye", read[0])
	}

	var errs []error
	done := false
	for !done {
		select {
		case err, ok := <-errC:
			if !ok {
				done = true
				break
			}
			fmt.Printf("error: %v\n", err)
			errs = append(errs, err)
		case <-time.After(1 * time.Second):
			ctx.Req.Fail("timed out waiting for errors")
		}
	}
	ctx.Req.Equal(0, len(errs))
}

func acceptConnectionsHalfClose(l net.Listener, errC chan error) {
	defer func() { _ = l.Close() }()
	defer close(errC)

	conn, err := l.Accept()
	if err != nil {
		return
	}
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	handleServerConnHalfClose(conn, errC)
}

func handleServerConnHalfClose(conn net.Conn, errC chan error) {
	var read []string

	defer func() {
		if len(read) != 1 {
			errC <- errors.Errorf("server expected on read result, got %+v", read)
		} else if read[0] != "hello" {
			errC <- errors.Errorf("server expected on read result of \"hello\", got %+v", read)
		}
	}()

	for {
		buf := make([]byte, len([]byte("hello")))
		n, err := conn.Read(buf)
		if err != nil && errors.Is(err, io.EOF) {
			if n > 0 {
				read = append(read, string(buf[:n]))
				fmt.Printf("client said: '%v'\n", string(buf[:n]))
			}
			break
		}
		if err != nil {
			errC <- err
			return
		}
		fmt.Printf("client said: '%v'\n", string(buf[:n]))
		read = append(read, string(buf[:n]))
	}

	_, err := conn.Write([]byte("goodbye"))
	if err != nil {
		errC <- err
		return
	}
	fmt.Println("server sent: goodbye")

	if err = conn.(edge.CloseWriter).CloseWrite(); err != nil {
		errC <- err
	}
}
