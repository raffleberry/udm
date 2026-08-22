package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/raffleberry/udm/udm"
)

func main() {
	cfg := udm.NewConfig()
	man := udm.NewManager(cfg)

	err := man.Start(context.Background())
	if err != nil {
		panic(err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

}
