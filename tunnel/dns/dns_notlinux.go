//go:build !linux

package dns

func NewDnsServer(addr string) (Resolver, error) {
	return nil, nil
}

func flushDnsCaches() {
	// not implemented
}
