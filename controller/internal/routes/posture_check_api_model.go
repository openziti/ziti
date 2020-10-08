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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
)

const EntityNamePostureCheck = "posture-checks"

var PostureCheckLinkFactory = NewPostureCheckLinkFactory()

type PostureCheckLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewPostureCheckLinkFactory() *PostureCheckLinkFactoryImpl {
	return &PostureCheckLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNamePostureCheck),
	}
}

func (factory *PostureCheckLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	links := factory.BasicLinkFactory.Links(entity)
	return links
}

func MapCreatePostureCheckToModel(postureCheck rest_model.PostureCheckCreate) *model.PostureCheck {
	ret := &model.PostureCheck{
		BaseEntity: models.BaseEntity{
			Tags: postureCheck.Tags(),
		},
		Name: stringz.OrEmpty(postureCheck.Name()),
	}

	return ret
}

func MapUpdatePostureCheckToModel(id string, postureCheck rest_model.PostureCheckUpdate) *model.PostureCheck {
	ret := &model.PostureCheck{
		BaseEntity: models.BaseEntity{
			Tags: postureCheck.Tags(),
			Id:   id,
		},
		Name: stringz.OrEmpty(postureCheck.Name()),
	}

	return ret
}

func MapPatchPostureCheckToModel(id string, postureCheck rest_model.PostureCheckPatch) *model.PostureCheck {
	ret := &model.PostureCheck{
		BaseEntity: models.BaseEntity{
			Tags: postureCheck.Tags(),
			Id:   id,
		},
		Name: postureCheck.Name(),
	}

	return ret
}

func MapPostureCheckToRestEntity(_ *env.AppEnv, _ *response.RequestContext, e models.Entity) (interface{}, error) {
	i, ok := e.(*model.PostureCheck)

	if !ok {
		err := fmt.Errorf("entity is not a Posture Check \"%s\"", e.GetId())
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}

	al, err := MapPostureCheckToRestModel(i)

	if err != nil {
		err := fmt.Errorf("could not convert to API entity \"%s\": %s", e.GetId(), err)
		log := pfxlog.Logger()
		log.Error(err)
		return nil, err
	}
	return al, nil
}

func MapPostureCheckToRestModel(i *model.PostureCheck) (rest_model.PostureCheckDetail, error) {
	ret := &rest_model.PostureCheckOperatingSystemDetail{
		PostureCheckDetail: rest_model.PostureCheckDetail{},
		OperatingSystems:   []*rest_model.OperatingSystemMatch{},
	}

	return ret.PostureCheckDetail, nil
}
