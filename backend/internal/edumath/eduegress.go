package edumath

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ErrEduEgressDenied is returned when a vendor destination is not on the
// configured allowlist (DEV-105, E96). The default policy is fail-closed:
// an empty or unset list denies every host.
var ErrEduEgressDenied = errors.New("edumath: egress destination is not on the allowlist")

const (
	envEgressAllowlist = "EDU_EGRESS_ALLOWLIST"
	envDataRegion      = "EDU_DATA_REGION"
)

// EgressRule is one allowed vendor endpoint. Port 0 means any port.
type EgressRule struct {
	Host   string `json:"host"`
	Port   int    `json:"port,omitempty"`
	Region string `json:"region,omitempty"`
}

// EgressPolicy is the configured region plus allowlist. Formal production
// hosts and regions still come from the deployer (U8); this is the local
// stand-in that default-denies anything unlisted.
type EgressPolicy struct {
	Region string
	Rules  []EgressRule
	Source string // env | empty | invalid
}

// LoadEgressPolicy reads EDU_EGRESS_ALLOWLIST (JSON array of {host,port,region}
// or "host[:port]" strings) and EDU_DATA_REGION. Invalid JSON fails closed.
func LoadEgressPolicy() EgressPolicy {
	region := strings.TrimSpace(os.Getenv(envDataRegion))
	raw := strings.TrimSpace(os.Getenv(envEgressAllowlist))
	if raw == "" {
		return EgressPolicy{Region: region, Source: "empty"}
	}
	rules, err := parseAllowlist(raw)
	if err != nil {
		return EgressPolicy{Region: region, Source: "invalid"}
	}
	return EgressPolicy{Region: region, Rules: rules, Source: "env"}
}

func parseAllowlist(raw string) ([]EgressRule, error) {
	var asRules []EgressRule
	if err := json.Unmarshal([]byte(raw), &asRules); err != nil {
		return nil, err
	}
	return normalizeRules(asRules)
}

func normalizeRules(in []EgressRule) ([]EgressRule, error) {
	out := make([]EgressRule, 0, len(in))
	for _, r := range in {
		host := canonicalHost(r.Host)
		if host == "" || r.Port < 0 || r.Port > 65535 {
			return nil, fmt.Errorf("invalid egress rule")
		}
		out = append(out, EgressRule{Host: host, Port: r.Port, Region: strings.TrimSpace(r.Region)})
	}
	return out, nil
}

func canonicalHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	if h == "" || strings.ContainsAny(h, "/@ ") || strings.Contains(h, "://") {
		return ""
	}
	return h
}

// Allow reports whether a vendor connection to host:port is permitted.
// Unlisted destinations are denied, including when the list is empty.
func (p EgressPolicy) Allow(host string, port int) error {
	host = canonicalHost(host)
	if host == "" || p.Source == "invalid" {
		return fmt.Errorf("%w: %s:%d", ErrEduEgressDenied, host, port)
	}
	for _, r := range p.Rules {
		if r.Host != host {
			continue
		}
		if r.Port == 0 || r.Port == port {
			return nil
		}
	}
	return fmt.Errorf("%w: %s:%d", ErrEduEgressDenied, host, port)
}

// AllowURL parses a vendor URL and applies Allow. https defaults to port 443,
// http to 80.
func (p EgressPolicy) AllowURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: invalid url", ErrEduEgressDenied)
	}
	host := u.Hostname()
	port := 0
	if u.Port() != "" {
		port, err = strconv.Atoi(u.Port())
		if err != nil {
			return fmt.Errorf("%w: invalid port", ErrEduEgressDenied)
		}
	} else if u.Scheme == "https" {
		port = 443
	} else if u.Scheme == "http" {
		port = 80
	}
	return p.Allow(host, port)
}

// Overview is the institution-admin view of the policy. It never includes
// hostnames (those may be deployer-private).
func (p EgressPolicy) Overview() gin.H {
	out := gin.H{"allowlistCount": len(p.Rules), "source": p.Source, "standIn": true}
	if p.Region != "" {
		out["region"] = p.Region
	}
	switch p.Source {
	case "env":
		out["state"] = "configured"
	case "invalid":
		out["state"] = "invalid"
	default:
		out["state"] = "not_configured"
		if len(p.Rules) == 0 {
			out["blockedBy"] = "DEV-105"
		}
	}
	return out
}
