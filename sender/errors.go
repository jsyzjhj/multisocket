package sender

import (
	"github.com/webee/multisocket"
)

type err string

func (e err) Error() string {
	return string(e)
}

// errors
const (
	ErrClosed      = multisocket.ErrClosed
	ErrSendTimeout = err("send time out")
)
