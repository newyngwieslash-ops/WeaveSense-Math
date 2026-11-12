package edumath

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAssetOriginFromEnvRejectsPathOrQuery(t *testing.T) {
	t.Setenv("EDU_ASSET_DOMAIN", "https://assets.example.edu")
	if got := AssetOriginFromEnv(); got != "https://assets.example.edu" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("EDU_ASSET_DOMAIN", "https://assets.example.edu/secret")
	if AssetOriginFromEnv() != "" {
		t.Fatal("path must be rejected")
	}
	t.Setenv("EDU_ASSET_DOMAIN", "")
	if AssetOriginFromEnv() != "" {
		t.Fatal("empty")
	}
}

func TestSignedURLUsesAssetOrigin(t *testing.T) {
	s := &Service{URLKey: []byte(strings.Repeat("u", 32)), AssetOrigin: "https://assets.example.edu", Now: func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }}
	u, _ := s.SignedURL("ma_1", "u_alice", time.Minute)
	if !strings.HasPrefix(u, "https://assets.example.edu/api/edu/files/ma_1?") {
		t.Fatalf("url %s", u)
	}
	s.AssetOrigin = ""
	u, _ = s.SignedURL("ma_1", "u_alice", time.Minute)
	if !strings.HasPrefix(u, "/api/edu/files/ma_1?") {
		t.Fatalf("relative %s", u)
	}
}

func TestFileCSPAllowsAssetOrigin(t *testing.T) {
	t.Setenv("EDU_ASSET_DOMAIN", "")
	if !strings.Contains(fileCSP(), "img-src data:") || strings.Contains(fileCSP(), "https://") {
		t.Fatalf("%s", fileCSP())
	}
	t.Setenv("EDU_ASSET_DOMAIN", "https://assets.example.edu")
	csp := fileCSP()
	if !strings.Contains(csp, "img-src data: https://assets.example.edu") || !strings.Contains(csp, "media-src data: https://assets.example.edu") {
		t.Fatalf("%s", csp)
	}
}

func TestAssetHostRejectsSessionRoutesAndStripsCookies(t *testing.T) {
	t.Setenv("EDU_ASSET_DOMAIN", "https://assets.example.edu")
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if isAssetHost(c) {
			c.Header("Referrer-Policy", "no-referrer")
			if !strings.HasPrefix(c.Request.URL.Path, "/api/edu/files/") {
				c.Status(http.StatusNotFound)
				c.Abort()
				return
			}
			c.Next()
			c.Writer.Header().Del("Set-Cookie")
			return
		}
		c.Next()
	})
	r.GET("/api/edu/files/:id", func(c *gin.Context) {
		c.SetCookie("canvas_session", "should-not-leak", 60, "/", "", false, true)
		c.String(200, "ok")
	})
	r.GET("/api/edu/home", func(c *gin.Context) { c.String(200, "home") })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/edu/home", nil)
	req.Host = "assets.example.edu"
	req.Header.Set("Cookie", "canvas_session=abc")
	r.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("session route on asset host: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/edu/files/ma_1", nil)
	req.Host = "assets.example.edu"
	r.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("file on asset host: %d cookie=%q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
}
