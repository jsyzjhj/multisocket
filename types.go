package multisocket

import (
	"github.com/multisocket/multisocket/connector"
	"github.com/multisocket/multisocket/receiver"
	"github.com/multisocket/multisocket/sender"
)

type (
	// Socket is a network peer
	Socket interface {
		connector.Action
		sender.Action
		receiver.Action

		Close() error

		GetConnector() connector.Connector
		GetSender() sender.Sender
		GetReceiver() receiver.Receiver
	}
)
