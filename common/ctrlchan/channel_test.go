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
	time.Sleep(500 * time.Millisecond)

	// Verify both sides are connected
	req.False(multiCh.IsClosed(), "Router channel should be open")
	req.Equal(int32(1), acceptCount.Load(), "Controller should have accepted one connection")

	acceptedLock.Lock()
	req.Len(acceptedChannels, 1, "Should have one accepted channel")
	controllerCh := acceptedChannels[0]
	acceptedLock.Unlock()

	req.False(controllerCh.IsClosed(), "Controller channel should be open")

	// Close the initial underlay to simulate connection failure
	t.Log("Closing initial underlay to simulate connection failure")
	err = initialUnderlay.Close()
	req.NoError(err)

	// Wait for the closure to be detected and processed
	time.Sleep(500 * time.Millisecond)

	// Verify router channel stays open (key behavior)
	req.False(multiCh.IsClosed(), "Router channel should remain open even at 0 underlays")

	// Verify controller channel closed (expected behavior)
	req.True(controllerCh.IsClosed(), "Controller channel should close when underlay count hits 0")

	// Wait for router to attempt re-dial
	time.Sleep(2 * time.Second)

	// Verify that a new connection was accepted (router re-dialed)
	req.GreaterOrEqual(acceptCount.Load(), int32(2), "Router should have re-dialed and controller accepted new connection")

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
	time.Sleep(500 * time.Millisecond)

	channelLock.Lock()
	ctrlCh := controllerChannel
	channelLock.Unlock()

	req.NotNil(ctrlCh, "Controller should have accepted connection")
	req.False(ctrlCh.IsClosed(), "Controller channel should be open")

	// Close the underlay
	t.Log("Closing underlay")
	err = underlay.Close()
	req.NoError(err)

	// Wait for closure to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify controller channel is now closed
	req.True(ctrlCh.IsClosed(), "Controller channel should close when underlay count hits 0")
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
	time.Sleep(3 * time.Second)

	// Get the accepted channel from the listener side
	acceptedChannelLock.Lock()
	serverCh := acceptedChannel
	acceptedChannelLock.Unlock()

	req.NotNil(serverCh, "Server should have a channel")

	// Get underlay counts from both sides
	clientCounts := multiCh.GetUnderlayCountsByType()
	serverCounts := serverCh.GetUnderlayCountsByType()

	t.Logf("Client underlay counts: %v", clientCounts)
	t.Logf("Server underlay counts: %v", serverCounts)

	// Verify all three priority levels established connections
	req.Greater(serverCounts[ChannelTypeDefault], 0, "Should have default priority connections")
	req.Greater(serverCounts[ChannelTypeHighPriority], 0, "Should have high priority connections")
	req.Greater(serverCounts[ChannelTypeLowPriority], 0, "Should have low priority connections")

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
	time.Sleep(500 * time.Millisecond)

	req.Equal(int32(1), acceptCount.Load(), "Should have accepted first connection")

	acceptedLock.Lock()
	firstCh := acceptedChannels[0]
	acceptedLock.Unlock()

	req.False(firstCh.IsClosed(), "First channel should be open")

	// Close first connection
	t.Log("Closing first connection")
	err = underlay1.Close()
	req.NoError(err)

	// Wait for channel to close
	time.Sleep(500 * time.Millisecond)

	req.True(firstCh.IsClosed(), "First channel should be closed")

	// Create second connection (simulating router reconnection)
	t.Log("Creating second connection")
	underlay2, err := dialer.CreateWithHeaders(5*time.Second, headers)
	req.NoError(err)
	defer func() { _ = underlay2.Close() }()

	// Wait for new connection to be accepted
	time.Sleep(500 * time.Millisecond)

	// Verify new connection was accepted as a separate channel
	req.Equal(int32(2), acceptCount.Load(), "Should have accepted second connection as new")

	acceptedLock.Lock()
	req.Len(acceptedChannels, 2, "Should have two separate channels")
	secondCh := acceptedChannels[1]
	acceptedLock.Unlock()

	req.False(secondCh.IsClosed(), "Second channel should be open")
	req.NotEqual(firstCh, secondCh, "Should be different channel instances")
}
