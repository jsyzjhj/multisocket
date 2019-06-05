package connector

import (
	"fmt"
	"sync"

	"github.com/webee/multisocket"
	"github.com/webee/multisocket/socket"
	"github.com/webee/multisocket/transport"
)

type connector struct {
	limit int

	sync.Mutex
	dialers      []*dialer
	listeners    []*listener
	pipes        map[*pipe]struct{}
	pipeChannels map[chan<- socket.Pipe]struct{}
	closed       bool
}

const (
	// defaultMaxRxSize is the default maximum Rx size
	defaultMaxRxSize = 1024 * 1024
)

// New create a any Connector
func New() socket.Connector {
	return NewWithLimit(-1)
}

// NewWithLimit create a Connector with limited connections.
func NewWithLimit(n int) socket.Connector {
	c := &connector{
		limit:        n,
		dialers:      make([]*dialer, 0),
		listeners:    make([]*listener, 0),
		pipes:        make(map[*pipe]struct{}),
		pipeChannels: make(map[chan<- socket.Pipe]struct{}),
	}

	return c
}

func (c *connector) addPipe(p *pipe) {
	c.Lock()
	defer c.Unlock()
	if c.limit == -1 || c.limit > len(c.pipes) {
		fmt.Printf("add %d, %d/%d\n", p.ID(), len(c.pipes), c.limit)
		c.pipes[p] = struct{}{}
		for channel := range c.pipeChannels {
			channel <- p
		}
	} else {
		go p.Close()
	}

	// check exceed limit
	if c.limit != -1 && c.limit <= len(c.pipes) {
		// stop connect
		for _, l := range c.listeners {
			l.stop()
		}

		for _, d := range c.dialers {
			d.stop()
		}
	}
}

func (c *connector) remPipe(p *pipe) {
	fmt.Printf("rem %d, %d/%d\n", p.ID(), len(c.pipes), c.limit)
	c.Lock()
	delete(c.pipes, p)
	c.Unlock()

	// If the pipe was from a dialer, inform it so that it can redial.
	if d := p.d; d != nil {
		go d.pipeClosed()
	}

	c.Lock()
	defer c.Unlock()
	if c.limit != -1 && c.limit > len(c.pipes) {
		// below limit

		// start connect
		for _, l := range c.listeners {
			l.start()
		}

		for _, d := range c.dialers {
			d.start()
		}
	}
}

func (c *connector) Dial(addr string) error {
	return c.DialOptions(addr, multisocket.NewOptions())
}

func (c *connector) DialOptions(addr string, opts multisocket.Options) error {
	d, err := c.NewDialer(addr, opts)
	if err != nil {
		return err
	}
	return d.Dial()
}

func (c *connector) NewDialer(addr string, opts multisocket.Options) (d socket.Dialer, err error) {
	c.Lock()
	defer c.Unlock()

	if c.closed {
		err = multisocket.ErrClosed
		return
	}

	var (
		t  transport.Transport
		td transport.Dialer
	)

	if t = transport.GetTransportFromAddr(addr); t == nil {
		err = multisocket.ErrBadTran
		return
	}

	if td, err = t.NewDialer(addr); err != nil {
		return
	}
	td.SetOption(transport.OptionMaxRecvSize, defaultMaxRxSize)

	xd := newDialer(c, td)
	if c.limit != -1 && c.limit <= len(c.pipes) {
		// exceed limit
		xd.stop()
	}
	d = xd
	for _, ov := range opts.OptionValues() {
		if err = d.SetOption(ov.Option, ov.Value); err != nil {
			return
		}
	}

	c.dialers = append(c.dialers, xd)
	return d, nil
}

func (c *connector) Listen(addr string) error {
	return c.ListenOptions(addr, multisocket.NewOptions())
}

func (c *connector) ListenOptions(addr string, opts multisocket.Options) error {
	l, err := c.NewListener(addr, opts)
	if err != nil {
		return err
	}
	return l.Listen()
}

func (c *connector) NewListener(addr string, opts multisocket.Options) (l socket.Listener, err error) {
	c.Lock()
	defer c.Unlock()

	if c.closed {
		err = multisocket.ErrClosed
		return
	}

	var (
		t  transport.Transport
		tl transport.Listener
	)

	if t = transport.GetTransportFromAddr(addr); t == nil {
		err = multisocket.ErrBadTran
		return
	}

	if tl, err = t.NewListener(addr); err != nil {
		return
	}
	tl.SetOption(transport.OptionMaxRecvSize, defaultMaxRxSize)

	for _, ov := range opts.OptionValues() {
		if err = tl.SetOption(ov.Option, ov.Value); err != nil {
			tl.Close()
			return
		}
	}

	xl := newListener(c, tl)
	if c.limit != -1 && c.limit <= len(c.pipes) {
		// exceed limit
		xl.stop()
	}
	l = xl

	c.listeners = append(c.listeners, xl)

	return
}

func (c *connector) Close() error {
	c.Lock()
	if c.closed {
		c.Unlock()
		return multisocket.ErrClosed
	}
	listeners := c.listeners
	dialers := c.dialers
	pipes := c.pipes

	c.listeners = nil
	c.dialers = nil
	c.pipes = nil
	c.Unlock()

	for _, l := range listeners {
		l.Close()
	}
	for _, d := range dialers {
		d.Close()
	}

	for p := range pipes {
		p.Close()
	}

	return nil
}

func (c *connector) AddPipeChannel(channel chan<- socket.Pipe) {
	c.Lock()
	c.pipeChannels[channel] = struct{}{}
	c.Unlock()
}

func (c *connector) RemovePipeChannel(channel chan<- socket.Pipe) {
	c.Lock()
	delete(c.pipeChannels, channel)
	c.Unlock()
}
