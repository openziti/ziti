package xgress_router

import (
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common/logcontext"
	"github.com/openziti/ziti/controller/xt"
	"github.com/openziti/sdk-golang/xgress"
	"time"
)

type Listener interface {
	Listen(address string, bindHandler xgress.BindHandler) error
	Close() error
}

type DialParams interface {
	GetCtrlId() string
	GetDestination() string
	GetCircuitId() *identity.TokenId
	GetAddress() xgress.Address
	GetBindHandler() xgress.BindHandler
	GetLogContext() logcontext.Context
	GetDeadline() time.Time
	GetCircuitTags() map[string]string
}

type Dialer interface {
	Dial(params DialParams) (xt.PeerData, error)
	IsTerminatorValid(id string, destination string) bool
}

type InspectableDialer interface {
	Dialer
	InspectTerminator(id string, destination string, fixInvalid bool) (bool, string)
}

type Inspectable interface {
	Inspect(key string, timeout time.Duration) any
}

type Factory interface {
	CreateListener(optionsData xgress.OptionsData) (Listener, error)
	CreateDialer(optionsData xgress.OptionsData) (Dialer, error)
}
