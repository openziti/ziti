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
	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/controller/response"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"github.com/openziti/foundation/util/stringz"
	"strings"
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
		Name:           stringz.OrEmpty(postureCheck.Name()),
		TypeId:         string(postureCheck.TypeID()),
		Version:        1,
		RoleAttributes: postureCheck.RoleAttributes(),
	}

	switch apiSubType := postureCheck.(type) {
	case *rest_model.PostureCheckOperatingSystemCreate:
		subType := &model.PostureCheckOperatingSystem{
			OperatingSystems: []model.OperatingSystem{},
		}

		for _, os := range apiSubType.OperatingSystems {
			subType.OperatingSystems = append(subType.OperatingSystems, model.OperatingSystem{
				OsType:     string(os.Type),
				OsVersions: os.Versions,
			})
		}
		ret.SubType = subType

	case *rest_model.PostureCheckDomainCreate:
		ret.SubType = &model.PostureCheckWindowsDomains{
			Domains: apiSubType.Domains,
		}
	case *rest_model.PostureCheckMacAddressCreate:
		ret.SubType = &model.PostureCheckMacAddresses{
			MacAddresses: apiSubType.MacAddresses,
		}
	case *rest_model.PostureCheckProcessCreate:
		ret.SubType = &model.PostureCheckProcess{
			OperatingSystem: string(apiSubType.Process.OsType),
			Path:            *apiSubType.Process.Path,
			Hashes:          apiSubType.Process.Hashes,
			Fingerprint:     apiSubType.Process.SignerFingerprint,
		}
	case *rest_model.PostureCheckMfaCreate:
		ret.SubType = &model.PostureCheckMfa{}
	}

	return ret
}

func MapUpdatePostureCheckToModel(id string, postureCheck rest_model.PostureCheckUpdate) *model.PostureCheck {
	ret := &model.PostureCheck{
		BaseEntity: models.BaseEntity{
			Tags: postureCheck.Tags(),
			Id:   id,
		},
		Name:           stringz.OrEmpty(postureCheck.Name()),
		RoleAttributes: postureCheck.RoleAttributes(),
	}

	switch postureCheck.(type) {
	case *rest_model.PostureCheckDomainUpdate:
		check := postureCheck.(*rest_model.PostureCheckDomainUpdate)
		ret.SubType = &model.PostureCheckWindowsDomains{
			Domains: check.Domains,
		}
	case *rest_model.PostureCheckMacAddressUpdate:
		check := postureCheck.(*rest_model.PostureCheckMacAddressUpdate)
		ret.SubType = &model.PostureCheckMacAddresses{
			MacAddresses: check.MacAddresses,
		}
	case *rest_model.PostureCheckProcessUpdate:
		check := postureCheck.(*rest_model.PostureCheckProcessUpdate)
		ret.SubType = &model.PostureCheckProcess{
			OperatingSystem: string(check.Process.OsType),
			Path:            stringz.OrEmpty(check.Process.Path),
			Hashes:          check.Process.Hashes,
			Fingerprint:     check.Process.SignerFingerprint,
		}
	case *rest_model.PostureCheckOperatingSystemUpdate:
		check := postureCheck.(*rest_model.PostureCheckOperatingSystemUpdate)
		osCheck := &model.PostureCheckOperatingSystem{}
		ret.SubType = osCheck

		for _, restOs := range check.OperatingSystems {
			modelOs := model.OperatingSystem{
				OsType:     string(restOs.Type),
				OsVersions: restOs.Versions,
			}
			osCheck.OperatingSystems = append(osCheck.OperatingSystems, modelOs)
		}
	case *rest_model.PostureCheckMfaUpdate:
		ret.SubType = &model.PostureCheckMfa{}
	}

	return ret
}

func MapPatchPostureCheckToModel(id string, postureCheck rest_model.PostureCheckPatch) *model.PostureCheck {
	ret := &model.PostureCheck{
		BaseEntity: models.BaseEntity{
			Tags: postureCheck.Tags(),
			Id:   id,
		},
		Name:           postureCheck.Name(),
		Version:        1,
		RoleAttributes: postureCheck.RoleAttributes(),
	}

	switch postureCheck.(type) {
	case *rest_model.PostureCheckDomainPatch:
		check := postureCheck.(*rest_model.PostureCheckDomainPatch)
		ret.SubType = &model.PostureCheckWindowsDomains{
			Domains: check.Domains,
		}
		ret.TypeId = model.PostureCheckTypeDomain

	case *rest_model.PostureCheckMacAddressPatch:
		check := postureCheck.(*rest_model.PostureCheckMacAddressPatch)
		ret.SubType = &model.PostureCheckMacAddresses{
			MacAddresses: check.MacAddresses,
		}
		ret.TypeId = model.PostureCheckTypeMAC

	case *rest_model.PostureCheckProcessPatch:
		check := postureCheck.(*rest_model.PostureCheckProcessPatch)
		subType := &model.PostureCheckProcess{}
		ret.SubType = subType

		if check.Process != nil {
			subType.OperatingSystem = string(check.Process.OsType)
			subType.Path = stringz.OrEmpty(check.Process.Path)
			subType.Hashes = check.Process.Hashes
			subType.Fingerprint = check.Process.SignerFingerprint
		}
		ret.TypeId = model.PostureCheckTypeProcess

	case *rest_model.PostureCheckOperatingSystemPatch:
		check := postureCheck.(*rest_model.PostureCheckOperatingSystemPatch)
		osCheck := &model.PostureCheckOperatingSystem{}
		ret.SubType = osCheck

		for _, restOs := range check.OperatingSystems {
			modelOs := model.OperatingSystem{
				OsType:     string(restOs.Type),
				OsVersions: restOs.Versions,
			}
			osCheck.OperatingSystems = append(osCheck.OperatingSystems, modelOs)
		}

		ret.TypeId = model.PostureCheckTypeOs
	case *rest_model.PostureCheckMfaPatch:
		ret.SubType = &model.PostureCheckMfa{}
		ret.TypeId = model.PostureCheckTypeMFA
	}

	return ret
}

func MapPostureChecksToRestEntity(ae *env.AppEnv, rc *response.RequestContext, es []*model.PostureCheck) ([]interface{}, error) {
	// can't use modelToApi b/c it require list of network.Entity
	restModel := make([]interface{}, 0)

	for _, e := range es {
		al, err := MapPostureCheckToRestEntity(ae, rc, e)

		if err != nil {
			return nil, err
		}

		restModel = append(restModel, al)
	}

	return restModel, nil
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
	var ret rest_model.PostureCheckDetail

	switch subType := i.SubType.(type) {
	case *model.PostureCheckOperatingSystem:
		osArray := rest_model.OperatingSystemArray{}

		for _, osMatch := range subType.OperatingSystems {
			osArray = append(osArray, &rest_model.OperatingSystem{
				Type:     rest_model.OsType(osMatch.OsType),
				Versions: osMatch.OsVersions,
			})
		}

		ret = &rest_model.PostureCheckOperatingSystemDetail{
			OperatingSystems: osArray,
		}

		setBaseEntityDetailsOnPostureCheck(ret, i)

	case *model.PostureCheckProcess:
		processMatch := &rest_model.Process{
			Hashes:            subType.Hashes,
			OsType:            rest_model.OsType(subType.OperatingSystem),
			Path:              &subType.Path,
			SignerFingerprint: subType.Fingerprint,
		}

		ret = &rest_model.PostureCheckProcessDetail{
			Process: processMatch,
		}

		setBaseEntityDetailsOnPostureCheck(ret, i)
	case *model.PostureCheckWindowsDomains:
		ret = &rest_model.PostureCheckDomainDetail{
			Domains: subType.Domains,
		}
		setBaseEntityDetailsOnPostureCheck(ret, i)
	case *model.PostureCheckMacAddresses:
		ret = &rest_model.PostureCheckMacAddressDetail{
			MacAddresses: subType.MacAddresses,
		}
		setBaseEntityDetailsOnPostureCheck(ret, i)
	case *model.PostureCheckMfa:
		ret = &rest_model.PostureCheckMfaDetail{}
		setBaseEntityDetailsOnPostureCheck(ret, i)
	}

	return ret, nil
}

func setBaseEntityDetailsOnPostureCheck(check rest_model.PostureCheckDetail, i *model.PostureCheck) {
	createdAt := strfmt.DateTime(i.CreatedAt)
	updatedAt := strfmt.DateTime(i.UpdatedAt)
	check.SetCreatedAt(&createdAt)
	check.SetUpdatedAt(&updatedAt)
	check.SetTags(i.Tags)
	check.SetID(&i.Id)
	check.SetLinks(PostureCheckLinkFactory.Links(i))
	check.SetName(&i.Name)
	check.SetTypeID(i.TypeId)
	check.SetVersion(&i.Version)
	check.SetRoleAttributes(i.RoleAttributes)
}

func GetNamedPostureCheckRoles(postureCheckHandler *model.PostureCheckHandler, roles []string) rest_model.NamedRoles {
	result := rest_model.NamedRoles{}
	for _, role := range roles {
		if strings.HasPrefix(role, "@") {

			postureCheck, err := postureCheckHandler.Read(role[1:])
			if err != nil {
				pfxlog.Logger().Errorf("error converting posture check role [%s] to a named role: %v", role, err)
				continue
			}

			result = append(result, &rest_model.NamedRole{
				Role: role,
				Name: "@" + postureCheck.Name,
			})
		} else {
			result = append(result, &rest_model.NamedRole{
				Role: role,
				Name: role,
			})
		}
	}
	return result
}
