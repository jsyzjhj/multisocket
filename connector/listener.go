package connector

import (
	"sync"
	"time"

	"github.com/webee/multisocket/options"
	"github.com/webee/multisocket/transport"
)

type listener struct {
	options.Options

	parent *connector
	addr   string
	l      transport.Listener
	sync.Mutex
	closed bool

	stopped bool
}

func newListener(parent *connector, addr string, tl transport.Listener) *listener {
	opts := options.NewOptionsWithUpDownStreamsAndAccepts(tl, tl)
	return &listener{
		Options: opts,
		parent:  parent,
		addr:    addr,
		l:       tl,
		closed:  false,
	}
}

func (l *listener) start() {
	l.Lock()
	defer l.Unlock()
	if !l.stopped {
		return
	}

	l.stopped = false
}

func (l *listener) stop() {
	l.Lock()
	defer l.Unlock()
	if l.stopped {
		return
	}

	l.stopped = true
}

func (l *listener) isStopped() bool {
	l.Lock()
	defer l.Unlock()
	return l.stopped
}

// serve spins in a loop, calling the accepter's Accept routine.
func (l *listener) serve() {
	for {
		// If the underlying PipeListener is closed, or not
		// listening, we expect to return back with an error.
		if tc, err := l.l.Accept(); err == transport.ErrClosed {
			return
		} else if err == nil {
			if l.isStopped() {
				tc.Close()
			} else {
				go l.parent.addPipe(newPipe(l.parent, tc, nil, l))
			}
		} else {
			// Debounce a little bit, to avoid thrashing the CPU.
			time.Sleep(time.Second / 100)
		}
	}
}

func (l *listener) Listen() error {
	if err := l.l.Listen(); err != nil {
		return err
	}

	go l.serve()
	return nil
}

func (l *listener) Close() error {
	l.Lock()
	defer l.Unlock()
	if l.closed {
		return ErrClosed
	}
	l.closed = true
	return l.l.Close()
}
