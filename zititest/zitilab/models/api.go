package models

import (
	"context"
	"fmt"
	"github.com/openziti/edge-api/rest_management_api_client/config"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_management_api_client/posture_checks"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/ziti/util"
	"github.com/openziti/ziti/zitirest"
	"time"
)

func ListServices(clients *zitirest.Clients, filter string, timeout time.Duration) ([]*rest_model.ServiceDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.Service.ListServices(&service.ListServicesParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func CreateService(clients *zitirest.Clients, svc *rest_model.ServiceCreate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Service.CreateService(&service.CreateServiceParams{
		Context: ctx,
		Service: svc,
	}, nil)

	return err
}

func DeleteService(clients *zitirest.Clients, id string, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Service.DeleteService(&service.DeleteServiceParams{
		Context: ctx,
		ID:      id,
	}, nil)

	return err
}

func UpdateServiceFromDetail(clients *zitirest.Clients, svc *rest_model.ServiceDetail, timeout time.Duration) error {
	svcUpdate := &rest_model.ServiceUpdate{
		Configs:            svc.Configs,
		EncryptionRequired: *svc.EncryptionRequired,
		MaxIdleTimeMillis:  *svc.MaxIdleTimeMillis,
		Name:               svc.Name,
		RoleAttributes:     *svc.RoleAttributes,
		Tags:               svc.Tags,
		TerminatorStrategy: *svc.TerminatorStrategy,
	}
	return UpdateService(clients, *svc.ID, svcUpdate, timeout)
}

func UpdateService(clients *zitirest.Clients, id string, svc *rest_model.ServiceUpdate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Service.UpdateService(&service.UpdateServiceParams{
		Context: ctx,
		ID:      id,
		Service: svc,
	}, nil)

	return err
}

func ListIdentities(clients *zitirest.Clients, filter string, timeout time.Duration) ([]*rest_model.IdentityDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.Identity.ListIdentities(&identity.ListIdentitiesParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func CreateIdentity(clients *zitirest.Clients, entity *rest_model.IdentityCreate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Identity.CreateIdentity(&identity.CreateIdentityParams{
		Context:  ctx,
		Identity: entity,
	}, nil)

	return util.WrapIfApiError(err)
}

func DeleteIdentity(clients *zitirest.Clients, id string, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Identity.DeleteIdentity(&identity.DeleteIdentityParams{
		Context: ctx,
		ID:      id,
	}, nil)

	return err
}

func UpdateIdentityFromDetail(clients *zitirest.Clients, entity *rest_model.IdentityDetail, timeout time.Duration) error {
	typeId := rest_model.IdentityType(entity.Type.ID)
	identityUpdate := &rest_model.IdentityUpdate{
		AppData:                   entity.AppData,
		AuthPolicyID:              entity.AuthPolicyID,
		DefaultHostingCost:        entity.DefaultHostingCost,
		DefaultHostingPrecedence:  entity.DefaultHostingPrecedence,
		ExternalID:                entity.ExternalID,
		IsAdmin:                   entity.IsAdmin,
		Name:                      entity.Name,
		RoleAttributes:            entity.RoleAttributes,
		ServiceHostingCosts:       entity.ServiceHostingCosts,
		ServiceHostingPrecedences: entity.ServiceHostingPrecedences,
		Tags:                      entity.Tags,
		Type:                      &typeId,
	}
	return UpdateIdentity(clients, *entity.ID, identityUpdate, timeout)
}

func UpdateIdentity(clients *zitirest.Clients, id string, entity *rest_model.IdentityUpdate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Identity.UpdateIdentity(&identity.UpdateIdentityParams{
		Context:  ctx,
		ID:       id,
		Identity: entity,
	}, nil)

	return err
}

func ListServicePolicies(clients *zitirest.Clients, filter string, timeout time.Duration) ([]*rest_model.ServicePolicyDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.ServicePolicy.ListServicePolicies(&service_policy.ListServicePoliciesParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func CreateServicePolicy(clients *zitirest.Clients, entity *rest_model.ServicePolicyCreate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.ServicePolicy.CreateServicePolicy(&service_policy.CreateServicePolicyParams{
		Context: ctx,
		Policy:  entity,
	}, nil)

	return err
}

func DeleteServicePolicy(clients *zitirest.Clients, id string, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.ServicePolicy.DeleteServicePolicy(&service_policy.DeleteServicePolicyParams{
		Context: ctx,
		ID:      id,
	}, nil)

	return err
}

func UpdateServicePolicyFromDetail(clients *zitirest.Clients, entity *rest_model.ServicePolicyDetail, timeout time.Duration) error {
	servicePolicyUpdate := &rest_model.ServicePolicyUpdate{
		Name:              entity.Name,
		IdentityRoles:     entity.IdentityRoles,
		PostureCheckRoles: entity.PostureCheckRoles,
		Semantic:          entity.Semantic,
		ServiceRoles:      entity.ServiceRoles,
		Tags:              entity.Tags,
		Type:              entity.Type,
	}
	return UpdateServicePolicy(clients, *entity.ID, servicePolicyUpdate, timeout)
}

func UpdateServicePolicy(clients *zitirest.Clients, id string, entity *rest_model.ServicePolicyUpdate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.ServicePolicy.UpdateServicePolicy(&service_policy.UpdateServicePolicyParams{
		Context: ctx,
		ID:      id,
		Policy:  entity,
	}, nil)

	return err
}

func ListConfigs(clients *zitirest.Clients, filter string, timeout time.Duration) ([]*rest_model.ConfigDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.Config.ListConfigs(&config.ListConfigsParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func CreateConfig(clients *zitirest.Clients, entity *rest_model.ConfigCreate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Config.CreateConfig(&config.CreateConfigParams{
		Context: ctx,
		Config:  entity,
	}, nil)

	return err
}

func DeleteConfig(clients *zitirest.Clients, id string, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Config.DeleteConfig(&config.DeleteConfigParams{
		Context: ctx,
		ID:      id,
	}, nil)

	return err
}

func UpdateConfigFromDetail(clients *zitirest.Clients, entity *rest_model.ConfigDetail, timeout time.Duration) error {
	entityUpdate := &rest_model.ConfigUpdate{
		Data: entity.Data,
		Name: entity.Name,
		Tags: entity.Tags,
	}
	return UpdateConfig(clients, *entity.ID, entityUpdate, timeout)
}

func UpdateConfig(clients *zitirest.Clients, id string, entity *rest_model.ConfigUpdate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Config.UpdateConfig(&config.UpdateConfigParams{
		Context: ctx,
		ID:      id,
		Config:  entity,
	}, nil)

	return err
}

func ListConfigTypes(clients *zitirest.Clients, filter string, timeout time.Duration) ([]*rest_model.ConfigTypeDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.Config.ListConfigTypes(&config.ListConfigTypesParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func ListConfigsOfConfigTypes(clients *zitirest.Clients, id string, timeout time.Duration) ([]*rest_model.ConfigDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.Config.ListConfigsForConfigType(&config.ListConfigsForConfigTypeParams{
		ID:      id,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

func CreateConfigType(clients *zitirest.Clients, entity *rest_model.ConfigTypeCreate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Config.CreateConfigType(&config.CreateConfigTypeParams{
		Context:    ctx,
		ConfigType: entity,
	}, nil)

	return err
}

func DeleteConfigType(clients *zitirest.Clients, id string, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Config.DeleteConfigType(&config.DeleteConfigTypeParams{
		Context: ctx,
		ID:      id,
	}, nil)

	return err
}

func UpdateConfigTypeFromDetail(clients *zitirest.Clients, entity *rest_model.ConfigTypeDetail, timeout time.Duration) error {
	entityUpdate := &rest_model.ConfigTypeUpdate{
		Schema: entity.Schema,
		Name:   entity.Name,
		Tags:   entity.Tags,
	}
	return UpdateConfigType(clients, *entity.ID, entityUpdate, timeout)
}

func UpdateConfigType(clients *zitirest.Clients, id string, entity *rest_model.ConfigTypeUpdate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.Config.UpdateConfigType(&config.UpdateConfigTypeParams{
		Context:    ctx,
		ID:         id,
		ConfigType: entity,
	}, nil)

	return err
}

func ListPostureChecks(clients *zitirest.Clients, filter string, timeout time.Duration) ([]rest_model.PostureCheckDetail, error) {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	result, err := clients.Edge.PostureChecks.ListPostureChecks(&posture_checks.ListPostureChecksParams{
		Filter:  &filter,
		Context: ctx,
	}, nil)

	if err != nil {
		return nil, err
	}
	return result.Payload.Data(), nil
}

func CreatePostureCheck(clients *zitirest.Clients, entity rest_model.PostureCheckCreate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.PostureChecks.CreatePostureCheck(&posture_checks.CreatePostureCheckParams{
		Context:      ctx,
		PostureCheck: entity,
	}, nil)

	return util.WrapIfApiError(err)
}

func DeletePostureCheck(clients *zitirest.Clients, id string, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.PostureChecks.DeletePostureCheck(&posture_checks.DeletePostureCheckParams{
		Context: ctx,
		ID:      id,
	}, nil)

	return err
}

func UpdatePostureCheckFromDetail(clients *zitirest.Clients, entity rest_model.PostureCheckDetail, timeout time.Duration) error {
	var update rest_model.PostureCheckUpdate
	switch p := entity.(type) {
	case *rest_model.PostureCheckDomainDetail:
		update = &rest_model.PostureCheckDomainUpdate{
			Domains: p.Domains,
		}
	case *rest_model.PostureCheckMacAddressDetail:
		update = &rest_model.PostureCheckMacAddressUpdate{
			MacAddresses: p.MacAddresses,
		}
	case *rest_model.PostureCheckMfaDetail:
		update = &rest_model.PostureCheckMfaUpdate{
			PostureCheckMfaProperties: p.PostureCheckMfaProperties,
		}
	case *rest_model.PostureCheckOperatingSystemDetail:
		update = &rest_model.PostureCheckOperatingSystemUpdate{
			OperatingSystems: p.OperatingSystems,
		}
	case *rest_model.PostureCheckProcessDetail:
		update = &rest_model.PostureCheckProcessUpdate{
			Process: p.Process,
		}
	case *rest_model.PostureCheckProcessMultiDetail:
		update = &rest_model.PostureCheckProcessMultiUpdate{
			Semantic:  p.Semantic,
			Processes: p.Processes,
		}
	default:
		return fmt.Errorf("unhandled posture check type %T", p)
	}

	update.SetName(entity.Name())
	update.SetRoleAttributes(entity.RoleAttributes())

	return UpdatePostureCheck(clients, *entity.ID(), update, timeout)
}

func UpdatePostureCheck(clients *zitirest.Clients, id string, entity rest_model.PostureCheckUpdate, timeout time.Duration) error {
	ctx, cancelF := context.WithTimeout(context.Background(), timeout)
	defer cancelF()

	_, err := clients.Edge.PostureChecks.UpdatePostureCheck(&posture_checks.UpdatePostureCheckParams{
		Context:      ctx,
		ID:           id,
		PostureCheck: entity,
	}, nil)

	return err
}
