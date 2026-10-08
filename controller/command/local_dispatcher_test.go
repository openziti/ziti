package command

import (
	"errors"
	"testing"

	"github.com/openziti/foundation/v2/rate"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/storage/boltz"
	"github.com/stretchr/testify/require"
)

type validatingTestCmd struct {
	validateErr error
	applied     bool
}

func (c *validatingTestCmd) Apply(boltz.MutateContext) error   { c.applied = true; return nil }
func (c *validatingTestCmd) GetChangeContext() *change.Context { return nil }
func (c *validatingTestCmd) Encode() ([]byte, error)           { return nil, nil }
func (c *validatingTestCmd) Validate() error                   { return c.validateErr }

func TestLocalDispatcher_Dispatch_Validates(t *testing.T) {
	req := require.New(t)
	dispatcher := &LocalDispatcher{Limiter: rate.NoOpRateLimiter{}}

	invalid := &validatingTestCmd{validateErr: errors.New("invalid")}
	err := dispatcher.Dispatch(invalid)
	req.Error(err)
	req.Contains(err.Error(), "invalid")
	req.False(invalid.applied, "a command whose Validate fails is not applied")

	valid := &validatingTestCmd{}
	req.NoError(dispatcher.Dispatch(valid))
	req.True(valid.applied)
}
