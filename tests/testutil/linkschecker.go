package testutil

import (
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/common/handler_common"
	"github.com/openziti/fabric/common/pb/ctrl_pb"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"sync"
	"time"
)

type TestLink struct {
	Id     string
	Dest   string
	Failed bool
}

type LinkStateChecker struct {
	errorC chan error
	links  map[string]*TestLink
	req    *require.Assertions
	sync.Mutex
}

func (self *LinkStateChecker) reportError(err error) {
	select {
	case self.errorC <- err:
	default:
	}
}

func (self *LinkStateChecker) HandleLink(msg *channel.Message, _ channel.Channel) {
	self.Lock()
	defer self.Unlock()

	routerLinks := &ctrl_pb.RouterLinks{}
	if err := proto.Unmarshal(msg.Body, routerLinks); err != nil {
		self.reportError(err)
	}

	for _, link := range routerLinks.Links {
		self.links[link.Id] = &TestLink{
			Id: link.Id,
		}
	}
}

func (self *LinkStateChecker) HandleFault(msg *channel.Message, _ channel.Channel) {
	self.Lock()
	defer self.Unlock()

	fault := &ctrl_pb.Fault{}
	if err := proto.Unmarshal(msg.Body, fault); err != nil {
		select {
		case self.errorC <- err:
		default:
		}
	}

	if fault.Subject == ctrl_pb.FaultSubject_LinkFault || fault.Subject == ctrl_pb.FaultSubject_LinkDuplicate {
		if link, found := self.links[fault.Id]; found {
			link.Failed = true
		} else {
			self.reportError(errors.Errorf("no link with Id %s found", fault.Id))
		}
	}
}

func (self *LinkStateChecker) HandleOther(msg *channel.Message, _ channel.Channel) {
	//  -33 = reconenct ping
	//    5 = heartbeat
	// 1007 = metrics message
	if msg.ContentType == -33 || msg.ContentType == 5 || msg.ContentType == 1007 {
		logrus.Debug("ignoring heartbeats, reconnect pings and metrics")
		return
	}

	self.reportError(errors.Errorf("unhandled msg of type %v received", msg.ContentType))
}

func (self *LinkStateChecker) RequireNoErrors() {
	var errList errorz.MultipleErrors

	done := false
	for !done {
		select {
		case err := <-self.errorC:
			errList = append(errList, err)
		default:
			done = true
		}
	}

	if len(errList) > 0 {
		self.req.NoError(errList)
	}
}

func (self *LinkStateChecker) RequireOneActiveLink() *TestLink {
	self.Lock()
	defer self.Unlock()

	var activeLink *TestLink

	for _, link := range self.links {
		if !link.Failed {
			self.req.Nil(activeLink, "more than one active link found")
			activeLink = link
		}
	}
	self.req.NotNil(activeLink, "no active link found")
	return activeLink
}

func StartLinkTest(id string, uf channel.UnderlayFactory, assertions *require.Assertions) (channel.Channel, *LinkStateChecker) {
	checker := &LinkStateChecker{
		errorC: make(chan error, 4),
		links:  map[string]*TestLink{},
		req:    assertions,
	}

	bindHandler := func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(channel.AnyContentType, checker.HandleOther)
		binding.AddReceiveHandlerF(int32(ctrl_pb.ContentType_VerifyRouterType), func(msg *channel.Message, ch channel.Channel) {
			handler_common.SendSuccess(msg, ch, "link success")
		})
		binding.AddReceiveHandlerF(int32(ctrl_pb.ContentType_RouterLinksType), checker.HandleLink)
		binding.AddReceiveHandlerF(int32(ctrl_pb.ContentType_FaultType), checker.HandleFault)
		return nil
	}

	timeoutUF := NewTimeoutUnderlayFactory(uf, 2*time.Second)
	ch, err := channel.NewChannel(id, timeoutUF, channel.BindHandlerF(bindHandler), channel.DefaultOptions())
	assertions.NoError(err)
	return ch, checker
}
