package xgress

import (
	"io"
	"sync"
)

type CloseHelper struct {
	closer    io.Closer
	closeLock sync.Mutex
}

func (self *CloseHelper) Init(closer io.Closer) {
	if self == nil {
		return
	}

	self.closeLock.Lock()
	defer self.closeLock.Unlock()
	self.closer = closer
}

func (self *CloseHelper) Close() error {
	if self == nil {
		return nil
	}

	self.closeLock.Lock()
	defer self.closeLock.Unlock()

	if self.closer != nil {
		result := self.closer.Close()
		self.closer = nil
		return result
	}
	return nil
}
