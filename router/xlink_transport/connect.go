package xlink_transport

import (
	"crypto/x509"
	"github.com/openziti/channel"
	"github.com/openziti/foundation/identity/identity"
	"github.com/openziti/foundation/util/errorz"
	"github.com/pkg/errors"
)

type ConnectionHandler struct {
	routerId identity.Identity
}

func (self *ConnectionHandler) HandleConnection(_ *channel.Hello, certificates []*x509.Certificate) error {
	if len(certificates) == 0 {
		return errors.New("no certificates provided, unable to verify dialer")
	}

	config := self.routerId.ServerTLSConfig()

	opts := x509.VerifyOptions{
		Roots:         config.RootCAs,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	var errorList errorz.MultipleErrors

	for _, cert := range certificates {
		if _, err := cert.Verify(opts); err == nil {
			return nil
		} else {
			errorList = append(errorList, err)
		}
	}

	//goland:noinspection GoNilness
	return errorList.ToError()
}
