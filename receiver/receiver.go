package receiver

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/webee/multisocket"
	"github.com/webee/multisocket/options"
)

type (
	receiver struct {
		options.Options

		sync.Mutex
		attachedConnectors map[multisocket.Connector]struct{}
		closed             bool
		closedq            chan struct{}
		pipes              map[uint32]*pipe
		recvq              chan *multisocket.Message
	}

	pipe struct {
		closedq chan struct{}
		p       multisocket.Pipe
	}
)

const (
	defaultRecvQueueSize = uint16(8)
)

var (
	nilQ <-chan time.Time
)

func newPipe(p multisocket.Pipe) *pipe {
	return &pipe{
		closedq: make(chan struct{}),
		p:       p,
	}
}

func (p *pipe) recvMsg() (msg *multisocket.Message, err error) {
	var (
		ok      bool
		payload []byte
		header  *multisocket.MsgHeader
		source  multisocket.MsgPath
		dest    multisocket.MsgPath
	)
	if payload, err = p.p.Recv(); err != nil {
		return
	}

	if header, err = newHeaderFromBytes(payload); err != nil {
		return
	}
	headerSize := header.Size()

	source = newPathFromBytes(int(header.Hops), payload[headerSize:])
	// update source, add current pipe id
	source = source.AddID(p.p.ID())
	header.Hops++
	header.TTL--
	sourceSize := source.Size()

	if header.SendType == multisocket.SendTypeReply {
		dest = newPathFromBytes(header.DestLength(), payload[headerSize+sourceSize:])
		// update destination, remove last pipe id
		if _, dest, ok = dest.NextID(); !ok {
			// anyway, msg arrived
			header.Distance = 0
		} else {
			header.Distance = dest.Length()
		}
	}

	content := payload[headerSize+sourceSize+dest.Size():]

	msg = &multisocket.Message{
		Header:      header,
		Source:      source,
		Destination: dest,
		Content:     content,
	}
	return
}

// New create a receiver.
func New() multisocket.Receiver {
	return NewWithOptions()
}

// NewWithOptions create a normal receiver with options.
func NewWithOptions(ovs ...*options.OptionValue) multisocket.Receiver {
	r := &receiver{
		Options:            options.NewOptions(),
		attachedConnectors: make(map[multisocket.Connector]struct{}),
		closed:             false,
		closedq:            make(chan struct{}),
		pipes:              make(map[uint32]*pipe),
	}
	for _, ov := range ovs {
		r.SetOption(ov.Option, ov.Value)
	}

	return r
}

func (r *receiver) AttachConnector(connector multisocket.Connector) {
	r.Lock()
	defer r.Unlock()

	// OptionRecvQueueSize useless after first attach
	if r.recvq == nil {
		r.recvq = make(chan *multisocket.Message, r.recvQueueSize())
	}

	connector.RegisterPipeEventHandler(r)
	r.attachedConnectors[connector] = struct{}{}
}

// options
func (r *receiver) recvQueueSize() uint16 {
	return OptionRecvQueueSize.Value(r.GetOptionDefault(OptionRecvQueueSize, defaultRecvQueueSize))
}

func (r *receiver) recvDeadline() time.Duration {
	return OptionRecvDeadline.Value(r.GetOptionDefault(OptionRecvDeadline, time.Duration(0)))
}

func (r *receiver) HandlePipeEvent(e multisocket.PipeEvent, pipe multisocket.Pipe) {
	switch e {
	case multisocket.PipeEventAdd:
		r.addPipe(pipe)
	case multisocket.PipeEventRemove:
		r.remPipe(pipe.ID())
	}
}

func (r *receiver) addPipe(pipe multisocket.Pipe) {
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
		msg *multisocket.Message
	)

RECVING:
	for {
		if msg, err = p.recvMsg(); err != nil {
			break
		}

		select {
		case <-r.closedq:
			break RECVING
		case <-p.closedq:
			// try recv pipe's last msg
			select {
			case <-r.closedq:
				break RECVING
			case r.recvq <- msg:
			}
			break RECVING
		case r.recvq <- msg:
		}
	}
	r.remPipe(p.p.ID())
	log.WithField("domain", "receiver").
		WithFields(log.Fields{"id": p.p.ID()}).
		Debug("pipe stopped run")
}

func (r *receiver) RecvMsg() (msg *multisocket.Message, err error) {
	recvDeadline := r.recvDeadline()
	tq := nilQ
	if recvDeadline > 0 {
		tq = time.After(recvDeadline)
	}

	select {
	case <-r.closedq:
		err = ErrClosed
		return
	case <-tq:
		err = ErrTimeout
		return
	case msg = <-r.recvq:
		return
	}
}

func (r *receiver) Recv() (content []byte, err error) {
	var msg *multisocket.Message
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
