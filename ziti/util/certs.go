package util

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"github.com/openziti/ziti/ziti/cmd/common"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/fullsailor/pkcs7"
	"github.com/pkg/errors"
)

func WriteCert(p common.Printer, id string, cert []byte) (string, error) {
	cfgDir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	certsDir := filepath.Join(cfgDir, "certs")
	if err = os.MkdirAll(certsDir, 0700); err != nil {
		return "", errors.Wrapf(err, "unable to create ziti certs dir %v", certsDir)
	}
	certFile := filepath.Join(certsDir, id)
	if err = ioutil.WriteFile(certFile, cert, 0600); err != nil {
		return "", err
	}
	p.Printf("Server certificate chain written to %v\n", certFile)
	return certFile, nil
}

func ReadCert(id string) ([]byte, string, error) {
	cfgDir, err := ConfigDir()
	if err != nil {
		return nil, "", errors.Wrapf(err, "couldn't get config dir while reading cert for %v", id)
	}
	certFile := filepath.Join(cfgDir, "certs", id)
	_, err = os.Stat(certFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", errors.Wrapf(err, "error while statting cert file for %v", id)
	}
	result, err := ioutil.ReadFile(certFile)
	if err != nil {
		return nil, "", errors.Wrapf(err, "error while reading cert for %v", id)
	}
	return result, certFile, nil
}

func IsServerTrusted(host string) (bool, error) {
	resp, err := http.DefaultClient.Get(fmt.Sprintf("%v/.well-known/est/cacerts", host))
	if err != nil {
		if ue, ok := err.(*url.Error); ok && (errors.As(ue.Err, &x509.UnknownAuthorityError{}) || strings.Contains(err.Error(), "x509")) {
			return false, nil
		}
		return false, err
	}
	_ = resp.Body.Close()
	return true, nil
}

func AreCertsTrusted(host string, certs []byte) (bool, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	if tlsConfig.RootCAs == nil {
		tlsConfig.RootCAs = x509.NewCertPool()
	}

	tlsConfig.RootCAs.AppendCertsFromPEM(certs)

	transport := *http.DefaultTransport.(*http.Transport)
	transport.TLSClientConfig = tlsConfig

	client := http.Client{
		Transport: &transport,
	}

	resp, err := client.Get(fmt.Sprintf("%v/.well-known/est/cacerts", host))
	if err != nil {
		if ue, ok := err.(*url.Error); ok && errors.As(ue.Err, &x509.UnknownAuthorityError{}) {
			return false, nil
		}
		return false, err
	}
	_ = resp.Body.Close()
	return true, nil
}

func GetWellKnownCerts(host string) ([]byte, []*x509.Certificate, error) {
	transport := *http.DefaultTransport.(*http.Transport)
	transport.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}
	client := http.Client{
		Transport: &transport,
	}

	resp, err := client.Get(fmt.Sprintf("%v/.well-known/est/cacerts", host))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	encoded, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	certData, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, nil, err
	}
	certs, err := pkcs7.Parse(certData)
	if err != nil {
		return nil, nil, err
	}

	buf := &bytes.Buffer{}

	for _, cert := range certs.Certificates {
		buf.WriteString("subject=")
		buf.WriteString(cert.Subject.ToRDNSequence().String())
		buf.WriteString("\nissuer=")
		buf.WriteString(cert.Issuer.ToRDNSequence().String())
		buf.WriteString("\n")

		encoded := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})
		buf.Write(encoded)
	}

	return buf.Bytes(), certs.Certificates, nil
}

func AreCertsSame(p common.Printer, server, local []byte) bool {
	serverCerts := decodeCerts(server)
	localCerts := decodeCerts(local)

	if len(serverCerts) != len(localCerts) {
		p.Printf("Comparing remote CA to local. Server cert count: %v, local cert count: %v\n", len(serverCerts), len(localCerts))
		return false
	}

	for i := 0; i < len(serverCerts); i++ {
		serverCert := serverCerts[i]
		localCert := localCerts[i]
		if !reflect.DeepEqual(serverCert, localCert) {
			p.Printf("Cert #%v in the chain doesn't match\n", i+i)
			return false
		}
	}

	return true
}

func decodeCerts(certs []byte) []*pem.Block {
	var result []*pem.Block
	for len(certs) > 0 {
		var block *pem.Block
		block, certs = pem.Decode(certs)
		if block != nil {
			if block.Type == "CERTIFICATE" {
				result = append(result, block)
			}
		} else {
			break
		}
	}
	sort.Sort(blockSort(result))
	return result
}

type blockSort []*pem.Block

func (self blockSort) Len() int {
	return len(self)
}

func (self blockSort) Less(i, j int) bool {
	return bytes.Compare(self[i].Bytes, self[j].Bytes) < 0
}

func (self blockSort) Swap(i, j int) {
	tmp := self[i]
	self[i] = self[j]
	self[j] = tmp
}
