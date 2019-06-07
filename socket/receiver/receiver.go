package receiver

import (
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/webee/multisocket/options"
	"github.com/webee/multisocket/socket"
)

type (
	receiver struct {
		options.Options

		recvNone bool // no recving

		sync.Mutex
		attachedConnectors map[socket.Connector]struct{}
		closed             bool
		closedq            chan struct{}
		pipes              map[uint32]*pipe
		recvq              chan *socket.Message
	}

	pipe struct {
		closedq chan struct{}
		p       socket.Pipe
	}
)

const (
	defaultRecvQueueSize = uint16(8)
)

func newPipe(p socket.Pipe) *pipe {
	return &pipe{
		closedq: make(chan struct{}),
		p:       p,
	}
}

func (p *pipe) recvMsg() (msg *socket.Message, err error) {
	var (
		payload []byte
		header  *socket.MsgHeader
		source  socket.MsgSource
	)
	if payload, err = p.p.Recv(); err != nil {
		return
	}

	if header, err = NewHeaderFromBytes(payload); err != nil {
		return
	}
	headerSize := header.Size()
	source = NewSourceFromBytes(int(header.Hops), payload[headerSize:])
	content := payload[headerSize+source.Size():]

	source = source.NewID(p.p.ID())
	header.Hops++
	header.TTL--
	msg = &socket.Message{
		Header:  header,
		Source:  source,
		Content: content,
	}
	return
}

// New create a normal receiver.
func New() socket.Receiver {
	return NewWithOptions()
}

// NewWithOptions create a normal receiver with options.
func NewWithOptions(ovs ...*options.OptionValue) socket.Receiver {
	return newWithOptions(false, ovs...)
}

// NewRecvNone create an recv none receiver.
func NewRecvNone() socket.Receiver {
	return newWithOptions(true)
}

func newWithOptions(recvNone bool, ovs ...*options.OptionValue) socket.Receiver {
	r := &receiver{
		Options:            options.NewOptions(),
		recvNone:           recvNone,
		attachedConnectors: make(map[socket.Connector]struct{}),
		closed:             false,
		closedq:            make(chan struct{}),
		pipes:              make(map[uint32]*pipe),
	}
	for _, ov := range ovs {
		r.SetOption(ov.Option, ov.Value)
	}

	return r
}

func (r *receiver) AttachConnector(connector socket.Connector) {
	r.Lock()
	defer r.Unlock()

	// OptionRecvQueueSize useless after first attach
	if !r.recvNone && r.recvq == nil {
		r.recvq = make(chan *socket.Message, r.recvQueueSize())
	}

	connector.RegisterPipeEventHandler(r)
	r.attachedConnectors[connector] = struct{}{}
}

// options
func (r *receiver) recvQueueSize() uint16 {
	return OptionRecvQueueSize.Value(r.GetOptionDefault(OptionRecvQueueSize, defaultRecvQueueSize))
}

func (r *receiver) HandlePipeEvent(e socket.PipeEvent, pipe socket.Pipe) {
	switch e {
	case socket.PipeEventAdd:
		r.addPipe(pipe)
	case socket.PipeEventRemove:
		r.remPipe(pipe.ID())
	}
}

func (r *receiver) addPipe(pipe socket.Pipe) {
	r.Lock()
	defer r.Unlock()
	p := newPipe(pipe)
	r.pipes[p.p.ID()] = p
	go r.run(p)
}

func (r *receiver) remPipe(id uint32) {
	r.Lock()
	defer r.Unlock()
	p, ok := r.pipes[id]
	if ok {
		delete(r.pipes, id)
		// async close pipe, avoid dead lock the receiver.
		go r.closePipe(p)
	}
}

func (r *receiver) closePipe(p *pipe) {
	select {
	case <-p.closedq:
	default:
		close(p.closedq)
	}
}

func (r *receiver) run(p *pipe) {
	log.WithField("domain", "receiver").
		WithFields(log.Fields{"id": p.p.ID()}).
		Debug("pipe start run")
	var (
		err error
		msg *socket.Message
	)

RECVING:
	for {
		if msg, err = p.recvMsg(); err != nil {
			break
		}
		if !r.recvNone {
			r.recvq <- msg
		}
		// else drop msg

		select {
		case <-r.closedq:
			break RECVING
		case <-p.closedq:
			break RECVING
		default:
		}
	}
	r.remPipe(p.p.ID())
	log.WithField("domain", "receiver").
		WithFields(log.Fields{"id": p.p.ID()}).
		Debug("pipe stopped run")
}

func (r *receiver) RecvMsg() (msg *socket.Message, err error) {
	if r.recvNone {
		err = ErrRecvNotAllowd
		return
	}

	msg = <-r.recvq
	return
}

func (r *receiver) Recv() (content []byte, err error) {
	if r.recvNone {
		err = ErrRecvNotAllowd
		return
	}

	var msg *socket.Message
	if msg, err = r.RecvMsg(); err != nil {
		return
	}
	content = msg.Content
	return
}

func (r *receiver) Close() {
	r.Lock()
	defer r.Unlock()
	if r.closed {
		return
	}
	r.closed = true

	// unregister
	for conns := range r.attachedConnectors {
		conns.UnregisterPipeEventHandler(r)
		delete(r.attachedConnectors, conns)
	}

	close(r.closedq)
}
