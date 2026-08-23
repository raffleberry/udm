package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/raffleberry/udm/udm"
)

func main() {
	cfg := udm.NewConfig()
	man := udm.NewManager(cfg)

	extSrv := udm.NewNativeMsgReceiver(cfg)

	err := extSrv.Start()
	if err != nil {
		slog.Error("Failed to Start extension server", "Err", err)
	} else {
		defer func() {
			err := extSrv.Stop()
			if err != nil {
				slog.Error("Failed to Stop extension server")
			}
		}()
	}

	err = man.Start(context.Background())
	if err != nil {
		panic(err)
	}

	defer man.Shutdown(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

}
