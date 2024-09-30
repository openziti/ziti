package dns

import (
	"github.com/michaelquigley/pfxlog"
	"net"
)

type dummy struct{}

func (d dummy) AddHostname(_ string, _ net.IP) error {
	pfxlog.Logger().Warnf("dummy resolver does not store hostname/ip mappings")
	return nil
}

func (d dummy) AddDomain(_ string, _ func(string) (net.IP, error)) error {
	pfxlog.Logger().Warnf("dummy resolver does not store hostname/ip mappings")
	return nil
}

func (d dummy) Lookup(_ net.IP) (string, error) {
	pfxlog.Logger().Warnf("dummy resolver does not store hostname/ip mappings")
	return "", nil
}

func (d dummy) LookupIP(_ string) (net.IP, bool) {
	pfxlog.Logger().Warnf("dummy resolver does not store hostname/ip mappings")
	return nil, false
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
	pfxlog.Logger().Warnf("dummy resolver does not store hostname/ip mappings")
	return &dummy{}
}
