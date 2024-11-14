package pki

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpiffedSetFromCa(t *testing.T) {
	out, errOut := streams()
	intermediateCmd := NewCmdPKICreateIntermediate(out, errOut)
	name := uuid.New().String()
	intermediateArgs := []string{
		fmt.Sprintf("--pki-root=%s", where),
		fmt.Sprintf("--ca-name=%s", rootCaWithSpiffeIdName),
		fmt.Sprintf("--intermediate-name=%s", name),
		fmt.Sprintf("--intermediate-file=%s", name),
		"--max-path-len=1",
	}

	intermediateCmd.SetArgs(intermediateArgs)
	pkiErr := intermediateCmd.Execute()
	if pkiErr != nil {
		logrus.Fatal(pkiErr)
	}

	bundle, e := testPki.GetCA(name)
	assert.NotNil(t, bundle)
	assert.Nil(t, e)

	assert.Contains(t, urisAsStrings(bundle.Cert.URIs), "spiffe://"+rootCaWithSpiffeIdName)
}
