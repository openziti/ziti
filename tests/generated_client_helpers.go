package tests

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	clientEnroll "github.com/openziti/edge-api/rest_client_api_client/enroll"
	"github.com/openziti/edge-api/rest_management_api_client/certificate_authority"
	managementEnrollment "github.com/openziti/edge-api/rest_management_api_client/enrollment"
	managementIdentity "github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfPem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/identity/certtools"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"math/big"
	"strings"
	"time"
)

type ManagementHelperClient struct {
	*edge_apis.ManagementApiClient
	testCtx *TestContext
}

func (helper *ManagementHelperClient) CreateAndEnrollOttIdentity(isAdmin bool, roleAttributes ...string) (*rest_model.IdentityDetail, *edge_apis.CertCredentials, error) {
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
		return nil, fmt.Errorf("failed to retrieve identity authenticators: %w", err)
	}

	return resp.Payload.Data, nil
}

type ClientHelperClient struct {
	*edge_apis.ClientApiClient
	testCtx *TestContext
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

	if err != nil {
		return nil, err
	}

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

	if err != nil {
		return nil, err
	}

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

	if len(clientCert) == 0 {
		return nil, fmt.Errorf("no client certificates returned from enrollment")
	}

	var rawCerts [][]byte

	for _, cert := range clientCert {
		rawCerts = append(rawCerts, cert.Raw)
	}

	tlsCerts := []tls.Certificate{
		{
			Certificate: rawCerts,
			PrivateKey:  clientKey,
			Leaf:        clientCert[0],
		},
	}

	helper.HttpTransport.TLSClientConfig.Certificates = tlsCerts
	helper.HttpTransport.CloseIdleConnections()

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

func (helper *ClientHelperClient) ExtendCertsWithAuthenticatorId(authenticatorId string) (*edge_apis.CertCredentials, error) {
	request, err := certtools.NewCertRequest(map[string]string{
		"C": "US", "O": "NetFoundry-API-Test", "CN": uuid.NewString(),
	}, nil)

	if err != nil {
		return nil, fmt.Errorf("could not create base CSR values: %w", err)
	}

	newPrivateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("could not generate private key: %w", err)
	}

	csr, err := x509.CreateCertificateRequest(rand.Reader, request, newPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("could not create CSR: %w", err)
	}

	csrPem := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csr}))

	extendParams := &current_api_session.ExtendCurrentIdentityAuthenticatorParams{
		Extend: &rest_model.IdentityExtendEnrollmentRequest{
			ClientCertCsr: &csrPem,
		},
		ID: authenticatorId,
	}

	extendResp, err := helper.API.CurrentAPISession.ExtendCurrentIdentityAuthenticator(extendParams, nil)

	if err != nil {
		return nil, fmt.Errorf("could not extend current identity authenticator: %w", err)
	}

	caCerts := nfPem.PemStringToCertificates(extendResp.Payload.Data.Ca)
	caPool := x509.NewCertPool()

	for _, c := range caCerts {
		caPool.AddCert(c)
	}

	newCerts := nfPem.PemStringToCertificates(extendResp.Payload.Data.ClientCert)

	newCreds := &edge_apis.CertCredentials{
		BaseCredentials: edge_apis.BaseCredentials{
			CaPool: caPool,
		},
		Certs: newCerts,
		Key:   newPrivateKey,
	}

	verifyParams := &current_api_session.ExtendVerifyCurrentIdentityAuthenticatorParams{
		Extend: &rest_model.IdentityExtendValidateEnrollmentRequest{
			ClientCert: &extendResp.Payload.Data.ClientCert,
		},
		ID: authenticatorId,
	}

	_, err = helper.API.CurrentAPISession.ExtendVerifyCurrentIdentityAuthenticator(verifyParams, nil)

	if err != nil {
		return nil, fmt.Errorf("could not verify the extension of the authenticator: %w", err)
	}

	return newCreds, nil

}

func (helper *ClientHelperClient) GetCurrentApiSessionDetail() (*rest_model.CurrentAPISessionDetail, error) {
	params := &current_api_session.GetCurrentAPISessionParams{}

	resp, err := helper.API.CurrentAPISession.GetCurrentAPISession(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not get current api session detail: %w", err)
	}

	return resp.Payload.Data, nil
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
