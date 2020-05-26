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
	"fmt"
	"github.com/gobuffalo/packr"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/internal/permissions"
	"github.com/openziti/edge/controller/response"
	"io/ioutil"
	"net/http"
	"time"
)

var specBox packr.Box

func init() {
	specBox = packr.NewBox("../../../specs")

	swagger, err := specBox.Open("swagger.yml")

	if err != nil {
		panic(err)
	}

	swaggerBody, err := ioutil.ReadAll(swagger)

	if err != nil {
		panic(err)
	}

	swaggerEntity = newSpec(SwaggerId, "Swagger/OpenApi 2.0", swaggerBody)

	specEntityList = []*specList{
		swaggerEntity,
	}

	r := NewSpecRouter()
	env.AddRouter(r)

}

const (
	EntityNameSpec     = "specs"
	EntityNameSpecBody = "spec"
	SwaggerId          = "swagger"
)

var swaggerEntity *specList
var specEntityList []*specList

type SpecRouter struct {
	BasePath string
	IdType   response.IdType
}

func NewSpecRouter() *SpecRouter {
	return &SpecRouter{
		BasePath: "/" + EntityNameSpec,
		IdType:   response.IdTypeString,
	}
}

func (ir *SpecRouter) Register(ae *env.AppEnv) {
	route := registerReadOnlyRouter(ae, ae.RootRouter, ir.BasePath, ir, permissions.Always())

	specBodyUrl := fmt.Sprintf("/{%s}/%s", response.IdPropertyName, EntityNameSpecBody)
	detailSpecBodyHandler := ae.WrapHandler(ir.DetailSpecBody, permissions.Always())
	route.HandleFunc(specBodyUrl, detailSpecBodyHandler).Methods(http.MethodGet)
}

func (ir *SpecRouter) List(ae *env.AppEnv, rc *response.RequestContext) {

	rc.RequestResponder.RespondWithOk(specEntityList, nil)
}

func (ir *SpecRouter) Detail(ae *env.AppEnv, rc *response.RequestContext) {
	id, err := rc.GetIdFromRequest(ir.IdType)

	if err != nil || id != SwaggerId {
		rc.RequestResponder.RespondWithNotFound()
		return
	}

	rc.RequestResponder.RespondWithOk(swaggerEntity, nil)
}

func (ir *SpecRouter) DetailSpecBody(ae *env.AppEnv, rc *response.RequestContext) {
	id, err := rc.GetIdFromRequest(ir.IdType)

	if err != nil || id != SwaggerId {
		rc.RequestResponder.RespondWithNotFound()
		return
	}
	rc.ResponseWriter.Header().Add("content-type", "text/yaml")
	rc.ResponseWriter.WriteHeader(http.StatusOK)
	rc.ResponseWriter.Write(swaggerEntity.Body)
}

type specList struct {
	*env.BaseApi
	Name string `json:"name"`
	Body []byte `json:"-"`
}

func newSpec(id, name string, specBody []byte) *specList {
	date, _ := time.Parse(time.RFC3339, "2013-03-30T12:00:00Z")
	ret := &specList{
		BaseApi: &env.BaseApi{
			Id:        id,
			CreatedAt: &date,
			UpdatedAt: &date,
			Tags:      map[string]interface{}{},
		},
		Name: name,
		Body: specBody,
	}
	ret.PopulateLinks()
	return ret
}

func (e *specList) GetSelfLink() *response.Link {
	return e.BuildSelfLink(e.Id)
}

func (specList) BuildSelfLink(id string) *response.Link {
	return response.NewLink(fmt.Sprintf("./%s/%s", EntityNameSpec, id))
}

func (e *specList) PopulateLinks() {
	if e.Links == nil {
		self := e.GetSelfLink()
		e.Links = &response.Links{
			EntityNameSelf: self,
			EntityNameSpec: response.NewLink(fmt.Sprintf(self.Href + "/spec")),
		}
	}
}
