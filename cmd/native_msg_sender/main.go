package main

import (
	"fmt"
	"net"
	"net/rpc"

	"github.com/raffleberry/udm/udm"
)

func main() {
	fmt.Println("Start")
	startPort := 54000
	endPort := 55000
	var client *rpc.Client
	var conn net.Conn
	var err error
	for port := startPort; port <= endPort; port++ {
		adr := fmt.Sprintf("127.0.0.1:%d", port)

		conn, err = net.Dial("tcp", adr)
		if err != nil {
			fmt.Println("Port", port, "is Open")
			continue
		}

		client = rpc.NewClient(conn)
		if client != nil {
			break
		}
		fmt.Printf("Port: %d listening, but no RPC Client", port)
		conn.Close()
	}
	if client == nil {
		fmt.Println("No rpc client found")
		return
	}
	fmt.Println(client)
	req := udm.RpcReq{Msg: "ping"}
	resp := udm.RpcRes{}
	err = client.Call("Rpc.Ping", &req, &resp)

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(resp)
}
