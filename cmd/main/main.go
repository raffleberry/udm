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

	var l *slog.Logger

	if udm.IsGoRun() {
		l = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		}))
	} else {
		logfile, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			panic(err)
		}
		defer logfile.Close()
		l = slog.New(slog.NewTextHandler(logfile, &slog.HandlerOptions{
			Level:     slog.LevelDebug,
			AddSource: true,
		}))
	}

	slog.SetDefault(l)

	str, err := udm.NewSqliteStore(cfg)
	if err != nil {
		slog.Error("Failed to init store", "err", err)
		panic(err)
	}

	man := udm.NewA2(cfg, str)

	extSrv := udm.NewNativeMsgReceiver(cfg, man)

	err = extSrv.Start()
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
