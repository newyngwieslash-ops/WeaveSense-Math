package edumath

import (
	"errors"
	"testing"
)

// TestEduEgressPolicy (DEV-105, E96): the allowlist is configurable and
// default-denies unlisted destinations. Formal production hosts stay with U8.
func TestEduEgressPolicy(t *testing.T) {
	empty := EgressPolicy{Source: "empty"}
	if err := empty.Allow("tts.example.invalid", 443); !errors.Is(err, ErrEduEgressDenied) {
		t.Fatalf("empty policy must deny: %v", err)
	}
	if st := empty.Overview()["state"]; st != "not_configured" {
		t.Fatalf("empty overview: %v", empty.Overview())
	}
	if empty.Overview()["blockedBy"] != "DEV-105" {
		t.Fatalf("empty overview must still point at DEV-105: %v", empty.Overview())
	}

	p := EgressPolicy{Region: "cn-north-standin", Source: "env", Rules: []EgressRule{
		{Host: "tts.example.invalid", Port: 443, Region: "cn-north-standin"},
		{Host: "ocr.example.invalid", Port: 0},
	}}
	if err := p.Allow("tts.example.invalid", 443); err != nil {
		t.Fatalf("listed host: %v", err)
	}
	if err := p.Allow("TTS.EXAMPLE.INVALID", 443); err != nil {
		t.Fatalf("host match is case-insensitive: %v", err)
	}
	if err := p.Allow("tts.example.invalid", 8443); !errors.Is(err, ErrEduEgressDenied) {
		t.Fatalf("wrong port must be denied: %v", err)
	}
	if err := p.Allow("ocr.example.invalid", 9); err != nil {
		t.Fatalf("port 0 allows any port: %v", err)
	}
	if err := p.Allow("other.example.invalid", 443); !errors.Is(err, ErrEduEgressDenied) {
		t.Fatalf("unlisted host must be denied: %v", err)
	}
	if err := p.AllowURL("https://tts.example.invalid/v1/speak"); err != nil {
		t.Fatalf("allow url: %v", err)
	}
	if err := p.AllowURL("https://tts.example.invalid:8443/v1"); !errors.Is(err, ErrEduEgressDenied) {
		t.Fatalf("url wrong port: %v", err)
	}
	ov := p.Overview()
	if ov["state"] != "configured" || ov["region"] != "cn-north-standin" || ov["allowlistCount"] != 2 || ov["standIn"] != true {
		t.Fatalf("configured overview: %v", ov)
	}
	if _, ok := ov["host"]; ok {
		t.Fatal("overview must not include hostnames")
	}

	t.Setenv(envEgressAllowlist, `[{"host":"tts.example.invalid","port":443,"region":"cn-north-standin"}]`)
	t.Setenv(envDataRegion, "cn-north-standin")
	got := LoadEgressPolicy()
	if got.Source != "env" || len(got.Rules) != 1 || got.Allow("tts.example.invalid", 443) != nil {
		t.Fatalf("load object list: %+v", got)
	}

	t.Setenv(envEgressAllowlist, `{not json`)
	got = LoadEgressPolicy()
	if got.Source != "invalid" || got.Overview()["state"] != "invalid" || !errors.Is(got.Allow("tts.example.invalid", 443), ErrEduEgressDenied) {
		t.Fatalf("invalid json must fail closed: %+v", got)
	}

	t.Setenv(envEgressAllowlist, "")
	t.Setenv(envDataRegion, "")
	got = LoadEgressPolicy()
	if got.Source != "empty" || got.Overview()["state"] != "not_configured" {
		t.Fatalf("unset: %+v", got)
	}
}
