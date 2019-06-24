package inproc

import (
	"fmt"
	"io"
	"sync"

	"github.com/webee/multisocket/options"

	"github.com/webee/multisocket/errs"
	"github.com/webee/multisocket/transport"
)

type (
	inprocTran string

	dialer struct {
		addr string
	}

	listener struct {
		addr    string
		accepts chan chan *inprocConn
		sync.Mutex
		closedq chan struct{}
	}

	// inprocConn implements PrimitiveConnection based on io.Pipe
	inprocConn struct {
		sync.Mutex
		laddr string
		raddr string
		pr    *io.PipeReader
		pw    *io.PipeWriter
	}
)

const (
	// Transport is a transport.Transport for intra-process communication.
	Transport = inprocTran("inproc")

	defaultAcceptQueueSize = 8
)

var listeners struct {
	sync.RWMutex
	// Who is listening, on which "address"?
	byAddr map[string]*listener
}

func init() {
	listeners.byAddr = make(map[string]*listener)

	io.Pipe()
	transport.RegisterTransport(Transport)
}

// inproc

func (p *inprocConn) Read(b []byte) (n int, err error) {
	return p.pr.Read(b)
}

func (p *inprocConn) Write(b []byte) (n int, err error) {
	return p.pw.Write(b)
}

func (p *inprocConn) Close() error {
	p.pr.Close()
	p.pw.Close()
	return nil
}

func (p *inprocConn) LocalAddress() string {
	return fmt.Sprintf("%s://%s", Transport.Scheme(), p.laddr)
}

func (p *inprocConn) RemoteAddress() string {
	return fmt.Sprintf("%s://%s", Transport.Scheme(), p.raddr)
}

// dialer

func (d *dialer) Dial(opts options.Options) (transport.Connection, error) {
	var (
		l  *listener
		ok bool
	)

	listeners.RLock()
	if l, ok = listeners.byAddr[d.addr]; !ok {
		listeners.RUnlock()
		return nil, transport.ErrConnRefused
	}
	listeners.RUnlock()

	ac := make(chan *inprocConn)
	select {
	case <-l.closedq:
		return nil, transport.ErrConnRefused
	case l.accepts <- ac:
	}

	select {
	case <-l.closedq:
		return nil, transport.ErrConnRefused
	case dc := <-ac:
		return transport.NewConnection(opts, Transport, dc)
	}
}

// listener

func (l *listener) Listen(opts options.Options) error {
	select {
	case <-l.closedq:
		return errs.ErrClosed
	default:
	}

	listeners.Lock()
	if xl, ok := listeners.byAddr[l.addr]; ok {
		listeners.Unlock()
		if xl != l {
			return errs.ErrAddrInUse
		}
		// already in listening
		return nil
	}

	l.accepts = make(chan chan *inprocConn, defaultAcceptQueueSize)

	listeners.byAddr[l.addr] = l
	listeners.Unlock()
	return nil
}

func (l *listener) Accept(opts options.Options) (transport.Connection, error) {
	select {
	case <-l.closedq:
		return nil, errs.ErrClosed
	default:
	}

	listeners.RLock()
	if listeners.byAddr[l.addr] != l {
		listeners.Unlock()
		// not in listening
		return nil, transport.ErrNotListening
	}
	listeners.RUnlock()

	select {
	case <-l.closedq:
		return nil, errs.ErrClosed
	case ac := <-l.accepts:
		lpr, rpw := io.Pipe()
		rpr, lpw := io.Pipe()
		// setup accept conn
		lc := &inprocConn{
			laddr: l.addr,
			raddr: l.addr + ".dialer",
			pr:    lpr,
			pw:    lpw,
		}
		// setup dialer conn
		dc := &inprocConn{
			laddr: lc.raddr,
			raddr: lc.laddr,
			pr:    rpr,
			pw:    rpw,
		}

		// notify dialer
		select {
		case <-l.closedq:
			return nil, errs.ErrClosed
		case ac <- dc:
		}

		return transport.NewConnection(opts, Transport, lc)
	}
}

func (l *listener) Close() error {
	l.Lock()
	select {
	case <-l.closedq:
		l.Unlock()
		return errs.ErrClosed
	default:
		close(l.closedq)
	}
	l.Unlock()

	listeners.Lock()
	if listeners.byAddr[l.addr] == l {
		delete(listeners.byAddr, l.addr)
	}
	listeners.Unlock()

	return nil
}

// inprocTran

func (t inprocTran) Scheme() string {
	return string(t)
}

func (t inprocTran) NewDialer(addr string) (transport.Dialer, error) {
	var err error
	if addr, err = transport.StripScheme(t, addr); err != nil {
		return nil, err
	}

	d := &dialer{addr: addr}
	return d, nil
}

func (t inprocTran) NewListener(addr string) (transport.Listener, error) {
	var err error
	if addr, err = transport.StripScheme(t, addr); err != nil {
		return nil, err
	}

	l := &listener{
		addr:    addr,
		closedq: make(chan struct{}),
	}
	return l, nil
}
