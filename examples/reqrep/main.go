package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/webee/multisocket/protocol/reqrep"
	_ "github.com/webee/multisocket/transport/all"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
}

func main() {
	if len(os.Args) > 3 && os.Args[1] == "rep" {
		n, _ := strconv.Atoi(os.Args[4])
		server(os.Args[2], os.Args[3], n)
		os.Exit(0)
	}
	if len(os.Args) > 4 && os.Args[1] == "req" {
		client(os.Args[2], os.Args[3], os.Args[4])
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr,
		"Usage: reqrep req|rep <URL> <ARG> ...\n")
	os.Exit(1)
}

func server(t, addr string, n int) {
	rep := reqrep.NewRep(reqHandler(n))
	rep.Start()
	switch t {
	case "dial":
		if err := rep.Dial(addr); err != nil {
			log.WithField("err", err).Panicf("dial")
		}
	default:
		if err := rep.Listen(addr); err != nil {
			log.WithField("err", err).Panicf("listen")
		}
	}

	setupSignal()
}

func client(t, addr string, name string) {
	req := reqrep.NewReq()
	switch t {
	case "listen":
		if err := req.Listen(addr); err != nil {
			log.WithField("err", err).Panicf("listen")
		}
	default:
		if err := req.Dial(addr); err != nil {
			log.WithField("err", err).Panicf("dial")
		}
	}

	idx := 0
	for {
		log.WithField("id", idx).Infof("request")
		if reply, err := req.Request([]byte(fmt.Sprintf("%s#%d", name, idx))); err != nil {
			log.WithError(err).WithField("id", idx).Errorf("request")
			time.Sleep(1 * time.Second)
		} else {
			fmt.Printf("%s\n", string(reply))
		}
		time.Sleep(100 * time.Millisecond)
		idx++
	}
}

type reqHandler int

func (h reqHandler) Handle(req []byte) (rep []byte) {
	rep = []byte(fmt.Sprintf("[#%d]Hello, %s", int(h), string(req)))
	time.Sleep(time.Millisecond * 10)
	return
}

// setupSignal register signals handler and waiting for.
func setupSignal() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	for {
		s := <-c
		log.WithField("signal", s.String()).Info("signal")
		switch s {
		case os.Interrupt, syscall.SIGTERM:
			return
		default:
			return
		}
	}
}
