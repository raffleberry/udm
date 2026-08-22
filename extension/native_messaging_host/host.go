package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

var (
	LogPath = "/tmp/udm-native-ping.log"
)

func logf(format string, args ...any) {
	f, err := os.OpenFile(LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", args...)
}

func readMessage() (any, error) {
	var n uint32
	if err := binary.Read(os.Stdin, binary.LittleEndian, &n); err != nil {
		return nil, err
	}
	logf("got length = %d", n)

	buf := make([]byte, n)
	if _, err := io.ReadFull(os.Stdin, buf); err != nil {
		return nil, err
	}
	logf("got body: %s", string(buf))

	var msg any
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, err
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

func main() {
	logf("=== host started (pid %d) ===", os.Getpid())

	for {
		msg, err := readMessage()
		if err != nil {
			logf("read error → exiting: %v", err)
			return
		}

		resp := map[string]any{
			"action":   "pong",
			"received": msg,
			"time":     time.Now().UTC().Format(time.RFC3339Nano),
		}

		if err := writeMessage(resp); err != nil {
			logf("write error → exiting: %v", err)
			return
		}
		logf("reply sent, waiting for next message…")
	}
}
