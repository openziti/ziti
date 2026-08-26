package sync_strats

import (
	"crypto/sha1"
	"fmt"
	"testing"

	"github.com/openziti/ziti/v2/common/pb/edge_ctrl_pb"
	"github.com/stretchr/testify/require"
)

func Test_newPublicKey(t *testing.T) {
	data := []byte("anchor-cert-der")
	intermediate1 := []byte("intermediate-1-der")
	intermediate2 := []byte("intermediate-2-der")

	t.Run("sets kid from data fingerprint and carries usages and intermediates", func(t *testing.T) {
		req := require.New(t)

		publicKey := newPublicKey(data, edge_ctrl_pb.DataState_PublicKey_X509CertDer, firstPartyCaUsages, intermediate1, intermediate2)

		req.Equal(data, publicKey.Data)
		req.Equal(fmt.Sprintf("%x", sha1.Sum(data)), publicKey.Kid)
		req.Equal(firstPartyCaUsages, publicKey.Usages)
		req.Equal(edge_ctrl_pb.DataState_PublicKey_X509CertDer, publicKey.Format)
		req.Equal([][]byte{intermediate1, intermediate2}, publicKey.Intermediates)
	})

	t.Run("no intermediates yields empty intermediates", func(t *testing.T) {
		req := require.New(t)

		publicKey := newPublicKey(data, edge_ctrl_pb.DataState_PublicKey_X509CertDer, thirdPartyCaUsages)

		req.Empty(publicKey.Intermediates)
	})

	t.Run("usage sets pair the deprecated usage with the party-specific usage", func(t *testing.T) {
		req := require.New(t)

		req.Equal([]edge_ctrl_pb.DataState_PublicKey_Usage{edge_ctrl_pb.DataState_PublicKey_JWTValidation}, controllerCertUsages)

		req.Contains(firstPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation)
		req.Contains(firstPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation)
		req.NotContains(firstPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation)

		req.Contains(thirdPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ClientX509CertValidation)
		req.Contains(thirdPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_ThirdPartyX509CertValidation)
		req.NotContains(thirdPartyCaUsages, edge_ctrl_pb.DataState_PublicKey_FirstPartyX509CertValidation)
	})
}
