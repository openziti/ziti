package dns

import "net"

type dummy struct{}

func (d dummy) AddHostname(_ string, _ net.IP) error {
	return nil
}

func (d dummy) AddDomain(_ string, _ func(string) (net.IP, error)) error {
	return nil
}

func (d dummy) Lookup(_ net.IP) (string, error) {
	return "", nil
}

func (d dummy) RemoveHostname(_ string) net.IP {
	return nil
}

func (d dummy) RemoveDomain(_ string) {
}

func (d dummy) Cleanup() error {
	return nil
}

func NewDummyResolver() Resolver {
	return &dummy{}
}
