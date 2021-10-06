package dns

import (
	cmap "github.com/orcaman/concurrent-map"
	"net"
)

func NewRefCountingResolver(resolver Resolver) Resolver {
	return &RefCountingResolver{
		names:   cmap.New(),
		wrapped: resolver,
	}
}

type RefCountingResolver struct {
	names   cmap.ConcurrentMap
	wrapped Resolver
}

func (self *RefCountingResolver) Lookup(ip net.IP) (string, error) {
	return self.wrapped.Lookup(ip)
}

func (self *RefCountingResolver) AddDomain(name string, cb func(string) (net.IP, error)) error {
	return self.wrapped.AddDomain(name, cb)
}

func (self *RefCountingResolver) AddHostname(s string, ip net.IP) error {
	err := self.wrapped.AddHostname(s, ip)
	if err != nil {
		self.names.Upsert(s, 1, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
			if exist {
				return valueInMap.(int) + 1
			}
			return 1
		})
	}
	return err
}

func (self *RefCountingResolver) RemoveHostname(s string) error {
	val := self.names.Upsert(s, 1, func(exist bool, valueInMap interface{}, newValue interface{}) interface{} {
		if exist {
			return valueInMap.(int) - 1
		}
		return 0
	})

	if result := val.(int); result == 0 {
		self.names.Remove(s)
		return self.wrapped.RemoveHostname(s)
	}
	return nil
}

func (self *RefCountingResolver) Cleanup() error {
	return self.wrapped.Cleanup()
}
