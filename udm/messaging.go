package udm

import (
	"fmt"
	"log/slog"
	"net"
	"net/rpc"
)

type Rpc struct {
	app *NativeMsgReceiver
}

type RpcReq struct {
	Msg string
}

type RpcNewDownload struct {
	Uri string
}

type RpcRes struct {
	Msg string
}

func (r *Rpc) Ping(req *RpcReq, res *RpcRes) error {
	rv := "bad request"
	if req.Msg == "ping" {
		rv = "pong"
	}
	res.Msg = rv
	return nil
}

func (r *Rpc) AddDownload(req *RpcNewDownload, res *bool) error {
	j := Job{
		Uri: req.Uri,
	}
	_, _ = r.app.man.AddDownload(j)
	return nil
}

type NativeMsgReceiver struct {
	rpcSvr    *rpc.Server
	man       Manager
	listener  net.Listener
	listening bool
	cfg       *Config
	Port      int
	clients   []string
}

func (r *NativeMsgReceiver) Start() error {
	var err error
	r.Port = r.cfg.NativeRcvrPort

	addr := fmt.Sprintf("127.0.0.1:%d", r.Port)

	slog.Info("Starting NativeMsgRcvr", "addr", addr)

	r.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	r.rpcSvr = rpc.NewServer()

	err = r.rpcSvr.Register(&Rpc{app: r})
	if err != nil {
		return err
	}

	go r.rpcSvr.Accept(r.listener)

	r.listening = true

	return nil
}

func (r *NativeMsgReceiver) Stop() error {
	return nil
}

func NewNativeMsgReceiver(cfg *Config, man Manager) *NativeMsgReceiver {
	return &NativeMsgReceiver{
		man: man,
		cfg: cfg,
	}
}
