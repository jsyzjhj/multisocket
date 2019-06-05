package connector

import (
	"sync"
	"time"

	"github.com/webee/multisocket"
	"github.com/webee/multisocket/transport"
)

type listener struct {
	multisocket.Options

	parent *connector
	l      transport.Listener
	sync.Mutex
	closed bool

	stopped bool
}

func newListener(parent *connector, tl transport.Listener) *listener {
	opts := multisocket.NewOptionsWithUpDownStreamsAndAccepts(tl, tl)
	return &listener{
		Options: opts,
		parent:  parent,
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
		if tc, err := l.l.Accept(); err == multisocket.ErrClosed {
			return
		} else if err == nil {
			if l.isStopped() {
				tc.Close()
			} else {
				l.parent.addPipe(newPipe(l.parent, tc, nil, l))
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
		return multisocket.ErrClosed
	}
	l.closed = true
	return l.l.Close()
}
