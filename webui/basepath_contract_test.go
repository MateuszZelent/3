package webui

import "testing"

func TestRetargetBasePathUsesActualPort(t *testing.T) {
	tests := []struct {
		name, base string
		want       string
	}{
		{name: "proxy route", base: "/proxy/35368", want: "/proxy/35369"},
		{name: "trailing slash", base: "/proxy/35368/", want: "/proxy/35369"},
		{name: "unrelated path", base: "/mumax", want: "/mumax"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RetargetBasePath(tt.base, 35368, 35369); got != tt.want {
				t.Fatalf("base path = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRetargetBasePathLeavesSamePortUnchanged(t *testing.T) {
	if got := RetargetBasePath("/proxy/35368", 35368, 35368); got != "/proxy/35368" {
		t.Fatalf("base path = %q", got)
	}
}
