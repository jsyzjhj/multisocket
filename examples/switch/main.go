package main

import (
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/webee/multisocket"
	"github.com/webee/multisocket/examples"
	_ "github.com/webee/multisocket/transport/all"
	"github.com/webee/multisocket/utils/connutils"
)

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})
}

func main() {
	backAddr := os.Args[1]
	frontAddr := os.Args[2]

	sockBack := multisocket.NewDefault()
	if err := connutils.ParseSmartAddress(backAddr).Connect(sockBack, nil); err != nil {
		log.WithField("err", err).WithField("socket", "back").Panicf("connect")
	}

	sockFront := multisocket.NewDefault()
	if err := connutils.ParseSmartAddress(frontAddr).Connect(sockFront, nil); err != nil {
		log.WithField("err", err).WithField("socket", "front").Panicf("connect")
	}

	go forward(sockFront, sockBack)
	go forward(sockBack, sockFront)
	examples.SetupSignal()
}

func forward(from multisocket.Socket, to multisocket.Socket) {
	for {
		msg, err := from.RecvMsg()
		if err != nil {
			log.WithField("err", err).Errorf("recv")
		}

		if err := to.SendMsg(msg); err != nil {
			log.WithField("err", err).Errorf("forward")
		}
	}
}
