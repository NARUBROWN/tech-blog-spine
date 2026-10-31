package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
)

type AnalyticsService struct {
	repository *AnalyticsRepository
	config     config.Config
	tokens     *security.TokenManager
	refresh    *security.RedisRefreshTokenStore
}

func NewAnalyticsService(r *AnalyticsRepository, c config.Config, t *security.TokenManager, refresh *security.RedisRefreshTokenStore) *AnalyticsService {
	return &AnalyticsService{r, c, t, refresh}
}

type VisitRequest struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Path      string `json:"path"`
	Referrer  string `json:"referrer"`
	Device    string `json:"device"`
}
type VisitResponse struct {
	Tracked       bool  `json:"tracked"`
	InterestCount int64 `json:"interestCount"`
}
type EventRequest struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Label      string `json:"label"`
	URL        string `json:"url"`
	Text       string `json:"text"`
	Context    string `json:"context"`
	Section    string `json:"section"`
	DurationMs int64  `json:"durationMs"`
}
type BatchRequest struct {
	VisitID     string         `json:"visitId"`
	ActiveMs    int64          `json:"activeMs"`
	ScrollDepth int            `json:"scrollDepth"`
	Events      []EventRequest `json:"events"`
}

var uuidPattern = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var whitespace = regexp.MustCompile(`\s+`)

func bad(code int, text string) error { return &httperr.HTTPError{Status: code, Message: text} }
func short(text string, n int) string {
	r := []rune(strings.TrimSpace(text))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}
func (s *AnalyticsService) hash(text string) string {
	h := hmac.New(sha256.New, []byte(s.config.JWTSecret))
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

// 분석용 쿠키는 댓글 쿠키와 분리해 초기 요청 경합을 방지합니다.
func (s *AnalyticsService) visitor(h header.Values, create bool) (string, []httpx.Cookie, error) {
	raw := security.CookieFromHeader(h.Get("Cookie"), "analytics_visitor")
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || len(parts[0]) != 64 || !hmac.Equal([]byte(parts[1]), []byte(s.hash("analytics-cookie:"+parts[0]))) {
		if !create {
			return "", nil, bad(403, "방문 식별 쿠키가 필요합니다.")
		}
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", nil, err
		}
		id := hex.EncodeToString(b[:])
		raw = id + "." + s.hash("analytics-cookie:"+id)
		return s.hash("analytics-visitor:" + raw), []httpx.Cookie{{Name: "analytics_visitor", Value: raw, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: s.config.CookieSecure, SameSite: httpx.SameSiteLax}}, nil
	}
	return s.hash("analytics-visitor:" + raw), nil, nil
}
func (s *AnalyticsService) origin(h header.Values) error {
	if h.Get("Origin") == "" {
		return nil
	}
	for _, o := range s.config.AllowedOrigins {
		if o == h.Get("Origin") {
			return nil
		}
	}
	return bad(403, "허용되지 않은 출처입니다.")
}
func (s *AnalyticsService) rate(ctx context.Context, h header.Values, action string, max int) error {
	ip := h.Get("X-Feedback-IP")
	if net.ParseIP(ip) == nil {
		ip = "local"
	}
	key := s.hash("analytics-network:" + ip)
	window := time.Now().Unix() / 60
	tx, err := s.repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO feedback_rate(network_hash,action,window_start) VALUES (?,?,?) ON DUPLICATE KEY UPDATE requests=requests+1`, key, action, window); err != nil {
		return err
	}
	var n int
	if err = tx.QueryRowContext(ctx, `SELECT requests FROM feedback_rate WHERE network_hash=? AND action=? AND window_start=?`, key, action, window).Scan(&n); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if n > max {
		return bad(429, "계측 요청이 너무 많습니다.")
	}
	return nil
}
func canonicalPage(raw string) (string, string, error) {
	if len(raw) > 4096 {
		return "", "", bad(400, "페이지 경로가 너무 깁니다.")
	}
	path, err := url.PathUnescape(raw)
	if err != nil || strings.ContainsAny(path, "?#\x00") || utf8.RuneCountInString(path) > 512 {
		return "", "", bad(400, "올바르지 않은 페이지 경로입니다.")
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	switch path {
	case "/":
		return path, "home", nil
	case "/about":
		return path, "about", nil
	case "/search":
		return path, "search", nil
	}
	if strings.HasPrefix(path, "/post/") && !strings.HasPrefix(path, "/post/edit/") && path != "/post/create" {
		return path, "post", nil
	}
	return "", "", nil
}
func (s *AnalyticsService) Start(ctx context.Context, h header.Values, r VisitRequest) (VisitResponse, []httpx.Cookie, error) {
	var out VisitResponse
	if err := s.origin(h); err != nil {
		return out, nil, err
	}
	if !uuidPattern.MatchString(r.ID) || !uuidPattern.MatchString(r.SessionID) {
		return out, nil, bad(400, "방문 식별자가 올바르지 않습니다.")
	}
	path, kind, err := canonicalPage(r.Path)
	if err != nil || kind == "" {
		return out, nil, err
	}
	ua := strings.ToLower(h.Get("User-Agent"))
	for _, bot := range []string{"bot", "spider", "crawler", "headlesschrome"} {
		if strings.Contains(ua, bot) {
			return out, nil, nil
		}
	}
	// 관리자 자신의 탐색은 공개 관심 수와 방문 통계에 포함하지 않습니다.
	claims, tokenErr := s.tokens.Parse(security.CookieFromHeader(h.Get("Cookie"), "accessToken"), security.AccessToken)
	if tokenErr != nil && s.refresh != nil {
		raw := security.CookieFromHeader(h.Get("Cookie"), "refreshToken")
		if refreshed, e := s.tokens.Parse(raw, security.RefreshToken); e == nil {
			if stored, e := s.refresh.Get(ctx, refreshed.UserID); e == nil && hmac.Equal([]byte(stored), []byte(raw)) {
				claims = refreshed
				tokenErr = nil
			}
		}
	}
	if tokenErr == nil {
		var role string
		_ = s.repository.db.QueryRowContext(ctx, "SELECT role FROM `user` WHERE user_id=?", claims.UserID).Scan(&role)
		if role == "ROLE_ADMIN" {
			return out, nil, nil
		}
	}

	if err = s.rate(ctx, h, "analytics-visit", 120); err != nil {
		return out, nil, err
	}
	visitor, cookies, err := s.visitor(h, true)
	if err != nil {
		return out, nil, err
	}
	var postID any
	if kind == "post" {
		var id int64
		if strings.HasPrefix(path, "/post/share/") {
			id, err = strconv.ParseInt(strings.TrimPrefix(path, "/post/share/"), 10, 64)
			if err == nil {
				err = s.repository.db.QueryRowContext(ctx, `SELECT post_id FROM post WHERE post_id=?`, id).Scan(&id)
			}
		} else {
			err = s.repository.db.QueryRowContext(ctx, `SELECT post_id FROM post WHERE slug=? ORDER BY post_id LIMIT 1`, strings.TrimPrefix(path, "/post/")).Scan(&id)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return out, cookies, nil
		}
		if err != nil {
			return out, cookies, bad(400, "게시글 경로가 올바르지 않습니다.")
		}
		postID = id
	}
	ref := ""
	if u, e := url.Parse(r.Referrer); e == nil {
		ref = short(strings.ToLower(u.Hostname()), 255)
	}
	device := r.Device
	if device != "mobile" && device != "tablet" && device != "desktop" {
		device = "desktop"
	}
	tx, err := s.repository.db.BeginTx(ctx, nil)
	if err != nil {
		return out, cookies, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT IGNORE INTO analytics_visit(id,visitor_hash,session_hash,page_path,page_kind,post_id,referrer_host,device) VALUES (?,?,?,?,?,?,?,?)`, r.ID, visitor, s.hash("analytics-session:"+visitor+":"+r.SessionID), path, kind, postID, ref, device)
	if err != nil {
		return out, cookies, err
	}
	var owner, storedPath string
	if err = tx.QueryRowContext(ctx, `SELECT visitor_hash,page_path FROM analytics_visit WHERE id=?`, r.ID).Scan(&owner, &storedPath); err != nil {
		return out, cookies, err
	}
	if owner != visitor || storedPath != path {
		return out, cookies, bad(409, "방문 식별자가 충돌했습니다.")
	}
	if kind == "about" {
		if _, err = tx.ExecContext(ctx, `INSERT IGNORE INTO analytics_interest(visitor_hash) VALUES (?)`, visitor); err != nil {
			return out, cookies, err
		}
	}
	if err = tx.Commit(); err != nil {
		return out, cookies, err
	}
	out.Tracked = true
	out.InterestCount, err = s.Interest(ctx)
	// 상세 데이터는 90일 동안 보관하고 공개 누적 관심 수는 별도 집계로 유지합니다.
	_, _ = s.repository.db.ExecContext(ctx, `DELETE FROM analytics_visit WHERE created_at<DATE_SUB(NOW(),INTERVAL 90 DAY) LIMIT 100`)
	return out, cookies, err
}
func (s *AnalyticsService) Interest(ctx context.Context) (int64, error) {
	var n int64
	err := s.repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM analytics_interest`).Scan(&n)
	return n, err
}
func safeTarget(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme == "mailto" {
		return "mailto:"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return short(u.String(), 512)
}
func validType(kind string) bool {
	return kind == "snackbar_resume" || kind == "snackbar_linkedin" || kind == "resume_selection" || kind == "resume_outbound"
}
func (s *AnalyticsService) Collect(ctx context.Context, h header.Values, r BatchRequest) error {
	if err := s.origin(h); err != nil {
		return err
	}
	if !uuidPattern.MatchString(r.VisitID) || len(r.Events) > 20 || r.ActiveMs < 0 {
		return bad(400, "올바르지 않은 계측 요청입니다.")
	}
	visitor, _, err := s.visitor(h, false)
	if err != nil {
		return err
	}
	if err = s.rate(ctx, h, "analytics-events", 1200); err != nil {
		return err
	}
	tx, err := s.repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var path, owner string
	var elapsed int64
	err = tx.QueryRowContext(ctx, `SELECT page_path,visitor_hash,GREATEST(0,TIMESTAMPDIFF(MICROSECOND,created_at,NOW(6)) DIV 1000) FROM analytics_visit WHERE id=? FOR UPDATE`, r.VisitID).Scan(&path, &owner, &elapsed)
	if errors.Is(err, sql.ErrNoRows) {
		return bad(404, "방문 기록을 찾을 수 없습니다.")
	}
	if err != nil {
		return err
	}
	if owner != visitor {
		return bad(404, "방문 기록을 찾을 수 없습니다.")
	}
	active := min(r.ActiveMs, elapsed+1000, int64(8*60*60*1000))
	depth := max(0, min(100, r.ScrollDepth))
	if _, err = tx.ExecContext(ctx, `UPDATE analytics_visit SET active_ms=GREATEST(active_ms,?),scroll_depth=GREATEST(scroll_depth,?) WHERE id=?`, active, depth, r.VisitID); err != nil {
		return err
	}
	for _, e := range r.Events {
		if !uuidPattern.MatchString(e.ID) || !validType(e.Type) {
			return bad(400, "지원하지 않는 이벤트입니다.")
		}
		if strings.HasPrefix(e.Type, "resume_") && path != "/about" {
			return bad(400, "이력서 이벤트는 자기소개서에서만 수집합니다.")
		}
		text, contextText := "", ""
		duration := int64(0)
		if e.Type == "resume_selection" {
			text = short(whitespace.ReplaceAllString(e.Text, " "), 1000)
			contextText = short(whitespace.ReplaceAllString(e.Context, " "), 1500)
			duration = min(e.DurationMs, 60000)
			if utf8.RuneCountInString(text) < 8 || duration < 500 {
				return bad(400, "짧은 선택은 기록하지 않습니다.")
			}
		}
		target := safeTarget(e.URL)
		if e.Type == "resume_outbound" && target == "" {
			return bad(400, "외부 링크가 올바르지 않습니다.")
		}
		if e.Type == "snackbar_resume" {
			target = "/about"
		}
		if e.Type == "snackbar_linkedin" {
			target = "https://www.linkedin.com/in/naru-brown/"
		}
		// 이벤트 ID를 전역으로 고유하게 유지해 heartbeat와 종료 시 재전송도 중복 집계하지 않습니다.
		if _, err = tx.ExecContext(ctx, `INSERT IGNORE INTO analytics_event(id,visit_id,event_type,label,target_url,selected_text,context_text,section_name,duration_ms) VALUES (?,?,?,?,?,?,?,?,?)`, e.ID, r.VisitID, e.Type, short(e.Label, 120), target, text, contextText, short(e.Section, 80), duration); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *AnalyticsService) Admin(ctx context.Context, id security.Identity) error {
	if id.UserID <= 0 {
		return bad(401, "로그인이 필요합니다.")
	}
	var role string
	if err := s.repository.db.QueryRowContext(ctx, "SELECT role FROM `user` WHERE user_id=?", id.UserID).Scan(&role); err != nil {
		return bad(403, "관리자만 접근할 수 있습니다.")
	}
	if role != "ROLE_ADMIN" {
		return bad(403, "관리자만 접근할 수 있습니다.")
	}
	return nil
}
