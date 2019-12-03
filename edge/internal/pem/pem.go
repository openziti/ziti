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

package pem

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"strings"
)

type KeyPair struct {
	Key     interface{}
	EcKey   *ecdsa.PrivateKey
	RsaKey  *rsa.PrivateKey
	Cert    *x509.Certificate
	CertPem []byte
	KeyPem  []byte
}

func (kp *KeyPair) IsEc() bool {
	return kp.EcKey != nil
}

func (kp *KeyPair) IsRsa() bool {
	return kp.RsaKey != nil
}

func NewKeyPair(privPath, pubPath, password string) (*KeyPair, error) {
	kp := &KeyPair{}

	err := kp.loadKey(privPath, password)

	if err != nil {
		return nil, err
	}
	err = kp.loadCertificate(pubPath)

	if err != nil {
		return nil, err
	}

	return kp, nil
}

func (kp *KeyPair) loadKey(privPath, password string) error {
	pemBytes, err := ioutil.ReadFile(privPath)
	kp.KeyPem = pemBytes
	if err != nil {
		return err
	}

	block := firstKeyBlock(pemBytes)
	if block == nil {
		return fmt.Errorf("invalid or missing private key input: %s", privPath)
	}

	derBytes := block.Bytes

	if x509.IsEncryptedPEMBlock(block) {
		derBytes, err = x509.DecryptPEMBlock(block, []byte(password))
		if err != nil {
			return err
		}
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		kp.RsaKey, err = x509.ParsePKCS1PrivateKey(derBytes)
		if err != nil {
			return err
		}
		kp.Key = kp.RsaKey
	case "EC PRIVATE KEY":
		kp.EcKey, err = x509.ParseECPrivateKey(derBytes)
		if err != nil {
			return err
		}
		kp.Key = kp.EcKey
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(derBytes)
		if err != nil {
			return err
		}

		switch key := k.(type) {
		case *rsa.PrivateKey:
			kp.RsaKey = key
			kp.Key = kp.RsaKey
		case *ecdsa.PrivateKey:
			kp.EcKey = key
			kp.Key = kp.EcKey
		default:
			return fmt.Errorf("found unsupported private key type(%v) in PKCS#8 wrapping", k)
		}

	default:
		return fmt.Errorf("unsupported private key: %s", privPath)
	}

	return nil
}

func (kp *KeyPair) loadCertificate(pubPath string) error {
	pemBytes, err := ioutil.ReadFile(pubPath)
	kp.CertPem = pemBytes

	if err != nil {
		return fmt.Errorf("problem loading certificate: %s", err)
	}

	block := firstCertBlock(pemBytes)
	if block == nil {
		return fmt.Errorf("problem loading certificate. invalid format or missing public key input: %s", pubPath)
	}
	derBytes := block.Bytes
	kp.Cert, err = x509.ParseCertificate(derBytes)

	if err != nil {
		return fmt.Errorf("problem converting certificate to x509. %s", err)
	}

	return nil
}

func firstKeyBlock(pemBytes []byte) *pem.Block {
	var block *pem.Block
	for len(pemBytes) > 0 {
		block, pemBytes = pem.Decode(pemBytes)
		if strings.HasSuffix(block.Type, "PRIVATE KEY") {
			return block
		}
	}
	return nil
}

func firstCertBlock(pemBytes []byte) *pem.Block {
	var block *pem.Block
	for len(pemBytes) > 0 {
		block, pemBytes = pem.Decode(pemBytes)
		if block != nil {
			if block.Type == "CERTIFICATE" {
				return block
			}
		} else {
			return nil
		}
	}
	return nil
}
