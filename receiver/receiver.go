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
	defaultRecvQueueSize = uint16(64)
)

var (
	nilQ    <-chan time.Time
	closedQ chan time.Time
)

func init() {
	closedQ = make(chan time.Time)
	close(closedQ)
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
	sendType := header.SendType()
	headerSize := header.Size()

	source = newPathFromBytes(int(header.Hops), payload[headerSize:])
	sourceSize := source.Size()

	if sendType == multisocket.SendTypeToDest {
		dest = newPathFromBytes(header.DestLength(), payload[headerSize+sourceSize:])
	}

	content := payload[headerSize+sourceSize+dest.Size():]

	// update source, add current pipe id
	source = source.AddID(p.p.ID())
	header.Hops++
	header.TTL--

	if sendType == multisocket.SendTypeToDest {
		// update destination, remove last pipe id
		if _, dest, ok = dest.NextID(); !ok {
			// anyway, msg arrived
			header.Distance = 0
		} else {
			header.Distance = dest.Length()
		}
	}

	msg = &multisocket.Message{
		Header:      header,
		Source:      source,
		Destination: dest,
		Content:     content,
	}
	return
}

func (p *pipe) recvRawMsg() (msg *multisocket.Message, err error) {
	var (
		payload []byte
	)
	if payload, err = p.p.Recv(); err != nil {
		return
	}

	msg = multisocket.NewMessage(multisocket.SendTypeToOne, nil, 0, payload)
	msg.Source = msg.Source.AddID(p.p.ID())
	msg.Header.Hops = msg.Source.Length()

	return
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

func (r *receiver) recvTimeout() time.Duration {
	return OptionRecvTimeout.Value(r.GetOptionDefault(OptionRecvTimeout, time.Duration(0)))
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
		p.close()
	}
}

func (p *pipe) close() {
	select {
	case <-p.closedq:
	default:
		close(p.closedq)
		p.p.Close()
	}
}

func (r *receiver) run(p *pipe) {
	if log.IsLevelEnabled(log.DebugLevel) {
		log.WithField("domain", "receiver").
			WithFields(log.Fields{"id": p.p.ID(), "raw": p.p.IsRaw()}).
			Debug("receiver start run")
	}

	recvMsg := p.recvMsg
	if p.p.IsRaw() {
		recvMsg = p.recvRawMsg

		// NOTE:
		// send a empty control to make a connection
		msg := multisocket.NewMessage(multisocket.SendTypeToOne, nil, multisocket.MsgFlagControl, nil)
		// update source, add current pipe id
		msg.Source = msg.Source.AddID(p.p.ID())
		msg.Header.Hops = msg.Source.Length()

		r.recvq <- msg
	}

	var (
		err error
		msg *multisocket.Message
	)
RECVING:
	for {
		if msg, err = recvMsg(); err != nil {
			if log.IsLevelEnabled(log.DebugLevel) {
				log.WithField("domain", "receiver").
					WithError(err).
					WithFields(log.Fields{"id": p.p.ID()}).
					Debug("recvMsg")
			}
			break
		}
		if msg == nil {
			// ignore nil msg
			continue
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
	if log.IsLevelEnabled(log.DebugLevel) {
		log.WithField("domain", "receiver").
			WithFields(log.Fields{"id": p.p.ID(), "raw": p.p.IsRaw()}).
			Debug("receiver stopped run")
	}
}

func (r *receiver) RecvMsg() (msg *multisocket.Message, err error) {
	var (
		timeoutTimer *time.Timer
		retryTimer   *time.Timer
		retryq       <-chan time.Time = closedQ
		retryTimes                    = time.Duration(0)
	)

	recvTimeout := r.recvTimeout()
	tq := nilQ
	if recvTimeout > 0 {
		timeoutTimer = time.NewTimer(recvTimeout)
		tq = timeoutTimer.C
	}

	for {
		select {
		case <-r.closedq:
			err = ErrClosed
		case <-tq:
			err = ErrTimeout
		case msg = <-r.recvq:
		case <-retryq:
			// FIXME: fix block on recvq channel bug: recvq is not empty but block here.
			if len(r.recvq) > 0 {
				retryq = nilQ
				continue
			}
			if retryTimes < 100 {
				retryTimes++
			}
			retryTimer = time.NewTimer(1 * time.Second)
			retryq = retryTimer.C
			if log.IsLevelEnabled(log.TraceLevel) {
				log.WithField("domain", "receiver").Tracef("retry recv: %d", retryTimes)
			}
			continue
		}
		if retryTimer != nil {
			retryTimer.Stop()
		}
		if timeoutTimer != nil {
			timeoutTimer.Stop()
		}
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
