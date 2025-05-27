/*
	Copyright NetFoundry Inc.

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
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/sha3"
)

type Fingerprints map[string]*x509.Certificate

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
	var ret []string

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
		pemLength := len(pemBytes)
		block, pemBytes = pem.Decode(pemBytes)

		if pemLength == len(pemBytes) {
			//pem isn't parsing, we received not blocks, pemBytes should shrink on each Decode()
			return nil, nil
		}

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
		fps[fp] = cert
	}
	return fps
}

func (fpg *defaultFingerprintGenerator) FromRaw(raw []byte) string {
	return fmt.Sprintf("%x", Shake256HexN(raw, 20))
}

// Shake256HexN returns a SHAKE256 hash of "length" bytes as a hex string (2*length = count of hex characters).
func Shake256HexN(data []byte, length int) string {
	hash := make([]byte, length)
	hasher := sha3.NewShake256()
	hasher.Write(data)
	hasher.Read(hash)
	return hex.EncodeToString(hash)
}
