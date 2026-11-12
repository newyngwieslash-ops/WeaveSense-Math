package edumath

import (
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// AssetOriginFromEnv reads EDU_ASSET_DOMAIN (scheme + host, no path), e.g.
// https://assets.example.edu. Empty means signed URLs stay on the API host.
func AssetOriginFromEnv() string {
	raw := strings.TrimSpace(os.Getenv("EDU_ASSET_DOMAIN"))
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return ""
	}
	return strings.TrimRight(u.String(), "/")
}

// AssetDomainHost is the host portion of EDU_ASSET_DOMAIN (lowercase, no port
// if it is the scheme default). Empty when the independent domain is unset.
func AssetDomainHost() string {
	return hostOf(AssetOriginFromEnv())
}

func hostOf(origin string) string {
	if origin == "" {
		return ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Host)
}

func requestHost(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if h := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); h != "" {
		return strings.ToLower(strings.Split(h, ",")[0])
	}
	return strings.ToLower(c.Request.Host)
}

// isAssetHost is true when this request arrived on the independent asset
// domain. That host must not use the application session: only signed file
// paths are served, and Set-Cookie is stripped.
func isAssetHost(c *gin.Context) bool {
	want := AssetDomainHost()
	return want != "" && requestHost(c) == want
}

func fileCSP() string {
	img := "data:"
	if o := AssetOriginFromEnv(); o != "" {
		img = "data: " + o
	}
	return "sandbox allow-same-origin; default-src 'none'; img-src " + img + "; media-src " + img + "; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"
}

func imageCSP() string {
	return "default-src 'none'; sandbox"
}
