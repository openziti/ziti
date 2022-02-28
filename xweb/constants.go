package xweb

import (
	"crypto/tls"
	"time"
)

const (
	MinTLSVersion = tls.VersionTLS12
	MaxTLSVersion = tls.VersionTLS13

	DefaultHttpWriteTimeout = time.Second * 10
	DefaultHttpReadTimeout  = time.Second * 5
	DefaultHttpIdleTimeout  = time.Second * 5
)

// TlsVersionMap is a map of configuration strings to TLS version identifiers
var TlsVersionMap = map[string]int{
	"TLS1.0": tls.VersionTLS10,
	"TLS1.1": tls.VersionTLS11,
	"TLS1.2": tls.VersionTLS12,
	"TLS1.3": tls.VersionTLS13,
}

// ReverseTlsVersionMap is a map of TLS version identifiers to configuration strings
var ReverseTlsVersionMap = map[int]string{
	tls.VersionTLS10: "TLS1.0",
	tls.VersionTLS11: "TLS1.1",
	tls.VersionTLS12: "TLS1.2",
	tls.VersionTLS13: "TLS1.3",
}
