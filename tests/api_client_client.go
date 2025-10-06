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
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/dgryski/dgoogauth"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	clientCurrentApiSession "github.com/openziti/edge-api/rest_client_api_client/current_api_session"
	clientCurrentIdentity "github.com/openziti/edge-api/rest_client_api_client/current_identity"
	clientEnroll "github.com/openziti/edge-api/rest_client_api_client/enroll"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	nfPem "github.com/openziti/foundation/v2/pem"
	"github.com/openziti/identity/certtools"
	edgeApis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
)

type ClientHelperClient struct {
	*edgeApis.ClientApiClient
	testCtx *TestContext
}

func (helper *ClientHelperClient) CompleteOttEnrollment(enrollmentToken string) (*edgeApis.CertCredentials, error) {
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

	return &edgeApis.CertCredentials{
		BaseCredentials: edgeApis.BaseCredentials{},
		Certs:           certs,
		Key:             privateKey,
	}, nil
}

func (helper *ClientHelperClient) CompleteOttGenericEnrollment(enrollmentToken string) (*edgeApis.CertCredentials, error) {
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

	return &edgeApis.CertCredentials{
		BaseCredentials: edgeApis.BaseCredentials{},
		Certs:           certs,
		Key:             privateKey,
	}, nil
}

func (helper *ClientHelperClient) CompleteOttCaEnrollment(enrollmentToken string, clientCert []*x509.Certificate, clientKey crypto.PrivateKey) (*edgeApis.CertCredentials, error) {
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

	certCreds := &edgeApis.CertCredentials{
		BaseCredentials: edgeApis.BaseCredentials{},
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

func (helper *ClientHelperClient) CompleteUpdbEnrollment(enrollmentToken string, username string, password string) (*edgeApis.UpdbCredentials, error) {
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

	return &edgeApis.UpdbCredentials{
		BaseCredentials: edgeApis.BaseCredentials{},
		Username:        username,
		Password:        password,
	}, nil
}

func (helper *ClientHelperClient) ExtendCertsWithAuthenticatorId(authenticatorId string) (*edgeApis.CertCredentials, error) {
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

	extendParams := &clientCurrentApiSession.ExtendCurrentIdentityAuthenticatorParams{
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

	newCreds := &edgeApis.CertCredentials{
		BaseCredentials: edgeApis.BaseCredentials{
			CaPool: caPool,
		},
		Certs: newCerts,
		Key:   newPrivateKey,
	}

	verifyParams := &clientCurrentApiSession.ExtendVerifyCurrentIdentityAuthenticatorParams{
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
	params := &clientCurrentApiSession.GetCurrentAPISessionParams{}

	resp, err := helper.API.CurrentAPISession.GetCurrentAPISession(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not get current api session detail: %w", err)
	}

	return resp.Payload.Data, nil
}

func (helper *ClientHelperClient) GetTotpMfa() (*rest_model.DetailMfa, error) {
	params := &clientCurrentIdentity.DetailMfaParams{}

	resp, err := helper.API.CurrentIdentity.DetailMfa(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not get totp mfa: %w", err)
	}

	return resp.Payload.Data, nil
}

func (helper *ClientHelperClient) CreateTotpMfaEnrollment() (*rest_model.DetailMfa, error) {
	params := &clientCurrentIdentity.EnrollMfaParams{}

	_, err := helper.API.CurrentIdentity.EnrollMfa(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not create totp mfa enrollment: %w", err)
	}

	return helper.GetTotpMfa()
}

func (helper *ClientHelperClient) VerifyTotpMfaEnrollment(code string) (*rest_model.DetailMfa, error) {
	params := &clientCurrentIdentity.VerifyMfaParams{
		MfaValidation: &rest_model.MfaCode{
			Code: ToPtr(code),
		},
		Context:    nil,
		HTTPClient: nil,
	}

	_, err := helper.API.CurrentIdentity.VerifyMfa(params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not verify totp mfa enrollment: %w", err)
	}

	return helper.GetTotpMfa()
}

func (helper *ClientHelperClient) EnrollTotpMfa() (*TotpProvider, *rest_model.DetailMfa, error) {
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

func (helper *ClientHelperClient) GetTotpToken(code string) (*rest_model.TotpToken, error) {
	params := clientCurrentApiSession.CreateTotpTokenParams{
		MfaValidation: &rest_model.MfaCode{
			Code: ToPtr(code),
		},
	}

	resp, err := helper.API.CurrentAPISession.CreateTotpToken(&params, nil)

	if err != nil {
		return nil, fmt.Errorf("could not get totp token: %w", err)
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

type TotpProvider struct {
	Secret          string
	Issuer          string
	Name            string
	ProvisioningUrl string
}

func (t *TotpProvider) FuncProvider() func(chan string) {
	return func(ch chan string) {
		ch <- t.Code()
	}
}

func (t *TotpProvider) ApplyProvisioningUrl(provisioningUrl string) error {
	parsedUrl, err := url.Parse(provisioningUrl)

	if err != nil {
		return fmt.Errorf("could not parse provisioning url: %w", err)
	}

	if parsedUrl.Scheme != "otpauth" {
		return fmt.Errorf("provisioning url must be an otpauth url")
	}

	queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
	if err != nil {
		return fmt.Errorf("could not parse query params from provisioning url: %w", err)
	}

	secrets, ok := queryParams["secret"]

	if !ok {
		return errors.New("could not find secret in provisioning url")
	}

	if len(secrets) != 1 {
		return fmt.Errorf("expected 1 secret in provisioning url, got %d", len(secrets))
	}

	t.Secret = secrets[0]
	t.ProvisioningUrl = provisioningUrl

	return nil
}

func (t *TotpProvider) Code() string {
	if t.ProvisioningUrl == "" {
		panic(errors.New("no provisioning url set"))
	}

	if t.Secret == "" {
		panic(errors.New("secret not set"))
	}

	now := time.Now().UTC().Unix() / 30
	code := dgoogauth.ComputeCode(t.Secret, now)

	//pad leading 0s to 6 characters
	return fmt.Sprintf("%06d", code)
}
