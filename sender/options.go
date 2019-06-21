package sender

import (
	"github.com/webee/multisocket/options"
)

type optionName int

const (
	optionNameTTL optionName = iota
	optionNameSendQueueSize
	optionNameSendBestEffort
)

// Options
var (
	OptionTTL            = options.NewUint8Option(optionNameTTL)
	OptionSendQueueSize  = options.NewUint16Option(optionNameSendQueueSize)
	OptionSendBestEffort = options.NewBoolOption(optionNameSendBestEffort)
)
