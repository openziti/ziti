/*
	Copyright NetFoundry, Inc.

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

package routes

import (
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"net/http"
)

type SessionRequestResponder struct {
	response.Responder
	ae *env.AppEnv
}

func (nsr *SessionRequestResponder) RespondWithCreatedId(id string, _ rest_model.Link) {
	modelSession, err := nsr.ae.GetHandlers().Session.Read(id)
	if err != nil {
		nsr.RespondWithError(err)
		return
	}
	restModel, err := MapSessionToRestModel(nsr.ae, modelSession)
	if err != nil {
		nsr.RespondWithError(err)
		return
	}
	newSessionEnvelope := &rest_model.SessionCreateEnvelope{
		Data: &restModel.SessionDetail,
		Meta: &rest_model.Meta{},
	}

	nsr.Respond(newSessionEnvelope, http.StatusCreated)
}
