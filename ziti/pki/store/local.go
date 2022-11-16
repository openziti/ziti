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

package store

import (
	"bufio"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"github.com/openziti/ziti/ziti/pki/certificate"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"crypto/x509"
)

// Predifined directory names.
const (
	LocalCertsDir = "certs"
	LocalKeysDir  = "keys"
	LocalCrlsDir  = "crls"
)

var (
	// Index format
	// 0 full string
	// 1 Valid/Revoked/Expired
	// 2 Expiration date
	// 3 Revocation date
	// 4 Serial
	// 5 Filename
	// 6 Subject
	indexRegexp = regexp.MustCompile("^(V|R|E)\t([0-9]{12}Z)\t([0-9]{12}Z)?\t([0-9a-fA-F]{2,})\t([^\t]+)\t(.+)")
)

// Local lets us store a Certificate Authority on the local filesystem.
//
// The structure used makes it compatible with openssl.
type Local struct {
	Root string
}

// path returns private and public key path.
func (l *Local) path(caName, name string) (key string, cert string) {
	key = filepath.Join(l.Root, caName, LocalKeysDir, name+".key")
	cert = filepath.Join(l.Root, caName, LocalCertsDir, name+".cert")
	return
}

// Exists checks if a certificate or private key already exist on the local
// filesystem for a given name.
func (l *Local) Exists(caName, name string) bool {
	privPath, certPath := l.path(caName, name)
	if _, err := os.Stat(privPath); err == nil {
		return true
	}
	if _, err := os.Stat(certPath); err == nil {
		return true
	}
	return false
}

// Fetch fetchs the private key and certificate for a given name signed by caName.
func (l *Local) Fetch(caName, name string) ([]byte, []byte, error) {
	filepath.Join(l.Root, caName)

	keyPath, certPath := l.path(caName, name)
	k, err := readPEM(keyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed reading CA private key from file %v: %v", keyPath, err)
	}
	c, err := readPEM(certPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed reading CA cert from file %v: %v", certPath, err)
	}
	return k, c, nil
}

// Fetch fetchs the private key and certificate for a given name signed by caName.
func (l *Local) FetchKeyBytes(caName, name string) ([]byte, error) {
	filepath.Join(l.Root, caName)

	keyPath, _ := l.path(caName, name)
	bytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed reading %v: %v", keyPath, err)
	}
	return bytes, nil
}

func readPEM(path string) ([]byte, error) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed reading %v: %v", path, err)
	}
	p, _ := pem.Decode(bytes)
	if p == nil {
		return nil, fmt.Errorf("no PEM data found for certificate")
	}
	return p.Bytes, nil
}

// Add adds the given bundle to the local filesystem.
func (l *Local) Add(caName, name string, isCa bool, key, cert []byte) error {
	if l.Exists(caName, name) {
		return fmt.Errorf("a bundle already exists for the name %v within CA %v", name, caName)
	}
	if err := l.writeBundle(caName, name, isCa, key, cert); err != nil {
		return fmt.Errorf("failed writing bundle %v within CA %v to the local filesystem: %v", name, caName, err)
	}
	if err := l.updateIndex(caName, name, cert); err != nil {
		return fmt.Errorf("failed updating CA %v index: %v", caName, err)
	}
	return nil
}

// Chain concats an intermediate cert and a newly signed certificate bundle and adds the chained cert to the store.
func (l *Local) Chain(caName, name string) error {
	chainName := name + ".chain.pem"
	if l.Exists(caName, chainName) {
		return fmt.Errorf("a bundle already exists for the name %v within CA %v", chainName, caName)
	}
	if err := l.writeChainBundle(caName, name, chainName); err != nil {
		return fmt.Errorf("failed writing chain %v to the local filesystem: %v", chainName, err)
	}
	return nil
}

// Add adds the given csr to the local filesystem.
func (l *Local) AddCSR(caName, name string, isCa bool, key, cert []byte) error {
	if l.Exists(caName, name) {
		return fmt.Errorf("a CSR already exists for the name %v within CA %v", name, caName)
	}
	if err := l.writeBundle(caName, name, isCa, key, cert); err != nil {
		return fmt.Errorf("failed writing CSR %v within CA %v to the local filesystem: %v", name, caName, err)
	}
	return nil
}

// Add adds the given key to the local filesystem.
func (l *Local) AddKey(caName string, name string, key []byte) error {
	if l.Exists(caName, name) {
		return fmt.Errorf("a key already exists for the key name %v within CA %v", name, caName)
	}
	if err := l.writeKey(caName, name, key); err != nil {
		return fmt.Errorf("failed writing key %v within CA %v to the local filesystem: %v", name, caName, err)
	}
	return nil
}

// writeKey encodes in PEM format the bundle private key and stores it on the local filesystem.
func (l *Local) writeKey(caName string, name string, key []byte) error {
	caDir := filepath.Join(l.Root, caName)
	if _, err := os.Stat(caDir); err != nil {
		if err := InitCADir(caDir); err != nil {
			return fmt.Errorf("root directory for CA %v does not exist and cannot be created: %v", caDir, err)
		}
	}
	keyPath, _ := l.path(caName, name)
	if err := encodeAndWrite(keyPath, "RSA PRIVATE KEY", key); err != nil {
		return fmt.Errorf("failed encoding and writing private key file: %v", err)
	}
	return nil
}

// writeBundle encodes in PEM format the bundle private key and
// certificate and stores them on the local filesystem.
func (l *Local) writeBundle(caName, name string, isCa bool, key, cert []byte) error {
	caDir := filepath.Join(l.Root, caName)
	if _, err := os.Stat(caDir); err != nil {
		if err := InitCADir(caDir); err != nil {
			return fmt.Errorf("root directory for CA %v does not exist and cannot be created: %v", caDir, err)
		}
	}
	keyPath, certPath := l.path(caName, name)
	if err := encodeAndWrite(keyPath, "RSA PRIVATE KEY", key); err != nil {
		return fmt.Errorf("failed encoding and writing private key file: %v", err)
	}
	if err := encodeAndWrite(certPath, "CERTIFICATE", cert); err != nil {
		return fmt.Errorf("failed encoding and writing cert file: %v", err)
	}

	if isCa && name != caName {
		intCaDir := filepath.Join(l.Root, name)
		if err := InitCADir(intCaDir); err != nil {
			return fmt.Errorf("root directory for CA %v does not exist and cannot be created: %v", intCaDir, err)
		}
		kp, cp := l.path(name, name)
		if err := os.Link(keyPath, kp); err != nil {
			return fmt.Errorf("failed creating hard link from %v to %v: %v", keyPath, kp, err)
		}
		if err := os.Link(certPath, cp); err != nil {
			return fmt.Errorf("failed creating hard link from %v to %v: %v", certPath, cp, err)
		}
	}
	return nil
}

// writeChainBundle concats...
func (l *Local) writeChainBundle(caName, name string, chainName string) error {
	caDir := filepath.Join(l.Root, caName)
	if _, err := os.Stat(caDir); err != nil {
		if err := InitCADir(caDir); err != nil {
			return fmt.Errorf("root directory for CA %v does not exist and cannot be created: %v", caDir, err)
		}
	}

	caPath := filepath.Join(l.Root, caName, LocalCertsDir, caName+".cert")
	caIn, err := os.Open(caPath)
	if err != nil {
		return fmt.Errorf("failed to open CA: %v: %v", caPath, err)
	}
	defer caIn.Close()

	serverCertPath := filepath.Join(l.Root, caName, LocalCertsDir, name+".cert")
	serverCertIn, err := os.Open(serverCertPath)
	if err != nil {
		return fmt.Errorf("failed to open server cert: %v: %v", serverCertPath, err)
	}
	defer serverCertIn.Close()

	chainCertPath := filepath.Join(l.Root, caName, LocalCertsDir, chainName)
	out, err := os.OpenFile(chainCertPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open chain file: %v: %v", chainCertPath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, serverCertIn)
	if err != nil {
		return fmt.Errorf("failed to append server cert to output: %v", err)
	}

	_, err = io.Copy(out, caIn)
	if err != nil {
		return fmt.Errorf("failed to append ca to output: %v", err)
	}

	return nil
}

func encodeAndWrite(path, pemType string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return pem.Encode(f, &pem.Block{
		Type:  pemType,
		Bytes: data,
	})
}

// updateIndex appends a line to the index.txt with few information about the
// given the certificate.
func (l *Local) updateIndex(caName, name string, rawCert []byte) error {
	f, err := os.OpenFile(filepath.Join(l.Root, caName, "index.txt"), os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	cert, err := x509.ParseCertificate(rawCert)
	if err != nil {
		return fmt.Errorf("failed parsing raw certificate %v: %v", name, err)
	}

	sn := fmt.Sprintf("%X", cert.SerialNumber)
	// For compatibility with openssl we need an even length.
	if len(sn)%2 == 1 {
		sn = "0" + sn
	}

	// Date format: yymmddHHMMSSZ
	// E|R|V<tab>Expiry<tab>[RevocationDate]<tab>Serial<tab>filename<tab>SubjectDN
	var subject string
	if strs := cert.Subject.Country; len(strs) == 1 {
		subject += "/C=" + strs[0]
	}
	if strs := cert.Subject.Organization; len(strs) == 1 {
		subject += "/O=" + strs[0]
	}
	if strs := cert.Subject.OrganizationalUnit; len(strs) == 1 {
		subject += "/OU=" + strs[0]
	}
	if strs := cert.Subject.Locality; len(strs) == 1 {
		subject += "/L=" + strs[0]
	}
	if strs := cert.Subject.Province; len(strs) == 1 {
		subject += "/ST=" + strs[0]
	}
	subject += "/CN=" + cert.Subject.CommonName

	n, err := fmt.Fprintf(f, "V\t%vZ\t\t%v\t%v.cert\t%v\n",
		cert.NotAfter.UTC().Format("060102150405"),
		sn,
		name,
		subject)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("written 0 bytes in index file")
	}
	return nil
}

// Update updates the state of a given certificate in the index.txt.
func (l *Local) Update(caName string, sn *big.Int, st certificate.State) error {
	f, err := os.OpenFile(filepath.Join(l.Root, caName, "index.txt"), os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	var state string
	switch st {
	case certificate.Valid:
		state = "V"
	case certificate.Revoked:
		state = "R"
	case certificate.Expired:
		state = "E"
	default:
		return fmt.Errorf("unhandled certificate state: %v", st)
	}

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		matches := indexRegexp.FindStringSubmatch(scanner.Text())
		if len(matches) != 7 {
			return fmt.Errorf("line [%v] is incorrectly formated", scanner.Text())
		}

		matchedSerial := big.NewInt(0)
		fmt.Sscanf(matches[4], "%X", matchedSerial)
		if matchedSerial.Cmp(sn) == 0 {
			if matches[1] == state {
				return nil
			}

			lines = append(lines, fmt.Sprintf("%v\t%v\t%vZ\t%v\t%v\t%v",
				state,
				matches[2],
				time.Now().UTC().Format("060102150405"),
				matches[4],
				matches[5],
				matches[6]))
		} else {
			lines = append(lines, matches[0])
		}
	}

	f.Truncate(0)
	f.Seek(0, 0)

	for _, line := range lines {
		n, err := fmt.Fprintln(f, line)
		if err != nil {
			return fmt.Errorf("failed writing line [%v]: %v", line, err)
		}
		if n == 0 {
			return fmt.Errorf("failed writing line [%v]: written 0 bytes", line)
		}
	}
	return nil
}

// Revoked returns a list of revoked certificates.
func (l *Local) Revoked(caName string) ([]pkix.RevokedCertificate, error) {
	index, err := os.Open(filepath.Join(l.Root, caName, "index.txt"))
	if err != nil {
		return nil, err
	}
	defer index.Close()

	var revokedCerts []pkix.RevokedCertificate
	scanner := bufio.NewScanner(index)
	for scanner.Scan() {
		matches := indexRegexp.FindStringSubmatch(scanner.Text())
		if len(matches) != 7 {
			return nil, fmt.Errorf("line [%v] is incorrectly formated", scanner.Text())
		}
		if matches[1] != "R" {
			continue
		}

		sn := big.NewInt(0)
		fmt.Sscanf(matches[4], "%X", sn)
		t, err := time.Parse("060102150405", strings.TrimSuffix(matches[3], "Z"))
		if err != nil {
			return nil, fmt.Errorf("failed parsing revocation time %v: %v", matches[3], err)
		}
		revokedCerts = append(revokedCerts, pkix.RevokedCertificate{
			SerialNumber:   sn,
			RevocationTime: t,
		})
	}
	return revokedCerts, nil
}

// InitCADir creates the basic structure of a CA subdirectory.
//
//	|- crlnumber
//	|- index.txt
//	|- index.txt.attr
//	|- serial
//	|- certs/
//	  |- ca.cert
//	  |- name.cert
//	|- keys/
//	  |- ca.key
//	  |- name.key
func InitCADir(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed creating CA root directory %v: %v", path, err)
	}
	dirs := map[string]os.FileMode{
		filepath.Join(path, LocalCrlsDir):  0700,
		filepath.Join(path, LocalCertsDir): 0755,
		filepath.Join(path, LocalKeysDir):  0700,
	}
	for d, m := range dirs {
		if err := os.Mkdir(d, m); err != nil {
			return fmt.Errorf("failed creating directory %v: %v", d, err)
		}
	}

	files := []struct {
		Name    string
		Content string
	}{
		{Name: "serial", Content: "01"},
		{Name: "crlnumber", Content: "01"},
		{Name: "index.txt", Content: ""},
		{Name: "index.txt.attr", Content: "unique_subject = no"},
	}
	for _, f := range files {
		if err := createFile(filepath.Join(path, f.Name), f.Content); err != nil {
			return err
		}
	}
	return nil
}

func createFile(path, content string) error {
	fh, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed creating file  %v: %v", path, err)
	}
	defer fh.Close()

	if content == "" {
		return nil
	}

	n, err := fmt.Fprintln(fh, content)
	if err != nil {
		return fmt.Errorf("failed wrinting %v in %v: %v", content, path, err)
	}
	if n == 0 {
		return fmt.Errorf("failed writing %v in %v: 0 bytes written", content, path)
	}
	return nil
}
