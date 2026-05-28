package dns

import (
	"net"
	"strings"
	"sync"
)

// NewRefCountingResolver wraps resolver so the underlying hostname is only
// removed once the last caller releases it. Successive AddHostname calls for
// the same hostname bump a reference count; RemoveHostname decrements it and
// only forwards to the wrapped resolver when the count reaches zero.
func NewRefCountingResolver(resolver Resolver) Resolver {
	return &RefCountingResolver{
		names:   map[string]int{},
		wrapped: resolver,
	}
}

// RefCountingResolver reference counts AddHostname/RemoveHostname calls per
// hostname. The lock is held across the wrapped resolver calls so the count
// and the wrapped resolver's state can't diverge under concurrent use.
type RefCountingResolver struct {
	lock    sync.Mutex
	names   map[string]int
	wrapped Resolver
}

func (self *RefCountingResolver) Lookup(ip net.IP) (string, error) {
	return self.wrapped.Lookup(ip)
}

func (self *RefCountingResolver) LookupIP(hostname string) (net.IP, bool) {
	return self.wrapped.LookupIP(hostname)
}

func (self *RefCountingResolver) AddDomain(name string, cb func(string) (net.IP, error)) error {
	return self.wrapped.AddDomain(name, cb)
}

func (self *RefCountingResolver) RemoveDomain(name string) {
	self.wrapped.RemoveDomain(name)
}

func (self *RefCountingResolver) AddHostname(s string, ip net.IP) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	err := self.wrapped.AddHostname(s, ip)
	if err == nil {
		// canonicalize so different-case spellings of the same hostname share
		// one count, matching the wrapped resolver's case-insensitive view
		self.names[strings.ToLower(s)]++
	}
	return err
}

func (self *RefCountingResolver) RemoveHostname(s string) net.IP {
	self.lock.Lock()
	defer self.lock.Unlock()

	key := strings.ToLower(s)
	if count := self.names[key]; count > 1 {
		self.names[key] = count - 1
		return nil
	}

	// count <= 1 covers both the last reference and hostnames never added
	// through this layer (e.g. wildcard-domain hostnames, which are added
	// directly on the wrapped resolver but cleaned up through this one)
	delete(self.names, key)
	return self.wrapped.RemoveHostname(s)
}

func (self *RefCountingResolver) Cleanup() error {
	return self.wrapped.Cleanup()
}
