package tests

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/certificate_authority"
	managementCurrentApiSession "github.com/openziti/edge-api/rest_management_api_client/current_api_session"
	managementCurrentIdentity "github.com/openziti/edge-api/rest_management_api_client/current_identity"
	managementEnrollment "github.com/openziti/edge-api/rest_management_api_client/enrollment"
	managementIdentity "github.com/openziti/edge-api/rest_management_api_client/identity"
	managementPostureChecks "github.com/openziti/edge-api/rest_management_api_client/posture_checks"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	edgeApis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/ziti/common/eid"
	"github.com/openziti/ziti/ziti/util"
)

type ManagementHelperClient struct {
	*edgeApis.ManagementApiClient
	testCtx *TestContext
}

func (helper *ManagementHelperClient) CreateAndEnrollOttIdentity(isAdmin bool, roleAttributes ...string) (*rest_model.IdentityDetail, *edgeApis.CertCredentials, error) {
	idLoc, err := helper.CreateIdentity(uuid.NewString(), isAdmin, roleAttributes...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create identity: %w", err)
	}
	expiresAt := time.Now().Add(1 * time.Hour)

	ottEnrollLoc, err := helper.CreateEnrollmentOtt(&idLoc.ID, &expiresAt)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ott enrollment: %w", err)
	}

	ottEnrollment, err := helper.GetEnrollment(ottEnrollLoc.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get enrollment: %w", err)
	}

	clientClient := helper.testCtx.NewEdgeClientApi(nil)
	ottCreds, err := clientClient.CompleteOttEnrollment(*ottEnrollment.Token)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to complete ott enrollment: %w", err)
	}

	identityDetail, err := helper.GetIdentity(idLoc.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get identity: %w", err)
	}

	return identityDetail, ottCreds, nil
}

func (helper *ManagementHelperClient) CreateIdentity(name string, isAdmin bool, roleAttributes ...string) (*rest_model.CreateLocation, error) {
	newIdentity := &rest_model.IdentityCreate{
		Name:           ToPtr(name),
		Type:           ToPtr(rest_model.IdentityTypeDefault),
		IsAdmin:        ToPtr(isAdmin),
		RoleAttributes: ToPtr(rest_model.Attributes(roleAttributes)),
	}

	newIdentityParams := &managementIdentity.CreateIdentityParams{
		Identity: newIdentity,
	}

	resp, err := helper.API.Identity.CreateIdentity(newIdentityParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) GetIdentity(identityId string) (*rest_model.IdentityDetail, error) {
	getParams := &managementIdentity.DetailIdentityParams{
		ID: identityId,
	}

	resp, err := helper.API.Identity.DetailIdentity(getParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) CreateEnrollmentOtt(identityId *string, expiresAt *time.Time) (*rest_model.CreateLocation, error) {
	var expAt *strfmt.DateTime

	if expiresAt != nil {
		expAt = ToPtr(strfmt.DateTime(*expiresAt))
	}

	createParams := &managementEnrollment.CreateEnrollmentParams{
		Enrollment: &rest_model.EnrollmentCreate{
			IdentityID: identityId,
			ExpiresAt:  expAt,
			Method:     ToPtr(rest_model.EnrollmentCreateMethodOtt),
		},
	}

	resp, err := helper.API.Enrollment.CreateEnrollment(createParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) GetEnrollment(enrollmentId string) (*rest_model.EnrollmentDetail, error) {
	getParams := &managementEnrollment.DetailEnrollmentParams{
		ID: enrollmentId,
	}

	resp, err := helper.API.Enrollment.DetailEnrollment(getParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) CreateCa(testCa *ca) (*rest_model.CreateLocation, error) {
	caCreate := &rest_model.CaCreate{
		CertPem:                   &testCa.certPem,
		IdentityNameFormat:        testCa.identityNameFormat,
		IdentityRoles:             testCa.identityRoles,
		IsAuthEnabled:             &testCa.isAuthEnabled,
		IsAutoCaEnrollmentEnabled: &testCa.isAutoCaEnrollmentEnabled,
		IsOttCaEnrollmentEnabled:  &testCa.isOttCaEnrollmentEnabled,
		Name:                      &testCa.name,
	}

	createParams := &certificate_authority.CreateCaParams{
		Ca: caCreate,
	}

	resp, err := helper.API.CertificateAuthority.CreateCa(createParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) CreateEnrollmentOttCa(identityId *string, caId *string, expiresAt *time.Time) (*rest_model.CreateLocation, error) {
	var expAt *strfmt.DateTime

	if expiresAt != nil {
		expAt = ToPtr(strfmt.DateTime(*expiresAt))
	}

	createParams := &managementEnrollment.CreateEnrollmentParams{
		Enrollment: &rest_model.EnrollmentCreate{
			IdentityID: identityId,
			ExpiresAt:  expAt,
			CaID:       caId,
			Method:     ToPtr(rest_model.EnrollmentCreateMethodOttca),
		},
	}

	resp, err := helper.API.Enrollment.CreateEnrollment(createParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) GetCa(caId string) (*rest_model.CaDetail, error) {
	getParams := &certificate_authority.DetailCaParams{
		ID: caId,
	}

	resp, err := helper.API.CertificateAuthority.DetailCa(getParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) VerifyCa(id string, token string, cert *x509.Certificate, key crypto.Signer) error {
	verifyCert, _, err := generateCaSignedClientCert(cert, key, token)

	if err != nil {
		return fmt.Errorf("could not generate verification certificate: %w", err)
	}

	verificationBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: verifyCert.Raw,
	}
	verifyPem := pem.EncodeToMemory(verificationBlock)

	verifyParams := &certificate_authority.VerifyCaParams{
		Certificate: string(verifyPem),
		ID:          id,
	}

	_, err = helper.API.CertificateAuthority.VerifyCa(verifyParams, nil)

	if err != nil {
		return fmt.Errorf("could not verify certificate: %w", rest_util.WrapErr(err))
	}

	return nil
}

func (helper *ManagementHelperClient) CreateEnrollmentUpdb(identityId *string, username *string, expiresAt *time.Time) (*rest_model.CreateLocation, error) {
	var expAt *strfmt.DateTime

	if expiresAt != nil {
		expAt = ToPtr(strfmt.DateTime(*expiresAt))
	}

	createParams := &managementEnrollment.CreateEnrollmentParams{
		Enrollment: &rest_model.EnrollmentCreate{
			IdentityID: identityId,
			ExpiresAt:  expAt,
			Username:   username,
			Method:     ToPtr(rest_model.EnrollmentCreateMethodUpdb),
		},
	}

	resp, err := helper.API.Enrollment.CreateEnrollment(createParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) GetIdentityAuthenticators(identityId string) ([]*rest_model.AuthenticatorDetail, error) {
	getIdAuthenticatorsParams := managementIdentity.NewGetIdentityAuthenticatorsParams()
	getIdAuthenticatorsParams.ID = identityId
	resp, err := helper.API.Identity.GetIdentityAuthenticators(getIdAuthenticatorsParams, nil)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve identity authenticators: %w", rest_util.WrapErr(err))
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) RefreshEnrollmentJwt(id string) (*rest_model.EnrollmentDetail, error) {
	refreshParams := &managementEnrollment.RefreshEnrollmentParams{
		ID: id,
		Refresh: &rest_model.EnrollmentRefresh{
			ExpiresAt: ToPtr(strfmt.DateTime(time.Now().Add(time.Hour * 24))),
		},
	}
	_, err := helper.API.Enrollment.RefreshEnrollment(refreshParams, nil)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	detail, err := helper.GetEnrollment(id)

	if err != nil {
		return nil, err
	}

	return detail, nil
}

func (helper *ManagementHelperClient) GetTotpMfa() (*rest_model.DetailMfa, error) {
	params := &managementCurrentIdentity.DetailMfaParams{}

	resp, err := helper.API.CurrentIdentity.DetailMfa(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not get totp mfa: %w", util.WrapIfApiError(err))
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) CreateTotpMfaEnrollment() (*rest_model.DetailMfa, error) {
	params := &managementCurrentIdentity.EnrollMfaParams{}

	_, err := helper.API.CurrentIdentity.EnrollMfa(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not create totp mfa enrollment: %w", util.WrapIfApiError(err))
	}

	return helper.GetTotpMfa()
}

func (helper *ManagementHelperClient) VerifyTotpMfaEnrollment(code string) (*rest_model.DetailMfa, error) {
	params := &managementCurrentIdentity.VerifyMfaParams{
		MfaValidation: &rest_model.MfaCode{
			Code: ToPtr(code),
		},
	}

	_, err := helper.API.CurrentIdentity.VerifyMfa(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not verify totp mfa enrollment: %w", util.WrapIfApiError(err))
	}

	return helper.GetTotpMfa()
}

func (helper *ManagementHelperClient) EnrollTotpMfa() (*TotpProvider, *rest_model.DetailMfa, error) {
	totpMfaEnrollment, err := helper.CreateTotpMfaEnrollment()

	if err != nil {
		return nil, nil, fmt.Errorf("could not create totp mfa enrollment: %w", err)
	}

	totpProvider := &TotpProvider{}
	err = totpProvider.ApplyProvisioningUrl(totpMfaEnrollment.ProvisioningURL)

	if err != nil {
		return nil, nil, fmt.Errorf("could not apply provisioning url: %w", err)
	}

	curCode := totpProvider.Code()

	totpMfaEnrollment, err = helper.VerifyTotpMfaEnrollment(curCode)

	if err != nil {
		return nil, nil, fmt.Errorf("could not verify totp mfa enrollment: %w", err)
	}

	return totpProvider, totpMfaEnrollment, err
}

func (helper *ManagementHelperClient) GetTotpToken(code string) (*rest_model.TotpToken, error) {
	params := managementCurrentApiSession.CreateTotpTokenParams{
		MfaValidation: &rest_model.MfaCode{
			Code: ToPtr(code),
		},
	}

	resp, err := helper.API.CurrentAPISession.CreateTotpToken(&params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not get totp token: %w", util.WrapIfApiError(err))
	}

	return resp.Payload.Data, nil
}

func (helper *ManagementHelperClient) GetPostureCheck(id string) (rest_model.PostureCheckDetail, error) {
	detailParams := managementPostureChecks.NewDetailPostureCheckParams()
	detailParams.ID = id

	detailResp, err := helper.API.PostureChecks.DetailPostureCheck(detailParams, nil)
	if err != nil {
		return nil, util.WrapIfApiError(err)
	}

	return detailResp.Payload.Data(), nil
}

func (helper *ManagementHelperClient) CreatePostureCheck(check rest_model.PostureCheckCreate) (rest_model.PostureCheckDetail, error) {
	createParams := managementPostureChecks.NewCreatePostureCheckParams()
	createParams.PostureCheck = check

	createResp, err := helper.API.PostureChecks.CreatePostureCheck(createParams, nil)

	if err != nil {
		return nil, fmt.Errorf("could not create posture check: %w", util.WrapIfApiError(err))
	}

	postureCheckDetail, err := helper.GetPostureCheck(createResp.Payload.Data.ID)

	if err != nil {
		return nil, fmt.Errorf("could not get posture check detail: %w", util.WrapIfApiError(err))
	}

	return postureCheckDetail, nil
}

func (helper *ManagementHelperClient) CreatePostureCheckOs(operatingSystems []*rest_model.OperatingSystem, attributes []string) (*rest_model.PostureCheckOperatingSystemDetail, error) {
	newCheck := &rest_model.PostureCheckOperatingSystemCreate{
		OperatingSystems: operatingSystems,
	}

	attrs := rest_model.Attributes(attributes)
	newCheck.SetRoleAttributes(&attrs)
	newCheck.SetName(ToPtr(eid.New()))

	postureCheckDetail, err := helper.CreatePostureCheck(newCheck)

	if err != nil {
		return nil, err
	}

	checkDetail, ok := postureCheckDetail.(*rest_model.PostureCheckOperatingSystemDetail)

	if !ok {
		return nil, fmt.Errorf("posture check detail is not the right type, expected %T, got %T", checkDetail, postureCheckDetail)
	}

	return checkDetail, nil
}

func (helper *ManagementHelperClient) CreatePostureCheckMac(addresses []string, attributes []string) (*rest_model.PostureCheckMacAddressDetail, error) {
	newCheck := &rest_model.PostureCheckMacAddressCreate{
		MacAddresses: addresses,
	}

	attrs := rest_model.Attributes(attributes)
	newCheck.SetRoleAttributes(&attrs)
	newCheck.SetName(ToPtr(eid.New()))

	postureCheckDetail, err := helper.CreatePostureCheck(newCheck)

	if err != nil {
		return nil, err
	}

	checkDetail, ok := postureCheckDetail.(*rest_model.PostureCheckMacAddressDetail)

	if !ok {
		return nil, fmt.Errorf("posture check detail is not the right type, expected %T, got %T", checkDetail, postureCheckDetail)
	}

	return checkDetail, nil
}

func (helper *ManagementHelperClient) CreatePostureCheckProcessMulti(processes []*rest_model.ProcessMulti, semantic rest_model.Semantic, attributes []string) (*rest_model.PostureCheckProcessMultiDetail, error) {
	newCheck := &rest_model.PostureCheckProcessMultiCreate{
		Processes: processes,
		Semantic:  &semantic,
	}

	attrs := rest_model.Attributes(attributes)
	newCheck.SetRoleAttributes(&attrs)
	newCheck.SetName(ToPtr(eid.New()))

	postureCheckDetail, err := helper.CreatePostureCheck(newCheck)

	if err != nil {
		return nil, err
	}

	checkDetail, ok := postureCheckDetail.(*rest_model.PostureCheckProcessMultiDetail)

	if !ok {
		return nil, fmt.Errorf("posture check detail is not the right type, expected %T, got %T", checkDetail, postureCheckDetail)
	}

	return checkDetail, nil
}

func (helper *ManagementHelperClient) CreatePostureCheckProcess(process *rest_model.Process, attributes []string) (*rest_model.PostureCheckProcessDetail, error) {
	newCheck := &rest_model.PostureCheckProcessCreate{
		Process: process,
	}

	attrs := rest_model.Attributes(attributes)
	newCheck.SetRoleAttributes(&attrs)
	newCheck.SetName(ToPtr(eid.New()))

	postureCheckDetail, err := helper.CreatePostureCheck(newCheck)

	if err != nil {
		return nil, err
	}

	checkDetail, ok := postureCheckDetail.(*rest_model.PostureCheckProcessDetail)

	if !ok {
		return nil, fmt.Errorf("posture check detail is not the right type, expected %T, got %T", checkDetail, postureCheckDetail)
	}

	return checkDetail, nil
}
