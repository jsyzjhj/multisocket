package address

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/multisocket/multisocket/options"
)

type (
	// DialListener is for connecting peers
	DialListener interface {
		DialOptions(addr string, ovs options.OptionValues) error
		ListenOptions(addr string, ovs options.OptionValues) error
	}

	// MultiSocketAddress group dial/listen, async, raw and address together
	MultiSocketAddress interface {
		String() string
		ConnectType() string
		Address() string
		OptionValues() options.OptionValues
		Connect(ctr DialListener, ovses ...options.OptionValues) error
		Dial(ctr DialListener, ovses ...options.OptionValues) error
		Listen(ctr DialListener, ovses ...options.OptionValues) error
	}

	multiSocketAddress struct {
		raw      string
		connType string
		addr     string
		ovs      options.OptionValues
	}
)

// errors
var (
	ErrBadConnectType     = errors.New("bad connect type")
	ErrConnectTypeMissing = errors.New("connect type missing")
)

// Connect Types
const (
	ConnDial   = "dial"
	ConnListen = "listen"
)

// ParseMultiSocketAddress parse s to a MultiSocketAddress
func ParseMultiSocketAddress(s string) (sa MultiSocketAddress, err error) {
	var (
		u *url.URL
	)
	if u, err = url.Parse(s); err != nil {
		return
	}

	var (
		connType string
		addr     = fmt.Sprintf("%s://%s%s", u.Scheme, u.Host, u.Path)
		ovs      = options.OptionValues{}
	)

	switch u.Fragment {
	case "dial":
		connType = ConnDial
	case "listen":
		connType = ConnListen
	case "":
		// connect type missing
	default:
		err = ErrBadConnectType
		return
	}

	q := u.Query()
	for k := range q {
		opt, perr := options.ParseOption(k)
		if perr != nil {
			return nil, perr
		}
		v := q.Get(k)
		val, perr := opt.Parse(v)
		if perr != nil {
			return nil, perr
		}
		ovs[opt] = val
	}

	address := &multiSocketAddress{
		raw:      s,
		connType: connType,
		addr:     addr,
		ovs:      ovs,
	}

	return address, nil
}

func (sa *multiSocketAddress) String() string {
	return sa.raw
}

func (sa *multiSocketAddress) ConnectType() string {
	return sa.connType
}

func (sa *multiSocketAddress) Address() string {
	return sa.addr
}

func (sa *multiSocketAddress) OptionValues() options.OptionValues {
	return sa.ovs
}

func (sa *multiSocketAddress) Connect(ctr DialListener, ovses ...options.OptionValues) error {
	switch sa.connType {
	case ConnDial:
		return sa.Dial(ctr, ovses...)
	case ConnListen:
		return sa.Listen(ctr, ovses...)
	default:
		return ErrConnectTypeMissing
	}
}

func (sa *multiSocketAddress) Dial(ctr DialListener, ovses ...options.OptionValues) error {
	xovs := options.OptionValues{}
	for o, v := range sa.ovs {
		xovs[o] = v
	}
	for _, ovs := range ovses {
		for o, v := range ovs {
			xovs[o] = v
		}
	}

	return ctr.DialOptions(sa.addr, xovs)
}

func (sa *multiSocketAddress) Listen(ctr DialListener, ovses ...options.OptionValues) error {
	xovs := options.OptionValues{}
	for o, v := range sa.ovs {
		xovs[o] = v
	}
	for _, ovs := range ovses {
		for o, v := range ovs {
			xovs[o] = v
		}
	}

	return ctr.ListenOptions(sa.addr, xovs)
}

// Connect parse s to MultiSocketAddress and Connect with option values.
func Connect(ctr DialListener, s string, ovses ...options.OptionValues) (err error) {
	var (
		sa MultiSocketAddress
	)
	if sa, err = ParseMultiSocketAddress(s); err != nil {
		return
	}

	return sa.Connect(ctr, ovses...)
}

// Dial parse s to MultiSocketAddress and Dial with option values.
func Dial(ctr DialListener, s string, ovses ...options.OptionValues) (err error) {
	var (
		sa MultiSocketAddress
	)
	if sa, err = ParseMultiSocketAddress(s); err != nil {
		return
	}

	return sa.Dial(ctr, ovses...)
}

// Listen parse s to MultiSocketAddress and Listen with option values.
func Listen(ctr DialListener, s string, ovses ...options.OptionValues) (err error) {
	var (
		sa MultiSocketAddress
	)
	if sa, err = ParseMultiSocketAddress(s); err != nil {
		return
	}

	return sa.Listen(ctr, ovses...)
}
