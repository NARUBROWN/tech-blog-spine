package analytics

import (
	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
	"net/http"
	"strings"
	"testing"
)

func TestAnalyticsPageBoundary(t *testing.T) {
	for _, path := range []string{"/login", "/signup", "/settings", "/admin/analytics", "/post/create", "/post/edit/1"} {
		_, kind, err := canonicalPage(path)
		if err != nil || kind != "" {
			t.Fatalf("private page tracked: %s", path)
		}
	}
	for _, path := range []string{"/about?secret=value", "/about#contact", "/post/%00", "/post/%GG"} {
		if _, _, err := canonicalPage(path); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
	path, kind, err := canonicalPage("/about/")
	if err != nil || path != "/about" || kind != "about" {
		t.Fatal("about canonicalization failed")
	}
}
func TestAnalyticsTargetRedaction(t *testing.T) {
	if value := safeTarget("https://user:password@example.com/path?token=private#secret"); value != "https://example.com/path" {
		t.Fatalf("target leak: %s", value)
	}
	if safeTarget("javascript:alert(1)") != "" || safeTarget("mailto:private@example.com?body=text") != "mailto:" {
		t.Fatal("unsafe target")
	}
}
func TestAnalyticsCookieIsolation(t *testing.T) {
	c := config.Config{JWTSecret: strings.Repeat("s", 40), CookieSecure: true, AllowedOrigins: []string{"https://www.na2ru2.me"}}
	s := NewAnalyticsService(nil, c, security.NewTokenManager(c), nil)
	visitor, cookies, err := s.visitor(header.NewValues(http.Header{}), true)
	if err != nil || len(cookies) != 1 || cookies[0].Name != "analytics_visitor" || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatal("invalid analytics cookie")
	}
	h := http.Header{}
	h.Set("Cookie", "analytics_visitor="+cookies[0].Value)
	repeated, _, err := s.visitor(header.NewValues(h), false)
	if err != nil || visitor != repeated {
		t.Fatal("visitor changed")
	}
	h.Set("Cookie", "analytics_visitor="+cookies[0].Value+"forged")
	if _, _, err = s.visitor(header.NewValues(h), false); err == nil {
		t.Fatal("forged visitor accepted")
	}
	h.Set("Origin", "https://evil.example")
	if s.origin(header.NewValues(h)) == nil {
		t.Fatal("untrusted analytics origin accepted")
	}
}
