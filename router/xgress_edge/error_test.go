package xgress_edge

import (
	"errors"
	"testing"
	"time"

	"github.com/openziti/channel/v5"
	"github.com/openziti/sdk-golang/v2/xgress"
	sdk "github.com/openziti/sdk-golang/v2/ziti/edge"
	"github.com/openziti/ziti/v2/common/ctrl_msg"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

// newCtrlErrorReply builds the reply a controller sends for a denied request, matching what
// handler_edge_ctrl's returnError puts on the wire.
func newCtrlErrorReply(msg string, code uint32, retryHint sdk.RetryHint) *channel.Message {
	result := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), []byte(msg))
	result.PutUint32Header(sdk.ErrorCodeHeader, code)
	result.PutUint32Header(ctrl_msg.ErrorRetryHintHeader, uint32(retryHint))
	return result
}

// TestCtrlErrorReachesSdkAsTypedFailure locks in the full dial-denial path: a controller error
// reply must survive the hop through the router as a structured error, so the SDK classifies the
// refusal instead of receiving opaque text.
func TestCtrlErrorReachesSdkAsTypedFailure(t *testing.T) {
	tests := []struct {
		name          string
		code          uint32
		retryHint     sdk.RetryHint
		expectedCause sdk.FailureCause
	}{
		{
			name:          "access denied",
			code:          sdk.ErrorCodeAccessDenied,
			retryHint:     sdk.RetryDefault,
			expectedCause: sdk.CauseAccessDenied,
		},
		{
			name:          "invalid service",
			code:          sdk.ErrorCodeInvalidService,
			retryHint:     sdk.RetryNotRetriable,
			expectedCause: sdk.CauseServiceNotAvailable,
		},
		{
			name:          "invalid session",
			code:          sdk.ErrorCodeInvalidSession,
			retryHint:     sdk.RetryStartOver,
			expectedCause: sdk.CauseSessionInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := require.New(t)

			ctrlReply := newCtrlErrorReply("denied for a reason", test.code, test.retryHint)

			err := newEdgeErrorFromCtrlError(ctrlReply, string(ctrlReply.Body))
			edgeErr, ok := err.(*EdgeError)
			req.True(ok, "controller error with a code must become an EdgeError")
			req.Equal(test.code, edgeErr.Code)
			req.Equal(test.retryHint, edgeErr.RetryHint)

			// the router relays it to the sdk on the state-closed reply
			sdkReply := sdk.NewStateClosedMsg(1, edgeErr.Message)
			edgeErr.ApplyToMsg(sdkReply)

			connErr := sdk.ConnRefusalError(sdkReply, "svc", "svc-1", "er", "er-1")
			var typed *sdk.ConnError
			req.ErrorAs(connErr, &typed)
			req.Equal(test.expectedCause, typed.Cause)
		})
	}
}

// TestCtrlErrorWithoutCodeStaysPlain covers controllers that predate the error code header. The
// router must not fabricate a classification the controller never sent.
func TestCtrlErrorWithoutCodeStaysPlain(t *testing.T) {
	req := require.New(t)

	ctrlReply := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), []byte("no code here"))

	err := newEdgeErrorFromCtrlError(ctrlReply, string(ctrlReply.Body))
	req.Error(err)
	req.Equal("no code here", err.Error())

	var edgeErr *EdgeError
	req.False(errors.As(err, &edgeErr), "must not become an EdgeError without a code")
}

// TestCtrlErrorWithCodeButNoRetryHint covers controllers that send the error code header but
// predate the retry hint header. The router must still build a typed EdgeError, defaulting the
// hint to RetryDefault rather than dropping the code.
func TestCtrlErrorWithCodeButNoRetryHint(t *testing.T) {
	req := require.New(t)

	ctrlReply := channel.NewMessage(int32(edge_ctrl_pb.ContentType_ErrorType), []byte("denied, old controller"))
	ctrlReply.PutUint32Header(sdk.ErrorCodeHeader, sdk.ErrorCodeAccessDenied)

	err := newEdgeErrorFromCtrlError(ctrlReply, string(ctrlReply.Body))
	edgeErr, ok := err.(*EdgeError)
	req.True(ok, "controller error with a code must become an EdgeError even without a retry hint")
	req.Equal(sdk.ErrorCodeAccessDenied, edgeErr.Code)
	req.Equal(sdk.RetryDefault, edgeErr.RetryHint)
	req.Equal("denied, old controller", edgeErr.Message)
}

// replyingSender answers any reply-expecting Sendable with a canned message. That is enough to
// drive SendForReply, which creates its buffered reply channel before calling Send.
type replyingSender struct {
	reply *channel.Message
	sent  []*channel.Message
}

func (self *replyingSender) Send(s channel.Sendable) error {
	self.sent = append(self.sent, s.Msg())
	if receiver := s.ReplyReceiver(); receiver != nil {
		receiver.AcceptReply(self.reply)
	}
	return nil
}

func (self *replyingSender) TrySend(s channel.Sendable) (bool, error) {
	return true, self.Send(s)
}

// CloseNotify returns nil so the never-ready case is selected against in WaitForReply.
func (self *replyingSender) CloseNotify() <-chan struct{} {
	return nil
}

// fakeCtrlChannel is a ctrlchan.CtrlChannel whose senders all deliver the canned reply.
type fakeCtrlChannel struct {
	sender *replyingSender
}

func (self *fakeCtrlChannel) InitChannel(channel.Channel)           {}
func (self *fakeCtrlChannel) PeerId() string                        { return "ctrl1" }
func (self *fakeCtrlChannel) GetChannel() channel.Channel           { return nil }
func (self *fakeCtrlChannel) GetDefaultSender() channel.Sender      { return self.sender }
func (self *fakeCtrlChannel) GetHighPrioritySender() channel.Sender { return self.sender }
func (self *fakeCtrlChannel) GetLowPrioritySender() channel.Sender  { return self.sender }
func (self *fakeCtrlChannel) IsConnected() bool                     { return true }
func (self *fakeCtrlChannel) Close() error                          { return nil }
func (self *fakeCtrlChannel) IsClosed() bool                        { return false }

// newCtrlErrorTestConn builds the minimum needed to call the create-circuit senders. The circuit
// timeout must be non-zero, or the envelope's context is already expired and it never sends.
func newCtrlErrorTestConn(reply *channel.Message) (*edgeClientConn, ctrlchan.CtrlChannel) {
	conn := &edgeClientConn{
		listener: &listener{
			options: &Options{Options: xgress.Options{GetCircuitTimeout: time.Second}},
		},
	}
	return conn, &fakeCtrlChannel{sender: &replyingSender{reply: reply}}
}

// TestV3DialConvertsCtrlError checks that the V3 relay types the controller's denial. The request
// carries a decoy error code, since both the request and the reply are messages in scope at that
// call site and reading the wrong one is the mistake this guards against.
func TestV3DialConvertsCtrlError(t *testing.T) {
	req := require.New(t)

	reply := newCtrlErrorReply("denied by policy", sdk.ErrorCodeAccessDenied, sdk.RetryDefault)
	conn, ctrlCh := newCtrlErrorTestConn(reply)

	request := channel.NewMessage(int32(edge_ctrl_pb.ContentType_CreateCircuitV3RequestType), nil)
	request.PutUint32Header(sdk.ErrorCodeHeader, sdk.ErrorCodeInvalidSession)

	resp, err := conn.sendCreateCircuitV3Msg(request, ctrlCh)
	req.Nil(resp)

	var edgeErr *EdgeError
	req.ErrorAs(err, &edgeErr)
	req.Equal(sdk.ErrorCodeAccessDenied, edgeErr.Code)
	req.Equal(sdk.RetryDefault, edgeErr.RetryHint)
	req.Equal("denied by policy", edgeErr.Message)
}

// TestV1DialConvertsCtrlError checks the same for the V1/V2 relay, which non-OIDC sessions and
// clients forcing connect-v1 still use.
func TestV1DialConvertsCtrlError(t *testing.T) {
	req := require.New(t)

	reply := newCtrlErrorReply("session no longer valid", sdk.ErrorCodeInvalidSession, sdk.RetryStartOver)
	conn, ctrlCh := newCtrlErrorTestConn(reply)

	resp, err := conn.sendCreateCircuitRequest(&ctrl_msg.CreateCircuitV2Request{}, ctrlCh)
	req.Nil(resp)

	var edgeErr *EdgeError
	req.ErrorAs(err, &edgeErr)
	req.Equal(sdk.ErrorCodeInvalidSession, edgeErr.Code)
	req.Equal(sdk.RetryStartOver, edgeErr.RetryHint)
	req.Equal("session no longer valid", edgeErr.Message)
}
