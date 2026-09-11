package main

import (
	"github.com/raffleberry/udm/extension/host"
)

func main() {
	err := host.Install()
	if err != nil {
		panic(err)
	}
}
