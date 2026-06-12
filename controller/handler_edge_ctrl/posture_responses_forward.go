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
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	sdkedge_pb "github.com/openziti/sdk-golang/pb/edge_client_pb"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"google.golang.org/protobuf/proto"
)

// contentTypePostureResponsesForward must match the constant in router/xgress_edge/listener.go.
const contentTypePostureResponsesForward = int32(20507)
const hdrPostureIdentityId = int32(2001)
const hdrPostureApiSessionId = int32(2002)

type postureResponsesForwardHandler struct {
	appEnv *env.AppEnv
}

func NewPostureResponsesForwardHandler(appEnv *env.AppEnv) channel.TypedReceiveHandler {
	return &postureResponsesForwardHandler{appEnv: appEnv}
}

func (self *postureResponsesForwardHandler) ContentType() int32 {
	return contentTypePostureResponsesForward
}

func (self *postureResponsesForwardHandler) Label() string {
	return "posture.responses.forward"
}

func (self *postureResponsesForwardHandler) HandleReceive(msg *channel.Message, _ channel.Channel) {
	identityId := string(msg.Headers[hdrPostureIdentityId])
	apiSessionId := string(msg.Headers[hdrPostureApiSessionId])

	if identityId == "" || apiSessionId == "" {
		pfxlog.Logger().Warn("forwarded posture responses missing identity or apiSession header, dropping")
		return
	}

	postureResponses := &sdkedge_pb.PostureResponses{}
	if err := proto.Unmarshal(msg.Body, postureResponses); err != nil {
		pfxlog.Logger().WithError(err).Warn("failed to unmarshal forwarded posture responses")
		return
	}

	// only the path->check-ID mapping touches the store, so build it only when a process is present.
	var singleProcessChecksByPath map[string][]string
	for _, resp := range postureResponses.Responses {
		if resp.GetProcessList() != nil {
			singleProcessChecksByPath = self.buildSingleProcessCheckPathMap()
			break
		}
	}

	modelResponses := postureResponsesToModel(postureResponses, time.Now().UTC(), singleProcessChecksByPath)

	if len(modelResponses) > 0 {
		self.appEnv.Managers.PostureResponse.Create(identityId, modelResponses)
	}
}

// postureResponsesToModel converts forwarded SDK posture responses into controller model objects.
// Process checks evaluate two ways: PROCESS_MULTI by path, legacy PROCESS by check ID.
// singleProcessChecksByPath supplies the real IDs for matching paths.
func postureResponsesToModel(postureResponses *sdkedge_pb.PostureResponses, now time.Time, singleProcessChecksByPath map[string][]string) []*model.PostureResponse {
	var modelResponses []*model.PostureResponse

	for _, resp := range postureResponses.Responses {
		if os := resp.GetOs(); os != nil {
			pr := &model.PostureResponse{
				PostureCheckId: "ha-forwarded-os",
				TypeId:         model.PostureCheckTypeOs,
				TimedOut:       false,
				LastUpdatedAt:  now,
			}
			pr.SubType = &model.PostureResponseOs{
				PostureResponse: pr,
				Type:            os.Type,
				Version:         os.Version,
				Build:           os.Build,
			}
			modelResponses = append(modelResponses, pr)

		} else if macs := resp.GetMacs(); macs != nil {
			pr := &model.PostureResponse{
				PostureCheckId: "ha-forwarded-mac",
				TypeId:         model.PostureCheckTypeMAC,
				TimedOut:       false,
				LastUpdatedAt:  now,
			}
			pr.SubType = &model.PostureResponseMac{
				PostureResponse: pr,
				Addresses:       macs.Addresses,
			}
			modelResponses = append(modelResponses, pr)

		} else if domain := resp.GetDomain(); domain != nil {
			pr := &model.PostureResponse{
				PostureCheckId: "ha-forwarded-domain",
				TypeId:         model.PostureCheckTypeDomain,
				TimedOut:       false,
				LastUpdatedAt:  now,
			}
			pr.SubType = &model.PostureResponseDomain{
				PostureResponse: pr,
				Name:            domain.Name,
			}
			modelResponses = append(modelResponses, pr)

		} else if procList := resp.GetProcessList(); procList != nil {
			for _, proc := range procList.Processes {
				// One entry per claiming check ID (legacy PROCESS), or a synthetic ID when none.
				// Either way Path is set, which is what PROCESS_MULTI matches on.
				checkIds := singleProcessChecksByPath[proc.Path]
				if len(checkIds) == 0 {
					checkIds = []string{"ha-forwarded-process:" + proc.Path}
				}

				for _, checkId := range checkIds {
					pr := &model.PostureResponse{
						PostureCheckId: checkId,
						TypeId:         model.PostureCheckTypeProcess,
						TimedOut:       false,
						LastUpdatedAt:  now,
					}
					pr.SubType = &model.PostureResponseProcess{
						PostureResponse:    pr,
						Path:               proc.Path,
						IsRunning:          proc.IsRunning,
						BinaryHash:         proc.Hash,
						SignerFingerprints: proc.SignerFingerprints,
					}
					modelResponses = append(modelResponses, pr)
				}
			}
		}
	}

	return modelResponses
}

// path -> IDs of legacy single-PROCESS checks watching it. PROCESS_MULTI is matched by path, not
// ID, so it is excluded.
func (self *postureResponsesForwardHandler) buildSingleProcessCheckPathMap() map[string][]string {
	result := map[string][]string{}

	checks, err := self.appEnv.Managers.PostureCheck.Query("limit none")
	if err != nil {
		pfxlog.Logger().WithError(err).Warn("could not query posture checks for process path mapping")
		return result
	}

	for _, check := range checks.PostureChecks {
		if proc, ok := check.SubType.(*model.PostureCheckProcess); ok && proc.Path != "" {
			result[proc.Path] = append(result[proc.Path], check.Id)
		}
	}

	return result
}
