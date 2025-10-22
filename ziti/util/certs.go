package util

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/openziti/ziti/ziti/cmd/common"

	"github.com/fullsailor/pkcs7"
	"github.com/pkg/errors"
)

func urlToId(url *url.URL) string {
	p := url.Port()
	if p == "" {
		if url.Scheme == "https" {
			p = "443"
		} else {
			p = "80"
		}
	}
	return url.Hostname() + "_" + p
}

func WriteCert(p common.Printer, url *url.URL, cert []byte) (string, error) {
	id := urlToId(url)
	cfgDir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	certsDir := filepath.Join(cfgDir, "certs")
	if err = os.MkdirAll(certsDir, 0700); err != nil {
		return "", errors.Wrapf(err, "unable to create ziti certs dir %v", certsDir)
	}
	certFile := filepath.Join(certsDir, id)
	if err = os.WriteFile(certFile, cert, 0600); err != nil {
		return "", err
	}
	p.Printf("Server certificate chain written to %v\n", certFile)
	return certFile, nil
}

func ReadCert(url *url.URL) ([]byte, string, error) {
	id := urlToId(url)
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
	result, err := os.ReadFile(certFile)
	if err != nil {
		return nil, "", errors.Wrapf(err, "error while reading cert for %v", id)
	}
	return result, certFile, nil
}

func IsServerTrusted(host string, client *http.Client) (bool, error) {
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(fmt.Sprintf("%v/.well-known/est/cacerts", host))
	if err != nil {
		if ue, ok := err.(*url.Error); ok && (errors.As(ue.Err, &x509.UnknownAuthorityError{}) || strings.Contains(err.Error(), "x509")) {
			return false, nil
		}
		return false, err
	}
	_ = resp.Body.Close()
	return true, nil
}

func AreCertsTrusted(host string, certs []byte, client http.Client) (bool, error) {
	c := InsecureClient(&client, certs)

	resp, err := c.Get(fmt.Sprintf("%v/.well-known/est/cacerts", host))
	if err != nil {
		if ue, ok := err.(*url.Error); ok && errors.As(ue.Err, &x509.UnknownAuthorityError{}) {
			return false, nil
		}
		return false, err
	}
	_ = resp.Body.Close()
	return true, nil
}

func GetWellKnownCerts(host string, client http.Client) ([]byte, []*x509.Certificate, error) {
	c := InsecureClient(&client, nil)

	resp, err := c.Get(fmt.Sprintf("%v/.well-known/est/cacerts", host))
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	encoded, err := io.ReadAll(resp.Body)
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

func InsecureClient(client *http.Client, certs []byte) *http.Client {
	tlsConfig := &tls.Config{
		RootCAs:            x509.NewCertPool(),
		InsecureSkipVerify: true,
	}

	if len(certs) > 0 {
		tlsConfig.RootCAs.AppendCertsFromPEM(certs)
	}

	var t *http.Transport
	if ot, ok := client.Transport.(*http.Transport); ok {
		t = ot.Clone()
		t.TLSClientConfig = tlsConfig
	} else {
		t = &http.Transport{TLSClientConfig: tlsConfig}
	}

	c := &http.Client{
		Transport: t,
	}
	return c
}
