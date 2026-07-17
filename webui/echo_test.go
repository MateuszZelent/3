package webui

import (
	"net"
	"testing"
)

func TestListenAvailableKeepsSelectedPortReserved(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	startPort := occupied.Addr().(*net.TCPAddr).Port
	listener, actualPort, err := listenAvailable("127.0.0.1", startPort)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	if actualPort == startPort {
		t.Fatalf("selected occupied port %d", startPort)
	}
	competitor, err := net.Listen("tcp", listener.Addr().String())
	if err == nil {
		competitor.Close()
		t.Fatalf("selected address %s was not reserved", listener.Addr())
	}
}
