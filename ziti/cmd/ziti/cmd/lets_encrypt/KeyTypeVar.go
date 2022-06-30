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

package lets_encrypt

import (
	"errors"
	"github.com/go-acme/lego/v4/certcrypto"
)

type KeyTypeVar string

func (f *KeyTypeVar) String() string {
	switch f.Get() {
	case certcrypto.EC256:
		return "EC256"
	case certcrypto.EC384:
		return "EC384"
	case certcrypto.RSA2048:
		return "RSA2048"
	case certcrypto.RSA4096:
		return "RSA4096"
	case certcrypto.RSA8192:
		return "RSA8192"
	default:
		return "?"
	}
}

func (f *KeyTypeVar) Set(value string) error {
	switch value {
	case "EC256":
		*f = KeyTypeVar(certcrypto.EC256)
	case "EC384":
		*f = KeyTypeVar(certcrypto.EC384)
	case "RSA2048":
		*f = KeyTypeVar(certcrypto.RSA2048)
	case "RSA4096":
		*f = KeyTypeVar(certcrypto.RSA4096)
	case "RSA8192":
		*f = KeyTypeVar(certcrypto.RSA8192)
	default:
		return errors.New("Invalid option")
	}

	return nil
}

func (f *KeyTypeVar) EC256() bool {
	if certcrypto.KeyType(*f) == certcrypto.EC256 {
		return true
	}
	return false
}

func (f *KeyTypeVar) EC384() bool {
	if certcrypto.KeyType(*f) == certcrypto.EC384 {
		return true
	}
	return false
}

func (f *KeyTypeVar) RSA2048() bool {
	if certcrypto.KeyType(*f) == certcrypto.RSA2048 {
		return true
	}
	return false
}

func (f *KeyTypeVar) RSA4096() bool {
	if certcrypto.KeyType(*f) == certcrypto.RSA4096 {
		return true
	}
	return false
}

func (f *KeyTypeVar) RSA8192() bool {
	if certcrypto.KeyType(*f) == certcrypto.RSA8192 {
		return true
	}
	return false
}

func (f *KeyTypeVar) Get() certcrypto.KeyType {
	return certcrypto.KeyType(*f)
}

func (f *KeyTypeVar) Type() string {
	return "EC256|EC384|RSA2048|RSA4096|RSA8192"
}
