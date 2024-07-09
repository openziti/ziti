package model

import (
	"testing"

	"github.com/openziti/ziti/common/pb/cmd_pb"
	"github.com/openziti/ziti/controller/command"
	"github.com/openziti/ziti/controller/models"
	"github.com/stretchr/testify/require"
)

func TestProtobufFactory(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	req := require.New(t)

	service := &Service{
		BaseEntity: models.BaseEntity{
			Id: "one",
		},
		Name:               "two",
		TerminatorStrategy: "smartrouting",
	}

	createCmd := &command.CreateEntityCommand[*Service]{
		Creator: ctx.GetManagers().Service,
		Entity:  service,
	}

	b, err := createCmd.Encode()
	req.NoError(err)

	val, err := ctx.GetManagers().Command.Decoders.Decode(b)
	req.NoError(err)
	msg, ok := val.(*command.CreateEntityCommand[*Service])
	req.True(ok)
	req.NoError(err)
	req.Equal(service.Id, msg.Entity.Id)
	req.Equal(service.Name, msg.Entity.Name)
}

func BenchmarkRegisterCommand(t *testing.B) {
	ctx := NewTestContext(t)
	defer ctx.Cleanup()

	req := require.New(t)

	service := &Service{
		BaseEntity: models.BaseEntity{
			Id: "one",
		},
		Name:               "two",
		TerminatorStrategy: "smartrouting",
	}

	createCmd := &command.CreateEntityCommand[*Service]{
		Creator: ctx.GetManagers().Service,
		Entity:  service,
	}

	b, err := createCmd.Encode()
	req.NoError(err)

	cmdType := int32(cmd_pb.CommandType_CreateEntityType)
	decoder := ctx.GetManagers().Command.Decoders.GetDecoder(cmdType)

	for i := 0; i < t.N; i++ {
		_, err = decoder.Decode(cmdType, b)
		req.NoError(err)
	}
}
