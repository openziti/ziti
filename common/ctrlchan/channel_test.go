package ctrlchan

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/transport/v2/tcp"
	"github.com/stretchr/testify/require"
)

const echoContentType int32 = 1000

func init() {
	// Register address parsers needed for tests
	transport.AddAddressParser(tcp.AddressParser{})
}

// Test that router-side channel stays open when underlays drop to 0 and actively re-dials
func TestDialCtrlChannel_StaysOpenAt0UnderlaysAndRedials(t *testing.T) {
	req := require.New(t)

	// Setup listener on controller side
	// Use a high port number for testing
	listenerAddr := "tcp:127.0.0.1:40001"
	id := &identity.TokenId{Token: "test-controller"}

	var acceptedChannels []channel.MultiChannel
	var acceptedLock sync.Mutex
	acceptCount := atomic.Int32{}

	// Create multi-listener for controller side
	multiListener := channel.NewMultiListener(
		func(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
			// This handles grouped underlays - first connection with IsFirstGroupConnection=true
			acceptCount.Add(1)

			listenerChannel := NewListenerCtrlChannel()
			multiConfig := &channel.MultiChannelConfig{
				LogicalName:     "ctrl/" + underlay.ConnectionId(),
				Options:         channel.DefaultOptions(),
				UnderlayHandler: listenerChannel,
				Underlay:        underlay,
				BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
					binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
						closeCallback()
					}))
					return nil
				}),
			}

			multiCh, err := channel.NewMultiChannel(multiConfig)
			if err != nil {
				return nil, err
			}

			acceptedLock.Lock()
			acceptedChannels = append(acceptedChannels, multiCh)
			acceptedLock.Unlock()

			return multiCh, nil
		},
		func(underlay channel.Underlay) error {
			// Fallback for ungrouped connections - reject them
			return fmt.Errorf("ungrouped connections not supported")
		},
	)

	listenerConfig := channel.ListenerConfig{
		ConnectOptions: channel.DefaultOptions().ConnectOptions,
	}

	bindAddr, err := transport.ParseAddress(listenerAddr)
	req.NoError(err)

	listener, err := channel.NewClassicListenerF(id, bindAddr, listenerConfig, multiListener.AcceptUnderlay)
	req.NoError(err)
	req.NotNil(listener)

	actualAddr := bindAddr
	t.Logf("Listener started on %s", actualAddr)

	defer func() { _ = listener.Close() }()

	// Setup dialer on router side
	headers := channel.Headers{}
	headers.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	dialConfig := channel.DialerConfig{
		Identity: id,
		Endpoint: actualAddr,
	}
	dialer := channel.NewClassicDialer(dialConfig)

	// Create initial underlay with headers
	initialUnderlay, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)
	req.NotNil(initialUnderlay)

	// Track underlay changes
	var changeCount atomic.Int32
	changeCallback := func(ch *DialCtrlChannel, oldCount, newCount uint32) {
		changeCount.Add(1)
	}

	// Create dial control channel
	dialChannel := NewDialCtrlChannel(DialCtrlChannelConfig{
		Dialer:                  dialer,
		MaxDefaultChannels:      1,
		MaxHighPriorityChannels: 0,
		MaxLowPriorityChannels:  0,
		UnderlayChangeCallback:  changeCallback,
	})

	// Create and initialize multi-channel on router side
	multiConfig := &channel.MultiChannelConfig{
		LogicalName:     "ctrl/router-test",
		Options:         channel.DefaultOptions(),
		UnderlayHandler: dialChannel,
		Underlay:        initialUnderlay,
	}

	multiCh, err := channel.NewMultiChannel(multiConfig)
	req.NoError(err)
	req.NotNil(multiCh)

	// Wait for connection to be established
	req.Eventually(func() bool {
		acceptedLock.Lock()
		defer acceptedLock.Unlock()
		return len(acceptedChannels) == 1
	}, 5*time.Second, 10*time.Millisecond, "Controller should have accepted one connection")

	// Verify both sides are connected
	req.False(multiCh.IsClosed(), "Router channel should be open")

	acceptedLock.Lock()
	controllerCh := acceptedChannels[0]
	acceptedLock.Unlock()

	req.False(controllerCh.IsClosed(), "Controller channel should be open")

	// Close the initial underlay to simulate connection failure
	t.Log("Closing initial underlay to simulate connection failure")
	err = initialUnderlay.Close()
	req.NoError(err)

	// Verify controller channel closed (expected behavior)
	req.Eventually(func() bool {
		return controllerCh.IsClosed()
	}, 5*time.Second, 10*time.Millisecond, "Controller channel should close when underlay count hits 0")

	// Verify router channel stays open (key behavior)
	req.False(multiCh.IsClosed(), "Router channel should remain open even at 0 underlays")

	// Wait for router to attempt re-dial
	req.Eventually(func() bool {
		acceptedLock.Lock()
		defer acceptedLock.Unlock()
		return len(acceptedChannels) >= 2
	}, 10*time.Second, 10*time.Millisecond, "Router should have re-dialed and controller accepted new connection")

	// Verify router channel is still open
	req.False(multiCh.IsClosed(), "Router channel should still be open after re-dial")

	// Clean up
	_ = multiCh.Close()
}

// Test that controller-side channel closes when underlays drop to 0
func TestListenerCtrlChannel_ClosesAt0Underlays(t *testing.T) {
	req := require.New(t)

	// Setup listener on controller side
	// Use a high port number for testing
	listenerAddr := "tcp:127.0.0.1:40002"
	id := &identity.TokenId{Token: "test-controller"}

	var controllerChannel channel.MultiChannel
	var channelLock sync.Mutex

	multiListener := channel.NewMultiListener(
		func(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
			listenerChannel := NewListenerCtrlChannel()
			multiConfig := &channel.MultiChannelConfig{
				LogicalName:     "ctrl/" + underlay.ConnectionId(),
				Options:         channel.DefaultOptions(),
				UnderlayHandler: listenerChannel,
				Underlay:        underlay,
				BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
					binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
						closeCallback()
					}))
					return nil
				}),
			}

			multiCh, err := channel.NewMultiChannel(multiConfig)
			if err != nil {
				return nil, err
			}

			channelLock.Lock()
			controllerChannel = multiCh
			channelLock.Unlock()

			return multiCh, nil
		},
		func(underlay channel.Underlay) error {
			return fmt.Errorf("ungrouped connections not supported")
		},
	)

	listenerConfig := channel.ListenerConfig{
		ConnectOptions: channel.DefaultOptions().ConnectOptions,
	}

	bindAddr, err := transport.ParseAddress(listenerAddr)
	req.NoError(err)

	listener, err := channel.NewClassicListenerF(id, bindAddr, listenerConfig, multiListener.AcceptUnderlay)
	req.NoError(err)
	actualAddr := bindAddr
	defer func() { _ = listener.Close() }()

	// Create a simple dialer
	headers := channel.Headers{}
	headers.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	dialConfig := channel.DialerConfig{
		Identity: id,
		Endpoint: actualAddr,
	}
	dialer := channel.NewClassicDialer(dialConfig)

	// Dial to establish connection
	underlay, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)

	// Wait for connection to be accepted
	var ctrlCh channel.MultiChannel
	req.Eventually(func() bool {
		channelLock.Lock()
		ctrlCh = controllerChannel
		channelLock.Unlock()
		return ctrlCh != nil
	}, 5*time.Second, 10*time.Millisecond, "Controller should have accepted connection")

	req.False(ctrlCh.IsClosed(), "Controller channel should be open")

	// Close the underlay
	t.Log("Closing underlay")
	err = underlay.Close()
	req.NoError(err)

	// Verify controller channel is now closed
	req.Eventually(func() bool {
		return ctrlCh.IsClosed()
	}, 5*time.Second, 10*time.Millisecond, "Controller channel should close when underlay count hits 0")
}

// Test all three priority levels can establish connections and handle failures
func TestDialCtrlChannel_AllPriorityLevels(t *testing.T) {
	req := require.New(t)

	// Setup listener on controller side
	// Use a high port number for testing
	listenerAddr := "tcp:127.0.0.1:40003"
	id := &identity.TokenId{Token: "test-controller"}

	var acceptedChannel channel.MultiChannel
	var acceptedChannelLock sync.Mutex

	multiListener := channel.NewMultiListener(
		func(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
			// Check if this is for an existing channel
			acceptedChannelLock.Lock()
			if acceptedChannel != nil && !acceptedChannel.IsClosed() {
				acceptedChannelLock.Unlock()
				// Return the existing channel to add this underlay to it
				return acceptedChannel, nil
			}
			acceptedChannelLock.Unlock()

			// Create new channel for first connection
			listenerChannel := NewListenerCtrlChannel()

			multiConfig := &channel.MultiChannelConfig{
				LogicalName:     "ctrl/" + underlay.ConnectionId(),
				Options:         channel.DefaultOptions(),
				UnderlayHandler: listenerChannel,
				Underlay:        underlay,
				BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
					binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
						closeCallback()
					}))
					return nil
				}),
			}

			multiCh, err := channel.NewMultiChannel(multiConfig)
			if err != nil {
				return nil, err
			}

			acceptedChannelLock.Lock()
			acceptedChannel = multiCh
			acceptedChannelLock.Unlock()

			return multiCh, nil
		},
		func(underlay channel.Underlay) error {
			return fmt.Errorf("ungrouped connections not supported")
		},
	)

	listenerConfig := channel.ListenerConfig{
		ConnectOptions: channel.DefaultOptions().ConnectOptions,
	}

	bindAddr, err := transport.ParseAddress(listenerAddr)
	req.NoError(err)

	listener, err := channel.NewClassicListenerF(id, bindAddr, listenerConfig, multiListener.AcceptUnderlay)
	req.NoError(err)
	actualAddr := bindAddr
	defer func() { _ = listener.Close() }()

	t.Logf("Listener started on %s", actualAddr)

	// Setup dialer on router side
	headers := channel.Headers{}
	headers.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	dialConfig := channel.DialerConfig{
		Identity: id,
		Endpoint: actualAddr,
	}
	dialer := channel.NewClassicDialer(dialConfig)

	// Create initial underlay
	initialUnderlay, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)

	changeCallback := func(ch *DialCtrlChannel, oldCount, newCount uint32) {}

	// Create dial control channel with all three priority levels
	dialChannel := NewDialCtrlChannel(DialCtrlChannelConfig{
		Dialer:                  dialer,
		MaxDefaultChannels:      2,
		MaxHighPriorityChannels: 2,
		MaxLowPriorityChannels:  2,
		UnderlayChangeCallback:  changeCallback,
	})

	// Create and initialize multi-channel
	multiConfig := &channel.MultiChannelConfig{
		LogicalName:     "ctrl/router-priorities",
		Options:         channel.DefaultOptions(),
		UnderlayHandler: dialChannel,
		Underlay:        initialUnderlay,
	}

	multiCh, err := channel.NewMultiChannel(multiConfig)
	req.NoError(err)

	// Wait for all underlays to be established
	var serverCh channel.MultiChannel
	req.Eventually(func() bool {
		acceptedChannelLock.Lock()
		serverCh = acceptedChannel
		acceptedChannelLock.Unlock()
		if serverCh == nil {
			return false
		}
		counts := serverCh.GetUnderlayCountsByType()
		return counts[ChannelTypeDefault] > 0 &&
			counts[ChannelTypeHighPriority] > 0 &&
			counts[ChannelTypeLowPriority] > 0
	}, 10*time.Second, 10*time.Millisecond, "All three priority levels should establish connections")

	// Get underlay counts from both sides
	clientCounts := multiCh.GetUnderlayCountsByType()
	serverCounts := serverCh.GetUnderlayCountsByType()

	t.Logf("Client underlay counts: %v", clientCounts)
	t.Logf("Server underlay counts: %v", serverCounts)

	// Verify channel is open
	req.False(multiCh.IsClosed(), "Channel should be open")
	req.False(serverCh.IsClosed(), "Server channel should be open")

	// Clean up
	_ = multiCh.Close()
	_ = serverCh.Close()
}

// Test that when router reconnects after channel closure, controller accepts it as a new connection
func TestListenerCtrlChannel_AcceptsReconnectionAsNew(t *testing.T) {
	req := require.New(t)

	// Setup listener on controller side
	// Use a high port number for testing
	listenerAddr := "tcp:127.0.0.1:40004"
	id := &identity.TokenId{Token: "test-controller"}

	var acceptedChannels []channel.MultiChannel
	var acceptedLock sync.Mutex
	acceptCount := atomic.Int32{}

	multiListener := channel.NewMultiListener(
		func(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
			acceptCount.Add(1)

			listenerChannel := NewListenerCtrlChannel()
			multiConfig := &channel.MultiChannelConfig{
				LogicalName:     "ctrl/" + underlay.ConnectionId(),
				Options:         channel.DefaultOptions(),
				UnderlayHandler: listenerChannel,
				Underlay:        underlay,
				BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
					binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
						closeCallback()
					}))
					return nil
				}),
			}

			multiCh, err := channel.NewMultiChannel(multiConfig)
			if err != nil {
				return nil, err
			}

			acceptedLock.Lock()
			acceptedChannels = append(acceptedChannels, multiCh)
			acceptedLock.Unlock()

			return multiCh, nil
		},
		func(underlay channel.Underlay) error {
			return fmt.Errorf("ungrouped connections not supported")
		},
	)

	listenerConfig := channel.ListenerConfig{
		ConnectOptions: channel.DefaultOptions().ConnectOptions,
	}

	bindAddr, err := transport.ParseAddress(listenerAddr)
	req.NoError(err)

	listener, err := channel.NewClassicListenerF(id, bindAddr, listenerConfig, multiListener.AcceptUnderlay)
	req.NoError(err)
	actualAddr := bindAddr
	defer func() { _ = listener.Close() }()

	// Create first connection
	headers := channel.Headers{}
	headers.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	dialConfig := channel.DialerConfig{
		Identity: id,
		Endpoint: actualAddr,
	}
	dialer := channel.NewClassicDialer(dialConfig)

	underlay1, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)

	// Wait for connection
	req.Eventually(func() bool {
		acceptedLock.Lock()
		defer acceptedLock.Unlock()
		return len(acceptedChannels) == 1
	}, 5*time.Second, 10*time.Millisecond, "Should have accepted first connection")

	acceptedLock.Lock()
	firstCh := acceptedChannels[0]
	acceptedLock.Unlock()

	req.False(firstCh.IsClosed(), "First channel should be open")

	// Close first connection
	t.Log("Closing first connection")
	err = underlay1.Close()
	req.NoError(err)

	// Wait for channel to close
	req.Eventually(func() bool {
		return firstCh.IsClosed()
	}, 5*time.Second, 10*time.Millisecond, "First channel should be closed")

	// Create second connection (simulating router reconnection)
	t.Log("Creating second connection")
	underlay2, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)
	defer func() { _ = underlay2.Close() }()

	// Wait for new connection to be accepted
	req.Eventually(func() bool {
		acceptedLock.Lock()
		defer acceptedLock.Unlock()
		return len(acceptedChannels) == 2
	}, 5*time.Second, 10*time.Millisecond, "Should have accepted second connection as new")

	acceptedLock.Lock()
	req.Len(acceptedChannels, 2, "Should have two separate channels")
	secondCh := acceptedChannels[1]
	acceptedLock.Unlock()

	req.False(secondCh.IsClosed(), "Second channel should be open")
	req.NotEqual(firstCh, secondCh, "Should be different channel instances")
}

// Test that a channel with all priority levels can survive repeated full disconnection/reconnection
// cycles while maintaining message delivery. Repeats 10 times.
func TestDialCtrlChannel_ReconnectCycle(t *testing.T) {
	req := require.New(t)

	listenerAddr := "tcp:127.0.0.1:40005"
	id := &identity.TokenId{Token: "test-controller"}

	// Controller side: accept connections and echo messages back
	multiListener := channel.NewMultiListener(
		func(underlay channel.Underlay, closeCallback func()) (channel.MultiChannel, error) {
			listenerChannel := NewListenerCtrlChannel()
			multiConfig := &channel.MultiChannelConfig{
				LogicalName:     "ctrl/" + underlay.ConnectionId(),
				Options:         channel.DefaultOptions(),
				UnderlayHandler: listenerChannel,
				Underlay:        underlay,
				BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
					binding.AddReceiveHandlerF(echoContentType, func(m *channel.Message, ch channel.Channel) {
						reply := channel.NewMessage(echoContentType, m.Body)
						reply.ReplyTo(m)
						_ = ch.Send(reply)
					})
					binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
						closeCallback()
					}))
					return nil
				}),
			}
			return channel.NewMultiChannel(multiConfig)
		},
		func(underlay channel.Underlay) error {
			return fmt.Errorf("ungrouped connections not supported")
		},
	)

	listenerConfig := channel.ListenerConfig{
		ConnectOptions: channel.DefaultOptions().ConnectOptions,
	}

	bindAddr, err := transport.ParseAddress(listenerAddr)
	req.NoError(err)

	listener, err := channel.NewClassicListenerF(id, bindAddr, listenerConfig, multiListener.AcceptUnderlay)
	req.NoError(err)
	defer func() { _ = listener.Close() }()

	// Router side: dial with all three priority levels
	dialConfig := channel.DialerConfig{
		Identity: id,
		Endpoint: bindAddr,
	}
	dialer := channel.NewClassicDialer(dialConfig)

	headers := channel.Headers{}
	headers.PutStringHeader(channel.TypeHeader, ChannelTypeDefault)
	headers.PutBoolHeader(channel.IsGroupedHeader, true)
	headers.PutBoolHeader(channel.IsFirstGroupConnection, true)

	initialUnderlay, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)

	dialChannel := NewDialCtrlChannel(DialCtrlChannelConfig{
		Dialer:                  dialer,
		MaxDefaultChannels:      1,
		MaxHighPriorityChannels: 1,
		MaxLowPriorityChannels:  1,
		UnderlayChangeCallback:  func(ch *DialCtrlChannel, oldCount, newCount uint32) {},
	}).(*DialCtrlChannel)

	multiConfig := &channel.MultiChannelConfig{
		LogicalName:     "ctrl/router-reconnect",
		Options:         channel.DefaultOptions(),
		UnderlayHandler: dialChannel,
		Underlay:        initialUnderlay,
	}

	multiCh, err := channel.NewMultiChannel(multiConfig)
	req.NoError(err)
	defer func() { _ = multiCh.Close() }()

	for i := 0; i < 10; i++ {
		t.Logf("=== Iteration %d ===", i)

		// Wait for all underlay types to reach their intended counts
		req.Eventually(func() bool {
			counts := multiCh.GetUnderlayCountsByType()
			return counts[ChannelTypeDefault] >= 1 &&
				counts[ChannelTypeHighPriority] >= 1 &&
				counts[ChannelTypeLowPriority] >= 1
		}, 15*time.Second, 50*time.Millisecond,
			"Iteration %d: underlays should reach intended counts", i)

		counts := multiCh.GetUnderlayCountsByType()
		t.Logf("Iteration %d: underlay counts: %v", i, counts)

		// Send echo messages via each priority sender and verify replies
		for _, tc := range []struct {
			name   string
			sender channel.Sender
		}{
			{"default", dialChannel.GetDefaultSender()},
			{"high-priority", dialChannel.GetHighPrioritySender()},
			{"low-priority", dialChannel.GetLowPrioritySender()},
		} {
			payload := fmt.Sprintf("echo-%d-%s", i, tc.name)
			msg := channel.NewMessage(echoContentType, []byte(payload))
			reply, err := msg.WithTimeout(5 * time.Second).SendForReply(tc.sender)
			req.NoError(err, "Iteration %d: send on %s should succeed", i, tc.name)
			req.Equal(payload, string(reply.Body),
				"Iteration %d: echo reply on %s should match", i, tc.name)
		}

		// Close all underlays to force reconnection
		underlays := multiCh.GetUnderlays()
		t.Logf("Iteration %d: closing %d underlays", i, len(underlays))
		for _, u := range underlays {
			_ = u.Close()
		}

		// Verify router channel stays open despite 0 underlays
		req.False(multiCh.IsClosed(),
			"Iteration %d: router channel should remain open after closing all underlays", i)
		req.Eventually(func() bool {
			return uint32(i+1) == dialChannel.iteration.Load()
		}, 5*time.Second, 10*time.Millisecond, "loop iteration %d, should match channel iteration %d", i, dialChannel.iteration.Load())
	}
}
