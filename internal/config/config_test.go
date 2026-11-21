package config

import (
	"errors"
	"flag"
	"io"
	"testing"
)

func env(m map[string]string) Getenv { return func(k string) string { return m[k] } }

func TestParseHubDefaultsAndOverrides(t *testing.T) {
	cfg, err := ParseHub(nil, io.Discard, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":8080" || cfg.DataDir != "/data" || cfg.TrustProxy {
		t.Fatalf("默认值不对: %+v", cfg)
	}

	cfg, err = ParseHub([]string{"--addr", ":9000"}, io.Discard, env(map[string]string{
		"TARSHIP_ADDR": ":7000", "TARSHIP_DATA": "/tmp/x", "TARSHIP_TRUST_PROXY": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.DataDir != "/tmp/x" || !cfg.TrustProxy {
		t.Fatalf("命令行应覆盖环境变量，环境变量应覆盖默认值: %+v", cfg)
	}
}

func TestParseHubBadBool(t *testing.T) {
	if _, err := ParseHub(nil, io.Discard, env(map[string]string{"TARSHIP_TRUST_PROXY": "maybe"})); err == nil {
		t.Fatal("非法布尔值应报错")
	}
}

func TestParseAgent(t *testing.T) {
	if _, err := ParseAgent(nil, io.Discard, env(nil)); err == nil {
		t.Fatal("缺少 server 应报错")
	}
	if _, err := ParseAgent([]string{"--server", "wss://h"}, io.Discard, env(nil)); err == nil {
		t.Fatal("缺少 token 应报错")
	}

	cfg, err := ParseAgent(nil, io.Discard, env(map[string]string{"TARSHIP_SERVER": "wss://h", "TARSHIP_TOKEN": "tsa_x"}))
	if err != nil || cfg.TokenFromFlag || cfg.Token != "tsa_x" {
		t.Fatalf("环境变量解析不对: %+v, %v", cfg, err)
	}

	cfg, err = ParseAgent([]string{"--server", "wss://h", "--token", "tsa_y", "--allow-insecure-ws"}, io.Discard, env(nil))
	if err != nil || !cfg.TokenFromFlag || !cfg.AllowInsecureWS {
		t.Fatalf("命令行解析不对: %+v, %v", cfg, err)
	}
}

func TestParseHelp(t *testing.T) {
	_, err := ParseHub([]string{"-h"}, io.Discard, env(nil))
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("-h 应返回 flag.ErrHelp, got %v", err)
	}
}

func TestHealthURL(t *testing.T) {
	cases := map[string]string{
		":8080":         "http://127.0.0.1:8080/healthz",
		"0.0.0.0:9000":  "http://127.0.0.1:9000/healthz",
		"10.0.0.5:8080": "http://10.0.0.5:8080/healthz",
		"[::]:8080":     "http://127.0.0.1:8080/healthz",
		"garbage":       "http://127.0.0.1:8080/healthz",
	}
	for addr, want := range cases {
		if got := HealthURL(addr); got != want {
			t.Errorf("HealthURL(%q) = %q; want %q", addr, got, want)
		}
	}
}
