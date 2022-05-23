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

package tests

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/openziti/edge/eid"
	"github.com/openziti/sdk-golang/ziti/config"
	"math/big"
	"sort"
	"time"

	"github.com/Jeffail/gabs"
)

type entity interface {
	getId() string
	setId(string)
	getEntityType() string
	toJson(create bool, ctx *TestContext, fields ...string) string
	validate(ctx *TestContext, c *gabs.Container)
}

type loadableEntity interface {
	entity
	fromJson(ctx *TestContext, c *gabs.Container)
}

type postureCheck struct {
	id             string
	name           string
	typeId         string
	roleAttributes []string
	tags           map[string]interface{}
}

func (p *postureCheck) getId() string {
	return p.id
}

func (p *postureCheck) setId(id string) {
	p.id = id
}

func (p *postureCheck) getEntityType() string {
	return "posture-checks"
}

func (p *postureCheck) toJson(create bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, p.name, "name")
	ctx.setJsonValue(entityData, p.roleAttributes, "roleAttributes")
	ctx.setJsonValue(entityData, p.typeId, "typeId")

	if len(p.tags) > 0 {
		ctx.setJsonValue(entityData, p.tags, "tags")
	}

	return entityData.String()
}

func (p postureCheck) validate(ctx *TestContext, c *gabs.Container) {}

type postureCheckDomain struct {
	postureCheck
	domains []string
}

func (entity *postureCheckDomain) getId() string {
	return entity.id
}

func (entity *postureCheckDomain) setId(id string) {
	entity.id = id
}

func (entity *postureCheckDomain) getEntityType() string {
	return "posture-checks"
}

func (entity *postureCheckDomain) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.domains, "domains")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")
	ctx.setJsonValue(entityData, entity.typeId, "typeId")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}

	return entityData.String()
}

func (entity *postureCheckDomain) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.tags, path("tags"))

	sort.Strings(entity.domains)
	ctx.pathEqualsStringSlice(c, entity.domains, path("domains"))

	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
}

type service struct {
	Id                 string
	Name               string
	terminatorStrategy string
	roleAttributes     []string
	configs            []string
	permissions        []string
	tags               map[string]interface{}
	encryptionRequired bool
}

func (entity *service) getId() string {
	return entity.Id
}

func (entity *service) setId(id string) {
	entity.Id = id
}

func (entity *service) getEntityType() string {
	return "services"
}

func (entity *service) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.Name, "name")
	ctx.setJsonValue(entityData, entity.terminatorStrategy, "terminatorStrategy")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")
	ctx.setJsonValue(entityData, entity.configs, "configs")
	ctx.setJsonValue(entityData, entity.encryptionRequired, "encryptionRequired")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}

	return entityData.String()
}

func (entity *service) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.Name, path("name"))
	ctx.pathEquals(c, entity.terminatorStrategy, path("terminatorStrategy"))
	ctx.pathEquals(c, entity.tags, path("tags"))

	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))

	sort.Strings(entity.permissions)
	ctx.pathEqualsStringSlice(c, entity.permissions, path("permissions"))
}

type terminator struct {
	id         string
	serviceId  string
	routerId   string
	binding    string
	address    string
	cost       int
	precedence string
	tags       map[string]interface{}
}

func (entity *terminator) getId() string {
	return entity.id
}

func (entity *terminator) setId(id string) {
	entity.id = id
}

func (entity *terminator) getEntityType() string {
	return "terminators"
}

func (entity *terminator) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.serviceId, "service")
	ctx.setJsonValue(entityData, entity.routerId, "router")
	ctx.setJsonValue(entityData, entity.binding, "binding")
	ctx.setJsonValue(entityData, entity.address, "address")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}

	return entityData.String()
}

func (entity *terminator) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.serviceId, path("serviceId"))
	ctx.pathEquals(c, entity.routerId, path("routerId"))
	ctx.pathEquals(c, entity.binding, path("binding"))
	ctx.pathEquals(c, entity.address, path("address"))
	ctx.pathEquals(c, float64(entity.cost), path("cost"))
	ctx.pathEquals(c, entity.precedence, path("precedence"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func (entity *terminator) fromJson(ctx *TestContext, c *gabs.Container) {
	entity.id = ctx.requireString(c, "id")
	entity.serviceId = ctx.requireString(c, "serviceId")
	entity.routerId = ctx.requireString(c, "routerId")
	entity.binding = ctx.requireString(c, "binding")
	entity.address = ctx.requireString(c, "address")
	entity.precedence = ctx.requireString(c, "precedence")
	entity.cost = ctx.requireInt(c, "cost")
}

func newTestIdentity(isAdmin bool, roleAttributes ...string) *identity {
	return &identity{
		name:           eid.New(),
		identityType:   "User",
		isAdmin:        isAdmin,
		roleAttributes: roleAttributes,
	}
}

type identity struct {
	Id                        string
	name                      string
	identityType              string
	isAdmin                   bool
	enrollment                map[string]interface{}
	roleAttributes            []string
	tags                      map[string]interface{}
	defaultHostingPrecedence  string
	defaultHostingCost        int
	serviceHostingPrecedences map[string]interface{}
	serviceHostingCosts       map[string]uint16
	config                    *config.Config
	authPolicyId              string
}

func (entity *identity) getId() string {
	return entity.Id
}

func (entity *identity) setId(id string) {
	entity.Id = id
}

func (entity *identity) getEntityType() string {
	return "identities"
}

func (entity *identity) toJson(isCreate bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.identityType, "type")
	ctx.setJsonValue(entityData, entity.isAdmin, "isAdmin")
	ctx.setJsonValue(entityData, entity.enrollment, "enrollment")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")
	if entity.defaultHostingPrecedence != "" {
		ctx.setJsonValue(entityData, entity.defaultHostingPrecedence, "defaultHostingPrecedence")
	}
	if entity.defaultHostingCost != 0 {
		ctx.setJsonValue(entityData, entity.defaultHostingCost, "defaultHostingCost")
	}
	ctx.setJsonValue(entityData, entity.serviceHostingPrecedences, "serviceHostingPrecedences")
	ctx.setJsonValue(entityData, entity.serviceHostingCosts, "serviceHostingCosts")
	ctx.setJsonValue(entityData, entity.authPolicyId, "authPolicyId")

	if isCreate {
		if entity.enrollment == nil {
			enrollments := map[string]interface{}{
				"updb": entity.name,
			}
			ctx.setJsonValue(entityData, enrollments, "enrollment")
		}
	}

	ctx.setJsonValue(entityData, entity.tags, "tags")

	return entityData.String()
}

func (entity *identity) getCompareServiceHostingsCosts() map[string]interface{} {
	if entity.serviceHostingCosts == nil {
		return nil
	}
	result := map[string]interface{}{}
	for k, v := range entity.serviceHostingCosts {
		result[k] = float64(v)
	}
	return result
}

func (entity *identity) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	if entity.serviceHostingCosts == nil {
		entity.serviceHostingCosts = map[string]uint16{}
	}
	if entity.serviceHostingPrecedences == nil {
		entity.serviceHostingPrecedences = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	if entity.defaultHostingPrecedence != "" {
		ctx.pathEquals(c, entity.defaultHostingPrecedence, path("defaultHostingPrecedence"))
	} else {
		ctx.pathEquals(c, "default", path("defaultHostingPrecedence"))
	}

	ctx.pathEquals(c, entity.defaultHostingCost, path("defaultHostingCost"))

	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
	ctx.pathEquals(c, entity.serviceHostingPrecedences, path("serviceHostingPrecedences"))
	ctx.pathEquals(c, entity.getCompareServiceHostingsCosts(), path("serviceHostingCosts"))
}

func (entity *identity) fromJson(ctx *TestContext, c *gabs.Container) {
	entity.Id = ctx.requireString(c, "id")
	entity.identityType = ctx.requireString(c, "type", "id")
	entity.isAdmin = ctx.requireBool(c, "isAdmin")
	entity.name = ctx.requireString(c, "name")
	entity.roleAttributes = ctx.requireStringSlice(c, "roleAttributes")
}

func newTestEdgeRouter(roleAttributes ...string) *edgeRouter {
	return &edgeRouter{
		name:           eid.New(),
		roleAttributes: roleAttributes,
	}
}

type edgeRouter struct {
	id                string
	name              string
	isTunnelerEnabled bool
	roleAttributes    []string
	tags              map[string]interface{}
}

func (entity *edgeRouter) getId() string {
	return entity.id
}

func (entity *edgeRouter) setId(id string) {
	entity.id = id
}

func (entity *edgeRouter) getEntityType() string {
	return "edge-routers"
}

func (entity *edgeRouter) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.roleAttributes, "roleAttributes")
	ctx.setJsonValue(entityData, entity.isTunnelerEnabled, "isTunnelerEnabled")
	ctx.setJsonValue(entityData, entity.tags, "tags")

	return entityData.String()
}

func (entity *edgeRouter) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.isTunnelerEnabled, path("isTunnelerEnabled"))
	sort.Strings(entity.roleAttributes)
	ctx.pathEqualsStringSlice(c, entity.roleAttributes, path("roleAttributes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newEdgeRouterPolicy(semantic string, edgeRouterRoles, identityRoles []string) *edgeRouterPolicy {
	return &edgeRouterPolicy{
		name:            eid.New(),
		semantic:        semantic,
		edgeRouterRoles: edgeRouterRoles,
		identityRoles:   identityRoles,
	}
}

type edgeRouterPolicy struct {
	id              string
	name            string
	semantic        string
	edgeRouterRoles []string
	identityRoles   []string
	tags            map[string]interface{}
}

func (entity *edgeRouterPolicy) getId() string {
	return entity.id
}

func (entity *edgeRouterPolicy) setId(id string) {
	entity.id = id
}

func (entity *edgeRouterPolicy) getEntityType() string {
	return "edge-router-policies"
}

func (entity *edgeRouterPolicy) toJson(_ bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.name, fields, "name")
	ctx.setValue(entityData, entity.semantic, fields, "semantic")
	ctx.setValue(entityData, entity.edgeRouterRoles, fields, "edgeRouterRoles")
	ctx.setValue(entityData, entity.identityRoles, fields, "identityRoles")
	ctx.setValue(entityData, entity.tags, fields, "tags")

	return entityData.String()
}

func (entity *edgeRouterPolicy) fromJson(ctx *TestContext, c *gabs.Container) {
	entity.id = ctx.requireString(c, "id")
	entity.name = ctx.requireString(c, "name")
	entity.semantic = ctx.requireString(c, "semantic")
	entity.edgeRouterRoles = ctx.requireStringSlice(c, "edgeRouterRoles")
	entity.identityRoles = ctx.requireStringSlice(c, "identityRoles")
}

func (entity *edgeRouterPolicy) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.semantic, path("semantic"))
	sort.Strings(entity.edgeRouterRoles)
	ctx.pathEqualsStringSlice(c, entity.edgeRouterRoles, path("edgeRouterRoles"))
	sort.Strings(entity.identityRoles)
	ctx.pathEqualsStringSlice(c, entity.identityRoles, path("identityRoles"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newServiceEdgeRouterPolicy(semantic string, edgeRouterRoles, serviceRoles []string) *serviceEdgeRouterPolicy {
	return &serviceEdgeRouterPolicy{
		name:            eid.New(),
		semantic:        semantic,
		edgeRouterRoles: edgeRouterRoles,
		serviceRoles:    serviceRoles,
	}
}

type serviceEdgeRouterPolicy struct {
	id              string
	name            string
	semantic        string
	edgeRouterRoles []string
	serviceRoles    []string
	tags            map[string]interface{}
}

func (entity *serviceEdgeRouterPolicy) getId() string {
	return entity.id
}

func (entity *serviceEdgeRouterPolicy) setId(id string) {
	entity.id = id
}

func (entity *serviceEdgeRouterPolicy) getEntityType() string {
	return "service-edge-router-policies"
}

func (entity *serviceEdgeRouterPolicy) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.semantic, "semantic")
	ctx.setJsonValue(entityData, entity.edgeRouterRoles, "edgeRouterRoles")
	ctx.setJsonValue(entityData, entity.serviceRoles, "serviceRoles")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *serviceEdgeRouterPolicy) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.semantic, path("semantic"))
	sort.Strings(entity.edgeRouterRoles)
	ctx.pathEqualsStringSlice(c, entity.edgeRouterRoles, path("edgeRouterRoles"))
	sort.Strings(entity.serviceRoles)
	ctx.pathEqualsStringSlice(c, entity.serviceRoles, path("serviceRoles"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func newServicePolicy(policyType string, semantic string, serviceRoles, identityRoles, postureCheckRoles []string) *servicePolicy {
	return &servicePolicy{
		name:              eid.New(),
		policyType:        policyType,
		semantic:          semantic,
		serviceRoles:      serviceRoles,
		identityRoles:     identityRoles,
		postureCheckRoles: postureCheckRoles,
	}
}

type servicePolicy struct {
	id                string
	name              string
	policyType        string
	semantic          string
	identityRoles     []string
	serviceRoles      []string
	tags              map[string]interface{}
	postureCheckRoles []string
}

func (entity *servicePolicy) getId() string {
	return entity.id
}

func (entity *servicePolicy) setId(id string) {
	entity.id = id
}

func (entity *servicePolicy) getEntityType() string {
	return "service-policies"
}

func (entity *servicePolicy) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.policyType, "type")
	ctx.setJsonValue(entityData, entity.semantic, "semantic")
	ctx.setJsonValue(entityData, entity.identityRoles, "identityRoles")
	ctx.setJsonValue(entityData, entity.serviceRoles, "serviceRoles")
	ctx.setJsonValue(entityData, entity.postureCheckRoles, "postureCheckRoles")

	if len(entity.tags) > 0 {
		ctx.setJsonValue(entityData, entity.tags, "tags")
	}
	return entityData.String()
}

func (entity *servicePolicy) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.policyType, path("type"))
	ctx.pathEquals(c, entity.semantic, path("semantic"))
	sort.Strings(entity.identityRoles)
	ctx.pathEqualsStringSlice(c, entity.identityRoles, path("identityRoles"))
	sort.Strings(entity.serviceRoles)
	ctx.pathEqualsStringSlice(c, entity.serviceRoles, path("serviceRoles"))
	sort.Strings(entity.postureCheckRoles)
	ctx.pathEqualsStringSlice(c, entity.postureCheckRoles, path("postureCheckRoles"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type Config struct {
	Id           string
	ConfigTypeId string
	Name         string
	Data         map[string]interface{}
	Tags         map[string]interface{}
	sendType     bool
}

func (entity *Config) getId() string {
	return entity.Id
}

func (entity *Config) setId(id string) {
	entity.Id = id
}

func (entity *Config) getEntityType() string {
	return "configs"
}

func (entity *Config) toJson(isCreate bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.Name, fields, "name")
	if isCreate || entity.sendType {
		ctx.setValue(entityData, entity.ConfigTypeId, fields, "configTypeId")
	}
	ctx.setValue(entityData, entity.Data, fields, "data")
	ctx.setValue(entityData, entity.Tags, fields, "tags")
	return entityData.String()
}

func (entity *Config) validate(ctx *TestContext, c *gabs.Container) {
	if entity.Tags == nil {
		entity.Tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.Name, path("name"))
	ctx.pathEquals(c, entity.ConfigTypeId, path("configTypeId"))
	ctx.pathEquals(c, entity.Data, path("data"))
	ctx.pathEquals(c, entity.Tags, path("tags"))
}

type configType struct {
	Id     string
	Name   string
	Schema map[string]interface{}
	Tags   map[string]interface{}
}

func (entity *configType) getId() string {
	return entity.Id
}

func (entity *configType) setId(id string) {
	entity.Id = id
}

func (entity *configType) getEntityType() string {
	return "config-types"
}

func (entity *configType) toJson(_ bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.Name, fields, "name")
	ctx.setValue(entityData, entity.Schema, fields, "schema")
	ctx.setValue(entityData, entity.Tags, fields, "tags")
	return entityData.String()
}

func (entity *configType) validate(ctx *TestContext, c *gabs.Container) {
	if entity.Tags == nil {
		entity.Tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.Name, path("name"))
	ctx.pathEquals(c, entity.Schema, path("schema"))
	ctx.pathEquals(c, entity.Tags, path("tags"))
}

type apiSession struct {
	id          string
	token       string
	identityId  string
	configTypes []string
	tags        map[string]interface{}
}

func (entity *apiSession) getId() string {
	return entity.id
}

func (entity *apiSession) setId(id string) {
	entity.id = id
}

func (entity *apiSession) getEntityType() string {
	return "apiSessions"
}

func (entity *apiSession) toJson(_ bool, ctx *TestContext, _ ...string) string {
	ctx.Req.FailNow("should not be called")
	return ""
}

func (entity *apiSession) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.token, path("token"))
	ctx.pathEquals(c, entity.identityId, path("identity", "id"))
	ctx.pathEquals(c, entity.configTypes, path("configTypes"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type configValidatingService struct {
	*service
	configs map[string]*Config
}

func (entity *configValidatingService) validate(ctx *TestContext, c *gabs.Container) {
	configs := c.Path("config")
	if len(entity.configs) == 0 && configs == nil {
		return
	}

	children, err := configs.Children()
	ctx.Req.NoError(err)
	ctx.Req.Equal(len(entity.configs), len(children))
	for configType, cfg := range entity.configs {
		ctx.pathEquals(configs, cfg.Data, s(configType))
	}
}

func newTestTransitRouter() *transitRouter {
	return &transitRouter{
		name: eid.New(),
	}
}

type transitRouter struct {
	id   string
	name string
	tags map[string]interface{}
}

func (entity *transitRouter) getId() string {
	return entity.id
}

func (entity *transitRouter) setId(id string) {
	entity.id = id
}

func (entity *transitRouter) getEntityType() string {
	return "transit-routers"
}

func (entity *transitRouter) toJson(_ bool, ctx *TestContext, _ ...string) string {
	entityData := gabs.New()
	ctx.setJsonValue(entityData, entity.name, "name")
	ctx.setJsonValue(entityData, entity.tags, "tags")

	return entityData.String()
}

func (entity *transitRouter) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

type ca struct {
	id                        string
	name                      string                 `json:"name"`
	isAutoCaEnrollmentEnabled bool                   `json:"isAutoCaEnrollmentEnabled"`
	isAuthEnabled             bool                   `json:"isAuthEnabled"`
	isOttCaEnrollmentEnabled  bool                   `json:"isOttCaEnrollmentEnabled"`
	certPem                   string                 `json:"certPem"`
	identityRoles             []string               `json:"identityRoles"`
	identityNameFormat        string                 `json:"identityNameFormat"`
	tags                      map[string]interface{} `json:"tags"`
	externalIdClaim           *externalIdClaim       `json:"externalIdClaim"`

	privateKey crypto.Signer     `json:"-"` //utility property, not used in API calls
	publicCert *x509.Certificate `json:"-"` //utility property, not used in API calls
}

type externalIdClaim struct {
	location        string `json:"location"`
	matcher         string `json:"matcher"`
	matcherCriteria string `json:"matcherCriteria"`
	parser          string `json:"parser"`
	parserCriteria  string `json:"parserCriteria"`
	index           int64  `json:"index"`
}

func newTestCaCert() (*x509.Certificate, *ecdsa.PrivateKey, *bytes.Buffer) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	caCert := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization: []string{"Ziti Dev"},
			Country:      []string{"US"},
			Province:     []string{"Anywhere"},
			Locality:     []string{"Anytime"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, 1),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caCert, caCert, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	caCert, err = x509.ParseCertificate(caBytes)

	caPEM := new(bytes.Buffer)
	_ = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	return caCert, key, caPEM
}

func newTestCa(identityRoles ...string) *ca {
	caCert, key, caPEM := newTestCaCert()

	if identityRoles == nil {
		identityRoles = []string{}
	}

	return &ca{
		name:                      eid.New(),
		isAutoCaEnrollmentEnabled: true,
		isAuthEnabled:             true,
		isOttCaEnrollmentEnabled:  true,
		certPem:                   caPEM.String(),
		identityRoles:             identityRoles,
		identityNameFormat:        "[caName]-[commonName]-[requestedName]",
		tags:                      map[string]interface{}{},
		privateKey:                key,
		publicCert:                caCert,
	}
}

func (entity ca) getId() string {
	return entity.id
}

func (entity ca) setId(id string) {
	entity.id = id
}

func (entity ca) getEntityType() string {
	return "cas"
}

func (entity ca) toJson(create bool, ctx *TestContext, fields ...string) string {
	entityData := gabs.New()
	ctx.setValue(entityData, entity.name, fields, "name")
	ctx.setValue(entityData, entity.isOttCaEnrollmentEnabled, fields, "isOttCaEnrollmentEnabled")
	ctx.setValue(entityData, entity.isAutoCaEnrollmentEnabled, fields, "isAutoCaEnrollmentEnabled")
	ctx.setValue(entityData, entity.isAuthEnabled, fields, "isAuthEnabled")
	ctx.setValue(entityData, entity.identityRoles, fields, "identityRoles")
	ctx.setValue(entityData, entity.tags, fields, "tags")
	ctx.setValue(entityData, entity.identityNameFormat, fields, "identityNameFormat")

	if entity.externalIdClaim != nil {
		ctx.setValueWithPath(entityData, entity.externalIdClaim.location, fields, "externalIdClaim", "externalIdClaim", "location")
		ctx.setValueWithPath(entityData, entity.externalIdClaim.index, fields, "externalIdClaim", "externalIdClaim", "index")
		ctx.setValueWithPath(entityData, entity.externalIdClaim.matcher, fields, "externalIdClaim", "externalIdClaim", "matcher")
		ctx.setValueWithPath(entityData, entity.externalIdClaim.matcherCriteria, fields, "externalIdClaim", "externalIdClaim", "matcherCriteria")
		ctx.setValueWithPath(entityData, entity.externalIdClaim.parser, fields, "externalIdClaim", "externalIdClaim", "parser")
		ctx.setValueWithPath(entityData, entity.externalIdClaim.parserCriteria, fields, "externalIdClaim", "externalIdClaim", "parserCriteria")
	}

	if create {
		ctx.setValue(entityData, entity.certPem, fields, "certPem")
	}

	return entityData.String()
}

func (entity ca) validate(ctx *TestContext, c *gabs.Container) {
	if entity.tags == nil {
		entity.tags = map[string]interface{}{}
	}
	ctx.pathEquals(c, entity.name, path("name"))
	sort.Strings(entity.identityRoles)
	ctx.pathEqualsStringSlice(c, entity.identityRoles, path("identityRoles"))
	ctx.pathEquals(c, entity.certPem, path("certPem"))
	ctx.pathEquals(c, entity.isAuthEnabled, path("isAuthEnabled"))
	ctx.pathEquals(c, entity.isAutoCaEnrollmentEnabled, path("isAutoCaEnrollmentEnabled"))
	ctx.pathEquals(c, entity.isOttCaEnrollmentEnabled, path("isOttCaEnrollmentEnabled"))
	ctx.pathEquals(c, entity.identityNameFormat, path("identityNameFormat"))
	ctx.pathEquals(c, entity.tags, path("tags"))
}

func (entity ca) CreateSignedCert(name string) *certAuthenticator {
	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}

	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   name,
			Organization: []string{"Ziti Dev"},
			Country:      []string{"US"},
			Province:     []string{"Anywhere"},
			Locality:     []string{"Anytime"},
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, clientKey)
	if err != nil {
		panic(err)
	}

	csr, err := x509.ParseCertificateRequest(csrBytes)

	if err != nil {
		panic(err)
	}

	if err = csr.CheckSignature(); err != nil {
		panic(err)
	}

	certTemplate := x509.Certificate{
		Signature: csr.Signature,

		PublicKeyAlgorithm: csr.PublicKeyAlgorithm,
		PublicKey:          csr.PublicKey,

		SerialNumber: big.NewInt(2020),
		Issuer:       entity.publicCert.Subject,
		Subject:      csr.Subject,
		NotBefore:    time.Now().AddDate(0, 0, -1),
		NotAfter:     time.Now().AddDate(1, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:         false,
	}

	clientBytes, err := x509.CreateCertificate(rand.Reader, &certTemplate, entity.publicCert, csr.PublicKey, entity.privateKey)

	if err != nil {
		panic(err)
	}

	clientCert, err := x509.ParseCertificate(clientBytes)

	if err != nil {
		panic(err)
	}

	clientPEM := new(bytes.Buffer)
	_ = pem.Encode(clientPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: clientBytes,
	})

	return &certAuthenticator{
		cert:    clientCert,
		key:     clientKey,
		certPem: clientPEM.String(),
	}
}
