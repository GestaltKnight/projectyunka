package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseVLESS(t *testing.T) {
	nodes, err := parseSubscriptionText("vless://11111111-1111-1111-1111-111111111111@example.com:443?security=tls&sni=example.com&type=ws&path=%2Fws#Example", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := nodes[0]; got.Type != "vless" || got.UUID == "" || got.Server != "example.com" || !got.TLS {
		t.Fatalf("unexpected node: %#v", got)
	}
}

func TestParseTrojan(t *testing.T) {
	nodes, err := parseSubscriptionText("trojan://secret@example.com:443?sni=example.com#Trojan", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := nodes[0]; got.Type != "trojan" || got.Password != "secret" || got.Tag != "Trojan" {
		t.Fatalf("unexpected node: %#v", got)
	}
}

func TestParseShadowsocksBase64UserInfo(t *testing.T) {
	nodes, err := parseSubscriptionText("ss://YWVzLTEyOC1nY206cGFzc3dvcmQ@example.com:8388#SSNode", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := nodes[0]; got.Type != "shadowsocks" || got.Method != "aes-128-gcm" || got.Password != "password" {
		t.Fatalf("unexpected node: %#v", got)
	}
}

func TestRejectEncryptedHappLinks(t *testing.T) {
	_, err := parseHappLink("happ://crypt5/example", Options{})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported crypt error, got %v", err)
	}
}

func TestBuildConfigIsJSON(t *testing.T) {
	nodes, err := parseSubscriptionText("trojan://secret@example.com:443#Trojan", Options{})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(buildSingBoxConfig(nodes, Options{ProxyMode: "global", DNSMode: "remote"}))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSON(data); err != nil {
		t.Fatal(err)
	}
}
