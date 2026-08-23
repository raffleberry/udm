package udm_test

import (
	"fmt"
	"log"
	"net/rpc"
	"os"
	"testing"

	"github.com/raffleberry/udm/udm"
)

var udmRpc *udm.NativeMsgReceiver

func setupRpc() {
	cfg := udm.NewConfig()
	cfg.MsgPortStart = 51010
	cfg.MsgPortEnd = 51020
	udmRpc = udm.NewNativeMsgReceiver(cfg)
}

func TestMain(m *testing.M) {

	// setup
	setupRpc()
	err := udmRpc.Start()
	if err != nil {
		log.Fatalf("Failed to start rpc, err: %v", err)
	}
	defer udmRpc.Stop()

	exitCode := m.Run()

	// teardown

	os.Exit(exitCode)
}

func TestRpc_Ping(t *testing.T) {
	adr := fmt.Sprintf("127.0.0.1:%d", udmRpc.Port)
	c, err := rpc.Dial("tcp", adr)
	req := udm.RpcReq{Msg: "ping"}
	res := udm.RpcRes{}
	err = c.Call("Rpc.Ping", &req, &res)
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg != "pong" {
		t.Fatalf("Expected pong, got %s", res.Msg)
	}
	req.Msg = "pong"
	res.Msg = ""
	err = c.Call("Rpc.Ping", &req, &res)
	if err != nil {
		t.Fatal(err)
	}
	if res.Msg != "bad request" {
		t.Fatalf("Expected empty, got %s", res.Msg)
	}
}
