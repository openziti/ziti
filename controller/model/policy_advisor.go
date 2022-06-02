package model

import (
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/fabric/controller/db"
	"github.com/openziti/foundation/util/stringz"
	"go.etcd.io/bbolt"
)

func NewPolicyAdvisor(env Env) *PolicyAdvisor {
	result := &PolicyAdvisor{
		env: env,
	}
	return result
}

type PolicyAdvisor struct {
	env Env
}

type AdvisorEdgeRouter struct {
	Router   *EdgeRouter
	IsOnline bool
}

type AdvisorServiceReachability struct {
	Identity            *Identity
	Service             *Service
	IsBindAllowed       bool
	IsDialAllowed       bool
	IdentityRouterCount int
	ServiceRouterCount  int
	CommonRouters       []*AdvisorEdgeRouter
}

func (advisor *PolicyAdvisor) AnalyzeServiceReachability(identityId, serviceId string) (*AdvisorServiceReachability, error) {
	identity, err := advisor.env.GetManagers().Identity.Read(identityId)
	if err != nil {
		return nil, err
	}

	service, err := advisor.env.GetManagers().EdgeService.Read(serviceId)
	if err != nil {
		return nil, err
	}

	permissions, err := advisor.getServicePermissions(identityId, serviceId)

	if err != nil {
		return nil, err
	}

	edgeRouters, err := advisor.getIdentityEdgeRouters(identityId)
	if err != nil {
		return nil, err
	}

	serviceEdgeRouters, err := advisor.getServiceEdgeRouters(serviceId)
	if err != nil {
		return nil, err
	}

	result := &AdvisorServiceReachability{
		Identity:            identity,
		Service:             service,
		IsBindAllowed:       stringz.Contains(permissions, persistence.PolicyTypeBindName),
		IsDialAllowed:       stringz.Contains(permissions, persistence.PolicyTypeDialName),
		IdentityRouterCount: len(edgeRouters),
		ServiceRouterCount:  len(serviceEdgeRouters),
	}

	for edgeRouterId := range serviceEdgeRouters {
		if edgeRouter, ok := edgeRouters[edgeRouterId]; ok {
			result.CommonRouters = append(result.CommonRouters, edgeRouter)
		}
	}

	return result, nil
}

func (advisor *PolicyAdvisor) getServicePermissions(identityId, serviceId string) ([]string, error) {
	var permissions []string

	servicePolicyStore := advisor.env.GetStores().ServicePolicy
	servicePolicyIterator := func(tx *bbolt.Tx, servicePolicyId string) error {
		servicePolicy, err := servicePolicyStore.LoadOneById(tx, servicePolicyId)
		if err != nil {
			return err
		}
		if servicePolicyStore.IsEntityRelated(tx, servicePolicyId, db.EntityTypeServices, serviceId) {
			if !stringz.Contains(permissions, servicePolicy.GetPolicyTypeName()) {
				permissions = append(permissions, servicePolicy.GetPolicyTypeName())
			}
		}
		return nil
	}

	if err := advisor.env.GetManagers().Identity.iterateRelatedEntities(identityId, persistence.EntityTypeServicePolicies, servicePolicyIterator); err != nil {
		return nil, err
	}

	return permissions, nil
}

func (advisor *PolicyAdvisor) getIdentityEdgeRouters(identityId string) (map[string]*AdvisorEdgeRouter, error) {
	edgeRouters := map[string]*AdvisorEdgeRouter{}

	edgeRouterPolicyIterator := func(tx *bbolt.Tx, edgeRouterPolicyId string) error {
		edgeRouterIterator := func(tx *bbolt.Tx, edgeRouterId string) error {
			commonRouter := edgeRouters[edgeRouterId]
			if commonRouter == nil {
				edgeRouter, err := advisor.env.GetManagers().EdgeRouter.readInTx(tx, edgeRouterId)
				if err != nil {
					return err
				}
				commonRouter = &AdvisorEdgeRouter{
					Router:   edgeRouter,
					IsOnline: advisor.env.IsEdgeRouterOnline(edgeRouter.Id),
				}
				edgeRouters[edgeRouterId] = commonRouter
			}

			return nil
		}

		return advisor.env.GetManagers().EdgeRouterPolicy.iterateRelatedEntitiesInTx(tx, edgeRouterPolicyId, db.EntityTypeRouters, edgeRouterIterator)
	}
	if err := advisor.env.GetManagers().Identity.iterateRelatedEntities(identityId, persistence.EntityTypeEdgeRouterPolicies, edgeRouterPolicyIterator); err != nil {
		return nil, err
	}

	return edgeRouters, nil
}

func (advisor *PolicyAdvisor) getServiceEdgeRouters(serviceId string) (map[string]struct{}, error) {
	edgeRouters := map[string]struct{}{}

	serviceEdgeRouterPolicyIterator := func(tx *bbolt.Tx, policyId string) error {
		edgeRouterIterator := func(tx *bbolt.Tx, edgeRouterId string) error {
			edgeRouters[edgeRouterId] = struct{}{}
			return nil
		}
		return advisor.env.GetManagers().ServiceEdgeRouterPolicy.iterateRelatedEntitiesInTx(tx, policyId, db.EntityTypeRouters, edgeRouterIterator)
	}

	if err := advisor.env.GetManagers().EdgeService.iterateRelatedEntities(serviceId, persistence.EntityTypeServiceEdgeRouterPolicies, serviceEdgeRouterPolicyIterator); err != nil {
		return nil, err
	}

	return edgeRouters, nil
}

type AdvisorIdentityEdgeRouterLinks struct {
	Identity   *Identity
	EdgeRouter *EdgeRouter
	Policies   []*EdgeRouterPolicy
}

func (advisor *PolicyAdvisor) InspectIdentityEdgeRouterLinks(identityId, edgeRouterId string) (*AdvisorIdentityEdgeRouterLinks, error) {
	identity, err := advisor.env.GetManagers().Identity.Read(identityId)
	if err != nil {
		return nil, err
	}

	edgeRouter, err := advisor.env.GetManagers().EdgeRouter.Read(edgeRouterId)
	if err != nil {
		return nil, err
	}

	policies, err := advisor.getEdgeRouterPolicies(identityId, edgeRouterId)
	if err != nil {
		return nil, err
	}

	result := &AdvisorIdentityEdgeRouterLinks{
		Identity:   identity,
		EdgeRouter: edgeRouter,
		Policies:   policies,
	}

	return result, nil
}

func (advisor *PolicyAdvisor) getEdgeRouterPolicies(identityId, edgeRouterId string) ([]*EdgeRouterPolicy, error) {
	var result []*EdgeRouterPolicy

	policyStore := advisor.env.GetStores().EdgeRouterPolicy
	policyIterator := func(tx *bbolt.Tx, policyId string) error {
		policy, err := advisor.env.GetManagers().EdgeRouterPolicy.readInTx(tx, policyId)
		if err != nil {
			return err
		}
		if policyStore.IsEntityRelated(tx, policyId, db.EntityTypeRouters, edgeRouterId) {
			result = append(result, policy)
		}
		return nil
	}

	if err := advisor.env.GetManagers().Identity.iterateRelatedEntities(identityId, persistence.EntityTypeEdgeRouterPolicies, policyIterator); err != nil {
		return nil, err
	}

	return result, nil
}

type AdvisorIdentityServiceLinks struct {
	Identity *Identity
	Service  *Service
	Policies []*ServicePolicy
}

func (advisor *PolicyAdvisor) InspectIdentityServiceLinks(identityId, serviceId string) (*AdvisorIdentityServiceLinks, error) {
	identity, err := advisor.env.GetManagers().Identity.Read(identityId)
	if err != nil {
		return nil, err
	}

	service, err := advisor.env.GetManagers().EdgeService.Read(serviceId)
	if err != nil {
		return nil, err
	}

	policies, err := advisor.getServicePolicies(identityId, serviceId)
	if err != nil {
		return nil, err
	}

	result := &AdvisorIdentityServiceLinks{
		Identity: identity,
		Service:  service,
		Policies: policies,
	}

	return result, nil
}

func (advisor *PolicyAdvisor) getServicePolicies(identityId, serviceId string) ([]*ServicePolicy, error) {
	var result []*ServicePolicy

	policyStore := advisor.env.GetStores().ServicePolicy
	policyIterator := func(tx *bbolt.Tx, policyId string) error {
		policy, err := advisor.env.GetManagers().ServicePolicy.readInTx(tx, policyId)
		if err != nil {
			return err
		}
		if policyStore.IsEntityRelated(tx, policyId, db.EntityTypeServices, serviceId) {
			result = append(result, policy)
		}
		return nil
	}

	if err := advisor.env.GetManagers().Identity.iterateRelatedEntities(identityId, persistence.EntityTypeServicePolicies, policyIterator); err != nil {
		return nil, err
	}

	return result, nil
}

type AdvisorServiceEdgeRouterLinks struct {
	Service    *Service
	EdgeRouter *EdgeRouter
	Policies   []*ServiceEdgeRouterPolicy
}

func (advisor *PolicyAdvisor) InspectServiceEdgeRouterLinks(serviceId, edgeRouterId string) (*AdvisorServiceEdgeRouterLinks, error) {
	service, err := advisor.env.GetManagers().EdgeService.Read(serviceId)
	if err != nil {
		return nil, err
	}

	edgeRouter, err := advisor.env.GetManagers().EdgeRouter.Read(edgeRouterId)
	if err != nil {
		return nil, err
	}

	policies, err := advisor.getServiceEdgeRouterPolicies(serviceId, edgeRouterId)
	if err != nil {
		return nil, err
	}

	result := &AdvisorServiceEdgeRouterLinks{
		Service:    service,
		EdgeRouter: edgeRouter,
		Policies:   policies,
	}

	return result, nil
}

func (advisor *PolicyAdvisor) getServiceEdgeRouterPolicies(serviceId, edgeRouterId string) ([]*ServiceEdgeRouterPolicy, error) {
	var result []*ServiceEdgeRouterPolicy

	policyStore := advisor.env.GetStores().ServiceEdgeRouterPolicy
	policyIterator := func(tx *bbolt.Tx, policyId string) error {
		policy, err := advisor.env.GetManagers().ServiceEdgeRouterPolicy.readInTx(tx, policyId)
		if err != nil {
			return err
		}
		if policyStore.IsEntityRelated(tx, policyId, db.EntityTypeRouters, edgeRouterId) {
			result = append(result, policy)
		}
		return nil
	}

	if err := advisor.env.GetManagers().EdgeService.iterateRelatedEntities(serviceId, persistence.EntityTypeServiceEdgeRouterPolicies, policyIterator); err != nil {
		return nil, err
	}

	return result, nil
}
