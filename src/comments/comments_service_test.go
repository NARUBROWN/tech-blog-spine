package comments

import (
	"net/http"
	"strings"
	"testing"

	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

func TestVisitorSignatureAndOrigin(t *testing.T) {
	c := config.Config{JWTSecret: strings.Repeat("secret", 8), CookieSecure: true, AllowedOrigins: []string{"https://www.na2ru2.me"}}
	s := NewCommentsService(nil, c, security.NewTokenManager(c))
	first, cookies, err := s.Visitor(header.NewValues(http.Header{}), false)
	if err != nil || len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatalf("cookie issuance: %v", err)
	}
	h := http.Header{"Cookie": []string{"feedback_visitor=" + cookies[0].Value}, "Origin": []string{"https://www.na2ru2.me"}, "X-Feedback-Ip": []string{"192.0.2.1"}}
	second, _, err := s.Visitor(header.NewValues(h), true)
	if err != nil || first.Key != second.Key || second.Network == "192.0.2.1" {
		t.Fatalf("identity: %v", err)
	}
	h.Set("Cookie", "feedback_visitor="+cookies[0].Value+"tampered")
	if _, _, err = s.Visitor(header.NewValues(h), true); err == nil {
		t.Fatal("forged cookie accepted")
	}
	h.Set("Cookie", "feedback_visitor="+cookies[0].Value)
	h.Set("Origin", "https://evil.example")
	if _, _, err = s.Visitor(header.NewValues(h), true); err == nil {
		t.Fatal("cross-origin write accepted")
	}
}
func TestCommentLimits(t *testing.T) {
	for _, r := range []CommentRequest{
		{Nickname: " ", Content: "ok", Password: "password"},
		{Nickname: "익명", Content: " ", Password: "password"},
		{Nickname: "익명", Content: strings.Repeat("가", 2001), Password: "password"},
		{Nickname: "익명", Content: "ok", Password: strings.Repeat("p", 73)},
	} {
		if checkComment(r, true) == nil {
			t.Fatal("invalid comment accepted")
		}
	}
	if err := checkComment(CommentRequest{Nickname: "익명", Content: "<script>는 일반 텍스트입니다", Password: "1234"}, true); err != nil {
		t.Fatal(err)
	}
}
