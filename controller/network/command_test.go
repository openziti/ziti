package network

import (
	"testing"

	"github.com/openziti/fabric/controller/command"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/stretchr/testify/require"
)

func TestProtobufFactory(t *testing.T) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := require.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	n, err := NewNetwork(config)
	req.NoError(err)

	service := &Service{
		BaseEntity: models.BaseEntity{
			Id: "one",
		},
		Name:               "two",
		TerminatorStrategy: "smartrouting",
	}

	createCmd := &command.CreateEntityCommand[*Service]{
		Creator: n.Managers.Services,
		Entity:  service,
	}

	b, err := createCmd.Encode()
	req.NoError(err)

	val, err := n.Managers.Command.Decoders.Decode(b)
	req.NoError(err)
	msg, ok := val.(*command.CreateEntityCommand[*Service])
	req.True(ok)
	req.NoError(err)
	req.Equal(service.Id, msg.Entity.Id)
	req.Equal(service.Name, msg.Entity.Name)
}

func BenchmarkRegisterCommand(t *testing.B) {
	ctx := db.NewTestContext(t)
	defer ctx.Cleanup()

	req := require.New(t)

	config := newTestConfig(ctx)
	defer close(config.closeNotify)

	n, err := NewNetwork(config)
	req.NoError(err)

	service := &Service{
		BaseEntity: models.BaseEntity{
			Id: "one",
		},
		Name:               "two",
		TerminatorStrategy: "smartrouting",
	}

	createCmd := &command.CreateEntityCommand[*Service]{
		Creator: n.Managers.Services,
		Entity:  service,
	}

	b, err := createCmd.Encode()
	req.NoError(err)

	cmdType := int32(cmd_pb.CommandType_CreateEntityType)
	decoder := n.Managers.Command.Decoders.GetDecoder(cmdType)

	for i := 0; i < t.N; i++ {
		_, err = decoder.Decode(cmdType, b)
		req.NoError(err)
	}
}
