/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package cert

import (
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
)

type Fingerprints map[string]interface{}

func (fingerprints Fingerprints) Contains(fp string) bool {
	if _, matchingFingerprints := fingerprints[fp]; matchingFingerprints {
		return true
	}
	return false
}

func (fingerprints Fingerprints) HasAny(fps []string) (string, bool) {
	for _, fp := range fps {
		if _, matchingFingerprints := fingerprints[fp]; matchingFingerprints {
			return fp, true
		}
	}
	return "", false
}

func (fingerprints Fingerprints) Prints() []string {
	ret := []string{}

	for k := range fingerprints {
		ret = append(ret, k)
	}

	return ret
}

func NewFingerprintGenerator() FingerprintGenerator {
	return &defaultFingerprintGenerator{}
}

type FingerprintGenerator interface {
	FromCert(cert *x509.Certificate) string
	FromCerts(certs []*x509.Certificate) Fingerprints
	FromRaw(raw []byte) string
	FromPem(pem []byte) string
}

type defaultFingerprintGenerator struct{}

func firstCertBlock(pemBytes []byte) (*pem.Block, []byte) {
	var block *pem.Block
	for len(pemBytes) > 0 {
		block, pemBytes = pem.Decode(pemBytes)
		if block == nil {
			continue
		}
		if block.Type == "CERTIFICATE" {
			return block, pemBytes
		}
	}
	return nil, nil
}

func (fpg *defaultFingerprintGenerator) FromPem(cert []byte) string {
	block, _ := firstCertBlock(cert)
	if block == nil {
		return ""
	}

	derBytes := block.Bytes

	c, err := x509.ParseCertificate(derBytes)

	if err != nil {
		return ""
	}

	return fpg.FromCert(c)
}

func (fpg *defaultFingerprintGenerator) FromCert(cert *x509.Certificate) string {
	return fpg.FromRaw(cert.Raw)
}

func (fpg *defaultFingerprintGenerator) FromCerts(certs []*x509.Certificate) Fingerprints {
	fps := make(Fingerprints)

	for _, cert := range certs {
		fp := fpg.FromCert(cert)
		fps[fp] = true
	}
	return fps
}

func (fpg *defaultFingerprintGenerator) FromRaw(raw []byte) string {
	rawFingerprint := sha1.Sum(raw)
	return fpg.toHex(rawFingerprint[:])
}

func (fpg *defaultFingerprintGenerator) toHex(f []byte) string {
	var buf bytes.Buffer
	for i, b := range f {
		if i > 0 {
			fmt.Fprintf(&buf, ":")
		}
		fmt.Fprintf(&buf, "%02x", b)
	}
	return strings.ToUpper(buf.String())
}
