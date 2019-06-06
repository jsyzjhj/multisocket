package socket

import (
	"github.com/webee/multisocket/options"
)

type (
	// Socket is a network peer
	Socket interface {
		Connector() Connector
		Sender() Sender
		Receiver() Receiver

		Dial(addr string) error
		DialOptions(addr string, opts options.Options) error
		NewDialer(addr string, opts options.Options) (Dialer, error)

		Listen(addr string) error
		ListenOptions(addr string, opts options.Options) error
		NewListener(addr string, opts options.Options) (Listener, error)

		SendTo(src MsgSource, content []byte) error // for reply send
		SendMsg(msg *Message) error                 // for forward send
		Send(content []byte) error                  // for initiative send

		RecvMsg() (*Message, error)
		Recv() ([]byte, error)

		Close() error
	}

	// Dialer is for connecting a listening socket.
	Dialer interface {
		options.Options

		Dial() error
		Close() error
	}

	// Listener is for listening and accepting connections.
	Listener interface {
		options.Options

		Listen() error
		Close() error
	}
)

type (
	// Pipe is a connection between two peer.
	Pipe interface {
		ID() uint32
		LocalAddress() string
		RemoteAddress() string

		Send(msgs ...[]byte) error
		Recv() ([]byte, error)

		Close() error
	}
)

// PipeEvent is pipe event
type PipeEvent int

// pipe events
const (
	PipeEventAdd PipeEvent = iota
	PipeEventRemove
)

// PipeEventHook is pipe event hook
type PipeEventHook func(PipeEvent, Pipe)

type (
	// Connector controls socket's connections
	Connector interface {
		Dial(addr string) error
		DialOptions(addr string, opts options.Options) error
		NewDialer(addr string, opts options.Options) (Dialer, error)

		Listen(addr string) error
		ListenOptions(addr string, opts options.Options) error
		NewListener(addr string, opts options.Options) (Listener, error)

		Close()

		RegisterPipeEventHook(PipeEventHook)
		UnregisterPipeEventHook(PipeEventHook)
	}

	// Sender controls socket's send.
	Sender interface {
		options.Options

		AttachConnector(Connector)

		SendTo(src MsgSource, content []byte) error // for reply send
		SendMsg(msg *Message) error                 // for forward send
		Send(content []byte) error                  // for initiative send

		Close()
	}

	// Receiver controls socket's recv.
	Receiver interface {
		options.Options

		AttachConnector(Connector)

		RecvMsg() (*Message, error)
		Recv() ([]byte, error)

		Close()
	}
)
