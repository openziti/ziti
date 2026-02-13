//go:build dataflow

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

package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/ziti/v2/controller/xt_smartrouting"
)

func Test_DialContext_AlreadyCancelled(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	ctx.CreateEnrollAndStartEdgeRouter()

	// Set up a listener with ManualStart so there's a terminator registered, but
	// connections will block until CompleteAcceptSuccess is called.
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener, err := hostContext.ListenWithOptions(service.Name, &ziti.ListenOptions{
		ManualStart: true,
	})
	ctx.Req.NoError(err)
	defer listener.Close()

	// Accept and complete connections in the background so the terminator stays healthy
	go func() {
		for {
			conn, err := listener.AcceptEdge()
			if err != nil {
				return
			}
			_ = conn.CompleteAcceptSuccess()
			_ = conn.Close()
		}
	}()

	_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	// Create an already-cancelled context
	dialCtx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err = clientContext.DialContext(dialCtx, service.Name)
	elapsed := time.Since(start)

	ctx.Req.Error(err, "expected error from DialContext with cancelled context")
	ctx.Req.True(errors.Is(err, context.Canceled), "expected context.Canceled, got: %v", err)
	ctx.Req.Less(elapsed, 2*time.Second, "DialContext with cancelled context should return promptly")
}

func Test_DialContext_CancelMidDial(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	ctx.CreateEnrollAndStartEdgeRouter()

	// Set up a listener with ManualStart that accepts connections but never completes them,
	// causing the dialer to block in the connect handshake.
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener, err := hostContext.ListenWithOptions(service.Name, &ziti.ListenOptions{
		ManualStart: true,
	})
	ctx.Req.NoError(err)
	defer listener.Close()

	// Accept connections but never call CompleteAcceptSuccess — the dial will block
	go func() {
		for {
			conn, err := listener.AcceptEdge()
			if err != nil {
				return
			}
			// Hold the connection open without completing the accept.
			// Close it after a long delay so resources are cleaned up.
			go func() {
				time.Sleep(30 * time.Second)
				_ = conn.Close()
			}()
		}
	}()

	_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	// Use a long timeout so we know cancellation is what stops the dial
	dialCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, dialErr := clientContext.DialContextWithOptions(dialCtx, service.Name, &ziti.DialOptions{
			ConnectTimeout: 30 * time.Second,
		})
		errCh <- dialErr
	}()

	// Cancel after a short delay to give the dial time to reach the connect handshake
	time.Sleep(500 * time.Millisecond)
	cancel()

	select {
	case dialErr := <-errCh:
		ctx.Req.Error(dialErr, "expected error from DialContext after cancellation")
		ctx.Req.True(errors.Is(dialErr, context.Canceled), "expected context.Canceled, got: %v", dialErr)
	case <-time.After(5 * time.Second):
		t.Fatal("DialContext did not return within 5 seconds after cancellation")
	}
}

func Test_DialContext_ShortDeadline(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	service := ctx.AdminManagementSession.RequireNewServiceAccessibleToAll(xt_smartrouting.Name)

	ctx.CreateEnrollAndStartEdgeRouter()

	// Set up a listener with ManualStart that accepts connections but never completes them,
	// so the dial blocks in the connect handshake.
	_, hostContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer hostContext.Close()

	listener, err := hostContext.ListenWithOptions(service.Name, &ziti.ListenOptions{
		ManualStart: true,
	})
	ctx.Req.NoError(err)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.AcceptEdge()
			if err != nil {
				return
			}
			go func() {
				time.Sleep(30 * time.Second)
				_ = conn.Close()
			}()
		}
	}()

	_, clientContext := ctx.AdminManagementSession.RequireCreateSdkContext()
	defer clientContext.Close()

	// Use a short context deadline that should take precedence over the long ConnectTimeout
	dialCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = clientContext.DialContextWithOptions(dialCtx, service.Name, &ziti.DialOptions{
		ConnectTimeout: 30 * time.Second,
	})
	elapsed := time.Since(start)

	pfxlog.Logger().Infof("DialContext with short deadline returned in %v with error: %v", elapsed, err)

	ctx.Req.Error(err, "expected error from DialContext with short deadline")
	ctx.Req.True(errors.Is(err, context.DeadlineExceeded), "expected context.DeadlineExceeded, got: %v", err)
	ctx.Req.Less(elapsed, 3*time.Second, "DialContext should respect context deadline, not ConnectTimeout")
}
