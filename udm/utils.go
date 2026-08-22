package udm

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
)

func scanFirstFreePort(startPort, endPort int) (int, error) {
	for port := startPort; port <= endPort; port++ {
		address := fmt.Sprintf("127.0.0.1:%d", port)

		ln, err := net.Listen("tcp", address)
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free ports found in range %d-%d", startPort, endPort)
}

func randomSecret() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
