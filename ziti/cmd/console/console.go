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

// Package console implements `ziti run console`: it serves the Ziti Admin Console (ZAC)
// as a local web app over https. The browser connects to whichever controller you configure
// inside ZAC. The command itself only serves the static console assets.
package console

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/controller/webapis"
	"github.com/spf13/cobra"
)

type ConsoleOptions struct {
	Out io.Writer
	Err io.Writer
	In  io.Reader

	BindAddress string
	Port        uint16

	Location string
	Version  string
	Yes      bool

	TlsCert string
	TlsKey  string
}

func NewConsoleCmd(out, errOut io.Writer) *cobra.Command {
	options := &ConsoleOptions{
		Out: out,
		Err: errOut,
		In:  os.Stdin,
	}

	cmd := &cobra.Command{
		Use:   "console",
		Short: "Serve the Ziti Admin Console (ZAC) locally over https",
		Long: `Runs the Ziti Admin Console (ZAC) as a local web app served over https. Point ZAC at the
controller of your choice from within the console's own UI.

The console assets are served from a local directory (--location) or downloaded for a chosen
--version (use "latest" to track the newest release).

Examples:
  # download the latest ZAC and serve it
  ziti run console --version latest

  # serve a console build you already have on disk
  ziti run console --location ./dist`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return options.Run()
		},
	}

	cmd.Flags().StringVarP(&options.BindAddress, "bind-address", "b", "127.0.0.1", "Address the console listens on")
	cmd.Flags().Uint16VarP(&options.Port, "port", "p", 8443, "Port the console listens on")
	cmd.Flags().StringVarP(&options.Location, "location", "l", "", "Directory of pre-built ZAC assets to serve; takes precedence over --version")
	cmd.Flags().StringVar(&options.Version, "version", "", `ZAC version to download and serve (e.g. "4.3.0" or "latest")`)
	cmd.Flags().BoolVarP(&options.Yes, "yes", "y", false, "Answer yes to prompts (e.g. permission to download ZAC assets)")
	cmd.Flags().StringVar(&options.TlsCert, "tls-cert", "", "PEM certificate the console serves with; a self-signed one is generated if omitted")
	cmd.Flags().StringVar(&options.TlsKey, "tls-key", "", "PEM private key for --tls-cert")

	return cmd
}

func (o *ConsoleOptions) Run() error {
	assetsDir, err := o.resolveAssets()
	if err != nil {
		return err
	}

	listenAddr := net.JoinHostPort(o.BindAddress, fmt.Sprintf("%d", o.Port))
	rawLn, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	cert, err := o.serverCertificate()
	if err != nil {
		return err
	}
	ln := tls.NewListener(rawLn, &tls.Config{Certificates: []tls.Certificate{cert}})

	// ZAC bundles ship with <base href="/zac/">, so the assets the browser requests are prefixed
	// with /zac. Serving under that context root strips the prefix back to the on-disk layout, so
	// both the bare root and /zac/ resolve correctly.
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           corsWrap(webapis.SpaHandler(assetsDir, "/zac", "index.html")),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	_, _ = fmt.Fprintf(o.Out, "serving Ziti Admin Console from %s\n", assetsDir)
	_, _ = fmt.Fprintf(o.Out, "console available at https://%s\n", o.listenOrigin())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if serveErr := server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()

	select {
	case serveErr := <-errCh:
		return serveErr
	case <-ctx.Done():
	}

	// Restore default signal handling so a second Ctrl-C force-quits if a held-open browser
	// connection makes graceful shutdown stall.
	stop()
	_, _ = fmt.Fprintln(o.Out, "\nshutting down (press Ctrl-C again to force)")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	return nil
}

func (o *ConsoleOptions) logger() *pfxlog.Builder {
	return pfxlog.Logger()
}

// corsWrap lets the served console assets answer cross-origin requests. The OIDC login bounces
// a fetch through the controller and back to the console's /auth/callback, by which point the
// request's Origin is opaque ("null"). A plain static server returns no CORS headers, so the
// browser blocks it. Reflecting the request Origin (including "null") with credentials, and
// answering preflight, lets that redirected callback complete.
func corsWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Add("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "content-type, authorization, accept, zt-session")
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// listenOrigin is the host:port a browser uses to reach the console.
func (o *ConsoleOptions) listenOrigin() string {
	host := o.BindAddress
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", o.Port))
}

// serverCertificate loads the user-supplied cert/key, or generates a short-lived self-signed
// certificate covering localhost, 127.0.0.1, and ::1 for the local listener.
func (o *ConsoleOptions) serverCertificate() (tls.Certificate, error) {
	if o.TlsCert != "" || o.TlsKey != "" {
		if o.TlsCert == "" || o.TlsKey == "" {
			return tls.Certificate{}, fmt.Errorf("--tls-cert and --tls-key must be supplied together")
		}
		cert, err := tls.LoadX509KeyPair(o.TlsCert, o.TlsKey)
		if err != nil {
			return tls.Certificate{}, fmt.Errorf("failed to load tls cert/key: %w", err)
		}
		return cert, nil
	}
	return generateSelfSignedCert()
}

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial: %w", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "ziti console"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to marshal key: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	return tls.X509KeyPair(certPEM, keyPEM)
}
