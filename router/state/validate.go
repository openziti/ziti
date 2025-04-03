package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	controllerEnv "github.com/openziti/ziti/controller/env"
	"github.com/openziti/ziti/router/env"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type ValidateDataStateRequestHandler struct {
	state Manager
	env   env.RouterEnv
}

func NewValidateDataStateRequestHandler(state Manager, env env.RouterEnv) *ValidateDataStateRequestHandler {
	return &ValidateDataStateRequestHandler{
		state: state,
		env:   env,
	}
}

func (*ValidateDataStateRequestHandler) ContentType() int32 {
	return controllerEnv.ValidateDataStateType
}

func (self *ValidateDataStateRequestHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	request := &edge_ctrl_pb.RouterDataModelValidateRequest{}

	if err := proto.Unmarshal(msg.Body, request); err != nil {
		pfxlog.Logger().WithError(err).Errorf("could not unmarshal validate data state request")
		return
	}

	newState := request.State
	model := common.NewBareRouterDataModel()
	model.WhileLocked(func(u uint64, b bool) {
		for _, event := range newState.Events {
			model.Handle(newState.EndIndex, event)
		}
		model.SetCurrentIndex(newState.EndIndex)
	})

	current := self.state.RouterDataModel()

	response := &edge_ctrl_pb.RouterDataModelValidateResponse{
		OrigEntityCounts: model.GetEntityCounts(),
		CopyEntityCounts: current.GetEntityCounts(),
	}

	reportedF := func(entityType string, id string, diffType common.DiffType, detail string) {
		response.Diffs = append(response.Diffs, &edge_ctrl_pb.RouterDataModelDiff{
			EntityType: entityType,
			EntityId:   id,
			DiffType:   string(diffType),
			Detail:     detail,
		})
	}

	current.Validate(model, reportedF)

	if len(response.Diffs) > 0 && request.Fix {
		model = common.NewReceiverRouterDataModelFromExisting(model, RouterDataModelListerBufferSize, self.state.GetEnv().GetCloseNotify())
		self.state.SetRouterDataModel(model, true)
	}

	go func() {
		err := protobufs.MarshalTyped(response).
			ReplyTo(msg).
			WithTimeout(self.env.DefaultRequestTimeout()).
			SendAndWaitForWire(ch)

		if err != nil {
			log.WithError(err).Error("failed to send validate router data model response")
		}
	}()
}
