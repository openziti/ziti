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

package handler_edge_ctrl

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/channel/v5"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// serviceSessionToken stands in for a signed service session JWT. It is a credential, so no
// log entry may contain it.
const serviceSessionToken = "eyJhbGciOiJFUzI1NiJ9.dGhpcy1pcy1hLXNlY3JldA.c2lnbmF0dXJl"

// loggingTestChannel satisfies channel.Channel by embedding the interface. Only the methods
// the logging paths call are implemented; anything else panics on a nil interface, which is
// the signal that the test needs to grow.
type loggingTestChannel struct {
	channel.Channel
	sent []*channel.Message
}

func (self *loggingTestChannel) Id() string {
	return "test-router"
}

func (self *loggingTestChannel) Label() string {
	return "test-channel"
}

func (self *loggingTestChannel) Send(s channel.Sendable) error {
	self.sent = append(self.sent, s.Msg())
	return nil
}

type loggingTestHandler struct {
	baseRequestHandler
}

func (*loggingTestHandler) Label() string {
	return "create.circuit"
}

// capturingHook collects every entry the standard logrus logger emits.
type capturingHook struct {
	entries []*logrus.Entry
}

func (*capturingHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (self *capturingHook) Fire(entry *logrus.Entry) error {
	self.entries = append(self.entries, entry)
	return nil
}

// rendered returns every captured entry serialized the way a log pipeline would see it, so
// an assertion covers the message, the field names and the field values together.
func (self *capturingHook) rendered(t *testing.T) []string {
	t.Helper()
	formatter := &logrus.JSONFormatter{}
	var result []string
	for _, entry := range self.entries {
		line, err := formatter.Format(entry)
		require.NoError(t, err)
		result = append(result, string(line))
	}
	return result
}

// captureLogs installs a hook on the standard logger, which is what both pfxlog and the
// bare logrus calls in these handlers write to, and restores the previous state on cleanup.
func captureLogs(t *testing.T) *capturingHook {
	t.Helper()
	hook := &capturingHook{}
	logger := logrus.StandardLogger()
	previousHooks := logger.ReplaceHooks(logrus.LevelHooks{})
	previousLevel := logger.GetLevel()
	logger.AddHook(hook)
	logger.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() {
		logger.ReplaceHooks(previousHooks)
		logger.SetLevel(previousLevel)
	})
	return hook
}

func newSession() *model.Session {
	return &model.Session{
		BaseEntity:   models.BaseEntity{Id: "session-id"},
		Token:        serviceSessionToken,
		IdentityId:   "identity-id",
		ApiSessionId: "api-session-id",
		ServiceId:    "service-id",
		Type:         db.SessionTypeDial,
	}
}

func newCircuitRequestContext(handler requestHandler, session *model.Session) *CreateCircuitRequestContext {
	return &CreateCircuitRequestContext{
		baseSessionRequestContext: baseSessionRequestContext{
			handler: handler,
			msg:     channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitRequestType), nil),
			session: session,
		},
		req: &edge_ctrl_pb.CreateCircuitRequest{
			SessionToken:    serviceSessionToken,
			ApiSessionToken: "api-session-token",
		},
	}
}

// TestReturnErrorDoesNotLogSessionToken covers the shared error path every session request
// funnels through. The diagnostic must survive; the credential must not appear.
func TestReturnErrorDoesNotLogSessionToken(t *testing.T) {
	req := require.New(t)

	ch := &loggingTestChannel{}
	handler := &loggingTestHandler{baseRequestHandler: baseRequestHandler{ch: ch}}
	ctx := newCircuitRequestContext(handler, newSession())
	hook := captureLogs(t)

	handler.returnError(ctx, internalError(errors.New("no terminators for service")))

	lines := hook.rendered(t)
	req.NotEmpty(lines)
	for _, line := range lines {
		req.NotContains(line, serviceSessionToken)
	}
	req.Contains(lines[0], "session-id")
	req.Contains(lines[0], "identity-id")
	req.Contains(lines[0], "api-session-id")
	req.Contains(lines[0], "service-id")
	req.Contains(lines[0], "no terminators for service")
	req.Len(ch.sent, 1)
}

// TestReturnErrorLogsTokenValidityWindow checks that a JWT service session contributes its
// validity window, which is the diagnostic the token value was standing in for.
func TestReturnErrorLogsTokenValidityWindow(t *testing.T) {
	req := require.New(t)

	issuedAt := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	expiresAt := issuedAt.Add(time.Hour)

	ch := &loggingTestChannel{}
	handler := &loggingTestHandler{baseRequestHandler: baseRequestHandler{ch: ch}}
	ctx := newCircuitRequestContext(handler, newSession())
	ctx.serviceAccessClaims = &common.ServiceAccessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "session-id",
			Subject:   "service-id",
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
		ApiSessionId: "api-session-id",
		IdentityId:   "identity-id",
		Type:         db.SessionTypeDial,
	}
	hook := captureLogs(t)

	handler.returnError(ctx, internalError(errors.New("no terminators for service")))

	lines := hook.rendered(t)
	req.NotEmpty(lines)
	req.NotContains(lines[0], serviceSessionToken)
	req.Contains(lines[0], issuedAt.Format(time.RFC3339))
	req.Contains(lines[0], expiresAt.Format(time.RFC3339))
	req.Contains(lines[0], "sessionExpiresAt")
}

// TestReturnErrorWithoutLoadedSessionDoesNotLogSessionToken covers the case the request
// failed before the session loaded, which is where the token used to be the only identifier
// available to the log entry.
func TestReturnErrorWithoutLoadedSessionDoesNotLogSessionToken(t *testing.T) {
	req := require.New(t)

	ch := &loggingTestChannel{}
	handler := &loggingTestHandler{baseRequestHandler: baseRequestHandler{ch: ch}}
	ctx := newCircuitRequestContext(handler, nil)
	hook := captureLogs(t)

	handler.returnError(ctx, InvalidSessionError{})

	lines := hook.rendered(t)
	req.NotEmpty(lines)
	for _, line := range lines {
		req.NotContains(line, serviceSessionToken)
	}
	req.Contains(lines[0], "create.circuit")
}

// TestLogResultDoesNotLogSessionToken covers the shared result path, on both the failure
// branch at Error and the success branch at Debug.
func TestLogResultDoesNotLogSessionToken(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "operation failed", err: errors.New("circuit creation failed")},
		{name: "operation success", err: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := require.New(t)

			ch := &loggingTestChannel{}
			handler := &loggingTestHandler{baseRequestHandler: baseRequestHandler{ch: ch}}
			ctx := newCircuitRequestContext(handler, newSession())
			hook := captureLogs(t)

			handler.logResult(ctx, test.err)

			lines := hook.rendered(t)
			req.Len(lines, 1)
			req.NotContains(lines[0], serviceSessionToken)
			req.Contains(lines[0], "session-id")
			req.Contains(lines[0], test.name)
		})
	}
}
