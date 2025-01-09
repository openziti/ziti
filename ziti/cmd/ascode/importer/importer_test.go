package importer

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_InputArgs(t *testing.T) {

	importer := Importer{}

	t.Run("service import", func(t *testing.T) {

		assert.True(t, importer.IsCertificateAuthorityImportRequired([]string{"certificate-authority"}), "should be imported")
		assert.True(t, importer.IsCertificateAuthorityImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsCertificateAuthorityImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsCertificateAuthorityImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("service import", func(t *testing.T) {

		assert.True(t, importer.IsServiceImportRequired([]string{"service"}), "should be imported")
		assert.True(t, importer.IsServiceImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsServiceImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsServiceImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsServiceImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("config import", func(t *testing.T) {

		assert.True(t, importer.IsConfigImportRequired([]string{"service"}), "should be imported")
		assert.True(t, importer.IsConfigImportRequired([]string{"config"}), "should be imported")
		assert.True(t, importer.IsConfigImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsConfigImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsConfigImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"certificate-authority"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsConfigImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("config-type import", func(t *testing.T) {

		assert.True(t, importer.IsConfigTypeImportRequired([]string{"service"}), "should be imported")
		assert.True(t, importer.IsConfigTypeImportRequired([]string{"config"}), "should be imported")
		assert.True(t, importer.IsConfigTypeImportRequired([]string{"config-type"}), "should be imported")
		assert.True(t, importer.IsConfigTypeImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsConfigTypeImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsConfigTypeImportRequired([]string{"certificate-authority"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsConfigTypeImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("identity import", func(t *testing.T) {

		assert.True(t, importer.IsIdentityImportRequired([]string{"identity"}), "should be imported")
		assert.True(t, importer.IsIdentityImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsIdentityImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsIdentityImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"external-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"certificate-authority"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsIdentityImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("auth-policy import", func(t *testing.T) {

		assert.True(t, importer.IsAuthPolicyImportRequired([]string{"auth-policy"}), "should be imported")
		assert.True(t, importer.IsAuthPolicyImportRequired([]string{"identity"}), "should be imported")
		assert.True(t, importer.IsAuthPolicyImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsAuthPolicyImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"external-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"certificate-authority"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsAuthPolicyImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("ext-jwt-signer import", func(t *testing.T) {

		assert.True(t, importer.IsExtJwtSignerImportRequired([]string{"ext-jwt-signer"}), "should be imported")
		assert.True(t, importer.IsExtJwtSignerImportRequired([]string{"external-jwt-signer"}), "should be imported")
		assert.True(t, importer.IsExtJwtSignerImportRequired([]string{"auth-policy"}), "should be imported")
		assert.True(t, importer.IsExtJwtSignerImportRequired([]string{"identity"}), "should be imported")
		assert.True(t, importer.IsExtJwtSignerImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsExtJwtSignerImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"certificate-authority"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsExtJwtSignerImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

	t.Run("posture-check import", func(t *testing.T) {

		assert.True(t, importer.IsPostureCheckImportRequired([]string{"posture-check"}), "should be imported")
		assert.True(t, importer.IsPostureCheckImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsPostureCheckImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsPostureCheckImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"external-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"service-edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsPostureCheckImportRequired([]string{"edge-router-policy"}), "should not be imported")

	})

	t.Run("service-policy import", func(t *testing.T) {

		assert.True(t, importer.IsServicePolicyImportRequired([]string{"service-policy"}), "should be imported")
		assert.True(t, importer.IsServicePolicyImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsServicePolicyImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsServicePolicyImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"external-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"service-edge-router-policy"}), "should not be imported")
		assert.False(t, importer.IsServicePolicyImportRequired([]string{"edge-router-policy"}), "should not be imported")

	})

	t.Run("service-edge-router-policy import", func(t *testing.T) {

		assert.True(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"service-edge-router-policy"}), "should be imported")
		assert.True(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"external-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsServiceEdgeRouterPolicyImportRequired([]string{"edge-router-policy"}), "should not be imported")

	})

	t.Run("service-edge-router-policy import", func(t *testing.T) {

		assert.True(t, importer.IsEdgeRouterPolicyImportRequired([]string{"edge-router-policy"}), "should be imported")
		assert.True(t, importer.IsEdgeRouterPolicyImportRequired([]string{"all"}), "should be imported")
		assert.True(t, importer.IsEdgeRouterPolicyImportRequired([]string{}), "should be imported")

		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"service"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"config"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"config-type"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"identity"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"auth-policy"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"ext-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"external-jwt-signer"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"posture-check"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"service-policy"}), "should not be imported")
		assert.False(t, importer.IsEdgeRouterPolicyImportRequired([]string{"service-edge-router-policy"}), "should not be imported")

	})

}
