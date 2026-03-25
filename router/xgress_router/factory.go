package xgress_router

import (
	"time"

	"github.com/openziti/identity"
	"github.com/openziti/sdk-golang/xgress"
	"github.com/openziti/ziti/v2/common/logcontext"
	"github.com/openziti/ziti/v2/controller/xt"
)

// Listener represents an xgress listener that handles incoming connections for a specific binding.
type Listener interface {
	Listen(address string, bindHandler xgress.BindHandler) error
	Close() error
	Binding() string
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
	InspectTerminator(id string, destination string, fixInvalid bool, postCreate bool) (bool, bool, string)
}

type Inspectable interface {
	Inspect(key string, timeout time.Duration) any
}

type Factory interface {
	CreateListener(optionsData xgress.OptionsData) (Listener, error)
	CreateDialer(optionsData xgress.OptionsData) (Dialer, error)
}
