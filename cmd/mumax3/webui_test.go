package main

import "testing"

func TestParseWebUIAddress(t *testing.T) {
	host, port, basePath, err := parseWebUIAddress(":35367/proxy/job")
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || port != 35367 || basePath != "/proxy/job" {
		t.Fatalf("parsed as host=%q port=%d path=%q", host, port, basePath)
	}
}
