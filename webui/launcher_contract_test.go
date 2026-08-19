package webui

import "testing"

func TestListenAvailableRejectsInvalidConfiguration(t *testing.T) {
	if _, _, err := listenAvailable("127.0.0.1", 0); err == nil {
		t.Fatal("accepted port 0")
	}
	if _, _, err := listenAvailable("127.0.0.1", 65536); err == nil {
		t.Fatal("accepted port 65536")
	}
	if _, _, err := listenAvailable("host.invalid.invalid", 35367); err == nil {
		t.Fatal("accepted an invalid host")
	}
}

func TestParseHostAndPortSupportsIPv6AndDynamicPorts(t *testing.T) {
	tests := []struct {
		raw, wantHost string
		wantPort      uint16
	}{
		{raw: "proxy.example:35369", wantHost: "proxy.example", wantPort: 35369},
		{raw: "[::1]:35369", wantHost: "::1", wantPort: 35369},
		{raw: "proxy.example", wantHost: "proxy.example", wantPort: 0},
	}
	for _, tt := range tests {
		host, port, err := parseHostAndPort(tt.raw)
		if err != nil || host != tt.wantHost || port != tt.wantPort {
			t.Errorf("%q = %q:%d, %v", tt.raw, host, port, err)
		}
	}
	if _, _, err := parseHostAndPort("proxy.example:not-a-port"); err == nil {
		t.Fatal("accepted an invalid SSH port")
	}
}
