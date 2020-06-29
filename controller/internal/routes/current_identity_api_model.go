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
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"path"
)

const EntityNameCurrentIdentity = "current-identity"

var CurrentIdentityLinkFactory FullLinkFactory = NewCurrentIdentityLinkFactory()

type CurrentIdentityLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewCurrentIdentityLinkFactory() *CurrentIdentityLinkFactoryImpl {
	return &CurrentIdentityLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCurrentIdentity),
	}
}

func (factory *CurrentIdentityLinkFactoryImpl) SelfUrlString(_ string) string {
	return "./" + factory.entityName
}


func (factory CurrentIdentityLinkFactoryImpl) NewNestedLink(_ models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{factory.SelfUrlString("")}, elem...)
	return NewLink("./" + path.Join(elem...))
}

func (factory *CurrentIdentityLinkFactoryImpl) SelfLink(_ models.Entity) rest_model.Link {
	return NewLink("./" + factory.entityName)
}

func (factory *CurrentIdentityLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf: factory.SelfLink(entity),
		EntityNameAuthenticator: factory.NewNestedLink(nil, EntityNameAuthenticator),
	}
}