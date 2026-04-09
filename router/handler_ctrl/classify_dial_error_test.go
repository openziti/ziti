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

package handler_ctrl

import (
	"fmt"
	"net"
	"syscall"
	"testing"

	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func Test_classifyDialError(t *testing.T) {
	t.Run("connection refused", func(t *testing.T) {
		err := fmt.Errorf("dial failed: %w", syscall.ECONNREFUSED)
		require.Equal(t, byte(ctrl_msg.ErrorTypeConnectionRefused), classifyDialError(err))
	})

	t.Run("DNS resolution failure", func(t *testing.T) {
		err := &net.DNSError{Err: "no such host", Name: "nonexistent.example.com", IsNotFound: true}
		require.Equal(t, byte(ctrl_msg.ErrorTypeDnsResolutionFailed), classifyDialError(err))
	})

	t.Run("wrapped DNS resolution failure", func(t *testing.T) {
		dnsErr := &net.DNSError{Err: "no such host", Name: "nonexistent.example.com", IsNotFound: true}
		err := errors.Wrap(dnsErr, "failed to dial")
		require.Equal(t, byte(ctrl_msg.ErrorTypeDnsResolutionFailed), classifyDialError(err))
	})

	t.Run("resource exhaustion EMFILE", func(t *testing.T) {
		err := fmt.Errorf("dial failed: %w", syscall.EMFILE)
		require.Equal(t, byte(ctrl_msg.ErrorTypeResourcesNotAvailable), classifyDialError(err))
	})

	t.Run("resource exhaustion ENFILE", func(t *testing.T) {
		err := fmt.Errorf("dial failed: %w", syscall.ENFILE)
		require.Equal(t, byte(ctrl_msg.ErrorTypeResourcesNotAvailable), classifyDialError(err))
	})

	t.Run("resource exhaustion ENOBUFS", func(t *testing.T) {
		err := fmt.Errorf("dial failed: %w", syscall.ENOBUFS)
		require.Equal(t, byte(ctrl_msg.ErrorTypeResourcesNotAvailable), classifyDialError(err))
	})

	t.Run("timeout", func(t *testing.T) {
		err := fmt.Errorf("dial failed: %w", syscall.ETIMEDOUT)
		require.Equal(t, byte(ctrl_msg.ErrorTypeDialTimedOut), classifyDialError(err))
	})

	t.Run("misconfigured terminator", func(t *testing.T) {
		err := xgress.MisconfiguredTerminatorError{InnerError: fmt.Errorf("bad address")}
		require.Equal(t, byte(ctrl_msg.ErrorTypeMisconfiguredTerminator), classifyDialError(err))
	})

	t.Run("invalid terminator", func(t *testing.T) {
		err := xgress.InvalidTerminatorError{InnerError: fmt.Errorf("not found")}
		require.Equal(t, byte(ctrl_msg.ErrorTypeInvalidTerminator), classifyDialError(err))
	})

	t.Run("port not allowed", func(t *testing.T) {
		err := fmt.Errorf("failed to establish connection: port 7070 is not in allowed port ranges")
		require.Equal(t, byte(ctrl_msg.ErrorTypePortNotAllowed), classifyDialError(err))
	})

	t.Run("rejected by application", func(t *testing.T) {
		err := fmt.Errorf("failed to establish connection with terminator address abc. error: (rejected by application)")
		require.Equal(t, byte(ctrl_msg.ErrorTypeRejectedByApplication), classifyDialError(err))
	})

	t.Run("generic error", func(t *testing.T) {
		err := fmt.Errorf("something unexpected happened")
		require.Equal(t, byte(ctrl_msg.ErrorTypeGeneric), classifyDialError(err))
	})

	// DNS errors should be classified before timeout errors, since *net.DNSError implements net.Error
	t.Run("DNS error not classified as timeout", func(t *testing.T) {
		err := &net.DNSError{Err: "server misbehaving", Name: "example.com", IsTimeout: true}
		require.Equal(t, byte(ctrl_msg.ErrorTypeDnsResolutionFailed), classifyDialError(err),
			"DNS errors should be classified as DNS failures even when IsTimeout is true")
	})

	// ER/T hosted services send errors as strings, so DNS errors lose their type
	t.Run("DNS error from string (no such host)", func(t *testing.T) {
		err := fmt.Errorf("failed to establish connection: dial tcp: lookup bad.host on 127.0.0.53:53: no such host")
		require.Equal(t, byte(ctrl_msg.ErrorTypeDnsResolutionFailed), classifyDialError(err))
	})

	t.Run("DNS error from string (server misbehaving)", func(t *testing.T) {
		err := fmt.Errorf("failed to establish connection: dial tcp: lookup bad.host on 127.0.0.53:53: server misbehaving")
		require.Equal(t, byte(ctrl_msg.ErrorTypeDnsResolutionFailed), classifyDialError(err))
	})
}
