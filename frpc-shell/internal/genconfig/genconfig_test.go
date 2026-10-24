package genconfig

import (
	"strings"
	"testing"
)

func TestTOMLContainsRequiredKeys(t *testing.T) {
	out := TOML("frps.example.com", 7000, "tok", "admin", "pwd123", 7400)
	for _, want := range []string{
		`serverAddr = "frps.example.com"`,
		"serverPort = 7000",
		`auth.token = "tok"`,
		"loginFailExit = false",
		"webServer.port = 7400",
		`webServer.password = "pwd123"`,
		`store.path = "./store/frpc_store.json"`,
		`log.to = "./logs/frpc.log"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("template missing %q", want)
		}
	}
}

func TestServerAddr(t *testing.T) {
	out := TOML("1.2.3.4", 7000, "", "admin", "p", 7400)
	if got := ServerAddr(out); got != "1.2.3.4" {
		t.Fatalf("serverAddr = %q", got)
	}
	if got := ServerAddr(TOML("", 7000, "", "admin", "p", 7400)); got != "" {
		t.Fatalf("empty serverAddr = %q", got)
	}
	// 用户手改后的紧凑写法
	if got := ServerAddr("serverAddr=\"nas.cn\"\n"); got != "nas.cn" {
		t.Fatalf("compact form = %q", got)
	}
}

func TestRandomHex(t *testing.T) {
	a, b := RandomHex(16), RandomHex(16)
	if len(a) != 32 || a == b {
		t.Fatalf("bad random: %q vs %q", a, b)
	}
}
