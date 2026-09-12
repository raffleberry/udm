package host

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

var MsgAction = struct {
	Ping     string
	Download string
}{
	Ping:     "ping",
	Download: "download",
}

type Msg struct {
	Action string `json:"action"`
	Data   string `json:"data"`
	Ts     int64  `json:"ts"`
	From   string `json:"from"`
}

func logf(format string, args ...any) {
	f, err := os.OpenFile(LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}

func readMessage() (Msg, error) {
	var msg Msg
	var n uint32
	if err := binary.Read(os.Stdin, binary.LittleEndian, &n); err != nil {
		return msg, err
	}
	logf("got length = %d", n)

	buf := make([]byte, n)
	if _, err := io.ReadFull(os.Stdin, buf); err != nil {
		return msg, err
	}
	logf("got body: %s", string(buf))

	if err := json.Unmarshal(buf, &msg); err != nil {
		return msg, err
	}
	return msg, nil
}

func writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	logf("sending reply (%d bytes)", len(data))

	if err := binary.Write(os.Stdout, binary.LittleEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err // ← no Sync()
}

func process(m Msg) error {
	logf("processing: %s, %s", m.Action, m.Data)

	switch m.Action {
	case MsgAction.Ping:
		resp := map[string]any{
			"action":   "pong",
			"received": m.Action,
			"time":     time.Now().UnixMilli(),
		}

		if err := writeMessage(resp); err != nil {
			logf("write error → exiting: %v", err)
			return err
		}
	case MsgAction.Download:
		url := m.Data
		logf("Got url : %s", url)
	}

	return nil
}

func Run() {

	logf("=== host started (pid %d) ===", os.Getpid())

	for {
		msg, err := readMessage()
		if err != nil {
			logf("read error → exiting: %v", err)
			return
		}

		err = process(msg)
		if err != nil {
			logf("Error Processing: %v", err)
			continue
		}

		logf("success processing, waiting for next message…")
	}
}
