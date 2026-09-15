package model

import (
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

func Test_ControllerManager_MarshallRoundTrip(t *testing.T) {
	req := require.New(t)

	manager := &ControllerManager{}
	original := &Controller{
		BaseEntity:        models.BaseEntity{Id: "ctrl1"},
		Name:              "ctrl1",
		CtrlAddress:       "tls:127.0.0.1:6262",
		CertPem:           "-----BEGIN CERTIFICATE-----\nleaf\n-----END CERTIFICATE-----\n",
		CaPem:             "-----BEGIN CERTIFICATE-----\nintermediate\n-----END CERTIFICATE-----\n",
		Fingerprint:       "abc",
		IsOnline:          true,
		LastJoinedAt:      time.Now().Truncate(time.Second),
		IsPreferredLeader: true,
		ApiAddresses:      map[string][]ApiAddress{"edge-client": {{Url: "https://localhost:1280", Version: "v1"}}},
	}

	bytes, err := manager.Marshall(original)
	req.NoError(err)

	decoded, err := manager.Unmarshall(bytes)
	req.NoError(err)

	req.Equal(original.CaPem, decoded.CaPem)
	req.Equal(original.CertPem, decoded.CertPem)
	req.False(original.IsChanged(decoded))
}

func Test_Controller_IsChanged_CaPem(t *testing.T) {
	req := require.New(t)

	a := &Controller{Name: "ctrl1", CaPem: "x"}
	b := &Controller{Name: "ctrl1", CaPem: "y"}

	req.True(a.IsChanged(b))
	b.CaPem = "x"
	req.False(a.IsChanged(b))
}
