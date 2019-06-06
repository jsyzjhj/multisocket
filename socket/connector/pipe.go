package connector

import (
	"math/rand"
	"sync"
	"time"

	"github.com/webee/multisocket/transport"
)

// pipe wraps the transport.Connection data structure with the stuff we need to keep.
// It implements the Pipe interface.
type pipe struct {
	sync.Mutex
	id     uint32
	parent *connector
	c      transport.Connection
	l      *listener
	d      *dialer
	closed bool
}

// The conns global state is just an ID allocator
type idGen struct {
	sync.Mutex
	IDs  map[uint32]struct{}
	next uint32
}

func (g *idGen) nextID() (id uint32) {
	g.Lock()
	defer g.Unlock()
	for {
		id = g.next & 0x7fffffff
		g.next++
		if id == 0 {
			continue
		}
		if _, ok := g.IDs[id]; !ok {
			g.IDs[id] = struct{}{}
			break
		}
	}
	return
}

func (g *idGen) free(id uint32) {
	g.Lock()
	delete(g.IDs, id)
	g.Unlock()
}

var pipeID = &idGen{
	IDs:  make(map[uint32]struct{}),
	next: uint32(rand.NewSource(time.Now().UnixNano()).Int63()),
}

func newPipe(parent *connector, tc transport.Connection, d *dialer, l *listener) *pipe {
	return &pipe{
		id:     pipeID.nextID(),
		parent: parent,
		c:      tc,
		d:      d,
		l:      l,
	}
}

func (p *pipe) ID() uint32 {
	return p.id
}

func (p *pipe) LocalAddress() string {
	return p.c.LocalAddress()
}

func (p *pipe) RemoteAddress() string {
	return p.c.RemoteAddress()
}

func (p *pipe) Close() error {
	p.Lock()
	if p.closed {
		p.Unlock()
		return nil
	}
	p.closed = true
	p.Unlock()

	p.c.Close()
	p.parent.remPipe(p)

	// This is last, as we keep the ID reserved until everything is
	// done with it.
	pipeID.free(p.id)

	return nil
}

func (p *pipe) Send(msgs ...[]byte) (err error) {
	if err = p.c.Send(msgs...); err != nil {
		// NOTE: close on any error
		go p.Close()
	}
	return
}

func (p *pipe) Recv() (msg []byte, err error) {
	if msg, err = p.c.Recv(); err != nil {
		// NOTE: close on any error
		go p.Close()
	}

	return
}
