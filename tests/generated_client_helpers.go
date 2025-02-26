package tests

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v5"
	clientEnroll "github.com/openziti/edge-api/rest_client_api_client/enroll"
	"github.com/openziti/edge-api/rest_management_api_client/certificate_authority"
	managementEnrollment "github.com/openziti/edge-api/rest_management_api_client/enrollment"
	managementIdentity "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfPem "github.com/openziti/foundation/v2/pem"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"math/big"
	"strings"
	"time"
)

type ManagementHelperClient struct {
	*edge_apis.ManagementApiClient
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

func (helper *ManagementHelperClient) NewEnrollmentOtt(identityId *string, expiresAt *time.Time) (*rest_model.CreateLocation, error) {
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

func (helper *ManagementHelperClient) NewEnrollmentOttCa(identityId *string, caId *string, expiresAt *time.Time) (*rest_model.CreateLocation, error) {
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

func (helper *ManagementHelperClient) NewEnrollmentUpdb(identityId *string, username *string, expiresAt *time.Time) (*rest_model.CreateLocation, error) {
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

type ClientHelperClient struct {
	*edge_apis.ClientApiClient
}

func (helper *ClientHelperClient) CompleteOttEnrollment(enrollmentToken string) (*edge_apis.CertCredentials, error) {
	token := enrollmentToken

	if IsJwt(token) {
		jwtParser := jwt.NewParser()
		enrollmentClaims := &ziti.EnrollmentClaims{}
		_, _, err := jwtParser.ParseUnverified(token, enrollmentClaims)

		if err != nil {
			return nil, fmt.Errorf("could not parse enrollment JWT: %w", rest_util.WrapErr(err))
		}

		token = enrollmentClaims.ID
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	template := &x509.CertificateRequest{}

	csr, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

	params := clientEnroll.NewEnrollOttParams()
	params.OttEnrollmentRequest = &rest_model.OttEnrollmentRequest{
		ClientCsr: string(csrPem),
		Token:     token,
	}

	resp, err := helper.API.Enroll.EnrollOtt(params)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	certs := nfPem.PemStringToCertificates(resp.Payload.Data.Cert)

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates returned from enrollment")
	}

	return &edge_apis.CertCredentials{
		BaseCredentials: edge_apis.BaseCredentials{},
		Certs:           certs,
		Key:             privateKey,
	}, nil
}

func (helper *ClientHelperClient) CompleteOttGenericEnrollment(enrollmentToken string) (*edge_apis.CertCredentials, error) {
	token := enrollmentToken

	if IsJwt(token) {
		jwtParser := jwt.NewParser()
		enrollmentClaims := &ziti.EnrollmentClaims{}
		_, _, err := jwtParser.ParseUnverified(token, enrollmentClaims)

		if err != nil {
			return nil, fmt.Errorf("could not parse enrollment JWT: %w", rest_util.WrapErr(err))
		}

		token = enrollmentClaims.ID
	}

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	template := &x509.CertificateRequest{}

	csr, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	csrPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr})

	params := clientEnroll.NewEnrollParams()
	params.Method = ToPtr(rest_model.EnrollmentCreateMethodOtt)
	params.Token = ToPtr(strfmt.UUID(token))
	params.Body = &rest_model.GenericEnroll{
		ClientCsr: string(csrPem),
	}

	resp, err := helper.API.Enroll.Enroll(params, func(operation *runtime.ClientOperation) {
		operation.ConsumesMediaTypes = []string{"text/plain"}
	})

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	certs := nfPem.PemStringToCertificates(resp.Payload.Data.Cert)

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates returned from enrollment")
	}

	return &edge_apis.CertCredentials{
		BaseCredentials: edge_apis.BaseCredentials{},
		Certs:           certs,
		Key:             privateKey,
	}, nil
}

func (helper *ClientHelperClient) CompleteOttCaEnrollment(enrollmentToken string, clientCert []*x509.Certificate, clientKey crypto.PrivateKey) (*edge_apis.CertCredentials, error) {
	token := enrollmentToken

	if IsJwt(token) {
		jwtParser := jwt.NewParser()
		enrollmentClaims := &ziti.EnrollmentClaims{}
		_, _, err := jwtParser.ParseUnverified(token, enrollmentClaims)

		if err != nil {
			return nil, fmt.Errorf("could not parse enrollment JWT: %w", rest_util.WrapErr(err))
		}

		token = enrollmentClaims.ID
	}

	params := clientEnroll.NewEnrollOttCaParams()
	params.OttEnrollmentRequest = &rest_model.OttEnrollmentRequest{
		Token: token,
	}

	certCreds := &edge_apis.CertCredentials{
		BaseCredentials: edge_apis.BaseCredentials{},
		Certs:           clientCert,
		Key:             clientKey,
	}

	helper.Credentials = certCreds

	_, err := helper.API.Enroll.EnrollOttCa(params)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return certCreds, nil
}

func (helper *ClientHelperClient) CompleteUpdbEnrollment(enrollmentToken string, username string, password string) (*edge_apis.UpdbCredentials, error) {
	token := enrollmentToken

	if IsJwt(token) {
		jwtParser := jwt.NewParser()
		enrollmentClaims := &ziti.EnrollmentClaims{}
		_, _, err := jwtParser.ParseUnverified(token, enrollmentClaims)

		if err != nil {
			return nil, fmt.Errorf("could not parse enrollment JWT: %w", rest_util.WrapErr(err))
		}

		token = enrollmentClaims.ID
	}

	params := clientEnroll.NewEnrollUpdbParams()
	params.Token = strfmt.UUID(token)
	params.UpdbCredentials = clientEnroll.EnrollUpdbBody{
		Password: rest_model.Password(password),
		Username: rest_model.Username(username),
	}

	_, err := helper.API.Enroll.EnrollUpdb(params)

	if err != nil {
		return nil, rest_util.WrapErr(err)
	}

	return &edge_apis.UpdbCredentials{
		BaseCredentials: edge_apis.BaseCredentials{},
		Username:        username,
		Password:        password,
	}, nil
}

func generateCaSignedClientCert(caCert *x509.Certificate, caSigner crypto.Signer, commonName string) (*x509.Certificate, crypto.Signer, error) {
	id, _ := rand.Int(rand.Reader, big.NewInt(100000000000000000))
	verificationCert := &x509.Certificate{
		SerialNumber: id,
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{"Ziti CLI Generated API Test Cert"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Minute * 10),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	verificationKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)

	if err != nil {
		return nil, nil, fmt.Errorf("could not generate private key for certificate (%v)", err)
	}

	signedCertBytes, err := x509.CreateCertificate(rand.Reader, verificationCert, caCert, verificationKey.Public(), caSigner)

	if err != nil {
		return nil, nil, fmt.Errorf("could not sign certificate with CA (%v)", err)
	}

	verificationCert, _ = x509.ParseCertificate(signedCertBytes)

	return verificationCert, verificationKey, nil
}

func IsJwt(token string) bool {
	if strings.HasPrefix(token, "eY") {
		parts := strings.Split(token, ",")
		return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
	}

	return false
}
