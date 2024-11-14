package pki

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrustDomain(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
		fmt.Sprintf("--trust-domain=%s", "spiffe://"+trustDomain),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+trustDomain)
}

func TestNoTrustDomain(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Empty(t, bundle.Cert.URIs)
}

func TestTrustDomainSpiffeAppended(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
		fmt.Sprintf("--trust-domain=%s", trustDomain),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+trustDomain)
}

func TestTrustDomainWithPath(t *testing.T) {
	out, errOut := streams()
	ca := NewCmdPKICreateCA(out, errOut)
	name := uuid.New().String()
	rootCaArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-file=%s", name),
		fmt.Sprintf("--ca-name=%s", name),
		fmt.Sprintf("--trust-domain=%s", "spiffe://"+trustDomain+"/path"),
	}

	ca.SetArgs(rootCaArgs)
	pkiErr := ca.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+trustDomain)
}
