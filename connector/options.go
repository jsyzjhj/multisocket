package connector

import (
	"time"

	"github.com/webee/multisocket/options"
)

type (
	dialerOptions struct {
		// reconnect when pipe closed
		Reconnect        options.BoolOption
		MinReconnectTime options.TimeDurationOption
		MaxReconnectTime options.TimeDurationOption
		DialAsync        options.BoolOption
	}

	pipeOptions struct {
		RawMode        options.BoolOption
		RawRecvBufSize options.IntOption
		// close pipe when peer shutdown write(half-close)
		CloseOnEOF options.BoolOption
	}

	connectorOptions struct {
		PipeLimit options.IntOption
		Dialer    dialerOptions
		Pipe      pipeOptions
	}
)

var (
	// OptionDomains is option's domain
	OptionDomains = []string{"Connector"}
	// Options for connector
	Options = connectorOptions{
		PipeLimit: options.NewIntOption(-1), // -1: no limit
		Dialer: dialerOptions{
			Reconnect:        options.NewBoolOption(true),
			MinReconnectTime: options.NewTimeDurationOption(100 * time.Millisecond),
			MaxReconnectTime: options.NewTimeDurationOption(8 * time.Second),
			DialAsync:        options.NewBoolOption(false),
		},
		Pipe: pipeOptions{
			RawMode:        options.NewBoolOption(false),
			RawRecvBufSize: options.NewIntOption(4 * 1024),
			CloseOnEOF:     options.NewBoolOption(true),
		},
	}
)

func init() {
	options.RegisterStructuredOptions(Options, OptionDomains)
}
