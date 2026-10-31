package comments

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/NARUBROWN/spine/pkg/header"
	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
	"golang.org/x/crypto/bcrypt"
)

type CommentsService struct {
	repository *CommentsRepository
	config     config.Config
	tokens     *security.TokenManager
}

func NewCommentsService(r *CommentsRepository, c config.Config, t *security.TokenManager) *CommentsService {
	return &CommentsService{r, c, t}
}

type Visitor struct {
	Key, Network string
	UserID       int64
}
type Comment struct {
	ID        int64  `json:"id"`
	Nickname  string `json:"nickname"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
type CommentRequest struct {
	Nickname string `json:"nickname"`
	Content  string `json:"content"`
	Password string `json:"password"`
}
type CommentPage struct {
	Content       []Comment `json:"content"`
	Number        int       `json:"number"`
	TotalPages    int       `json:"totalPages"`
	TotalElements int64     `json:"totalElements"`
}
type ReactionState struct {
	Count          int64                `json:"count"`
	Liked          bool                 `json:"liked"`
	AnonymousCount int64                `json:"anonymousCount"`
	Users          []model.UserResponse `json:"users"`
}

func fail(code int, msg string) error { return &httperr.HTTPError{Status: code, Message: msg} }
func (s *CommentsService) digest(value string) string {
	m := hmac.New(sha256.New, []byte(s.config.JWTSecret))
	m.Write([]byte(value))
	return hex.EncodeToString(m.Sum(nil))
}

// 식별자는 서명된 난수 쿠키로 발급하고 DB에는 HMAC만 보관합니다.
func (s *CommentsService) Visitor(h header.Values, write bool) (Visitor, []httpx.Cookie, error) {
	if write && h.Get("Origin") != "" {
		ok := false
		for _, o := range s.config.AllowedOrigins {
			if o == h.Get("Origin") {
				ok = true
			}
		}
		if !ok {
			return Visitor{}, nil, fail(403, "허용되지 않은 요청입니다.")
		}
	}
	raw := security.CookieFromHeader(h.Get("Cookie"), "feedback_visitor")
	parts := strings.Split(raw, ".")
	valid := len(parts) == 2 && len(parts[0]) == 64 && hmac.Equal([]byte(parts[1]), []byte(s.digest("cookie:"+parts[0])))
	var cookies []httpx.Cookie
	if !valid {
		if write {
			return Visitor{}, nil, fail(409, "브라우저 쿠키를 허용하고 페이지를 새로고침해 주세요.")
		}
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return Visitor{}, nil, err
		}
		id := hex.EncodeToString(b[:])
		raw = id + "." + s.digest("cookie:"+id)
		cookies = []httpx.Cookie{{Name: "feedback_visitor", Value: raw, Path: "/", MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, Secure: s.config.CookieSecure, SameSite: httpx.SameSiteLax}}
	}
	// 운영 Nginx가 덮어쓰는 연결 IP만 사용합니다. 전달 헤더의 임의 첫 값을 신뢰하지 않습니다.
	network := h.Get("X-Feedback-IP")
	if net.ParseIP(network) == nil {
		network = "local"
	}
	v := Visitor{Key: s.digest("visitor:" + raw), Network: s.digest("network:" + network)}
	if c, err := s.tokens.Parse(security.CookieFromHeader(h.Get("Cookie"), "accessToken"), security.AccessToken); err == nil {
		v.UserID = c.UserID
	}
	return v, cookies, nil
}

// 저장 시각을 서버 기준으로 고정하고 DB 원자 증가로 재시도와 여러 프로세스를 함께 제한합니다.
func (s *CommentsService) limit(ctx context.Context, v Visitor, action string, max int) error {
	window := time.Now().Unix() / 60
	tx, err := s.repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO feedback_rate(network_hash,action,window_start) VALUES (?,?,?) ON DUPLICATE KEY UPDATE requests=requests+1`, v.Network, action, window)
	if err != nil {
		return err
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT requests FROM feedback_rate WHERE network_hash=? AND action=? AND window_start=?`, v.Network, action, window).Scan(&count); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	_, _ = s.repository.db.ExecContext(ctx, `DELETE FROM feedback_rate WHERE window_start < ? LIMIT 100`, window-2880)
	if count > max {
		return fail(429, "요청이 많습니다. 잠시 후 다시 시도해 주세요.")
	}
	return nil
}
func checkComment(r CommentRequest, create bool) error {
	if utf8.RuneCountInString(strings.TrimSpace(r.Content)) < 1 || utf8.RuneCountInString(r.Content) > 2000 {
		return fail(400, "댓글은 1자 이상 2,000자 이하로 입력해 주세요.")
	}
	if create && (utf8.RuneCountInString(strings.TrimSpace(r.Nickname)) < 1 || utf8.RuneCountInString(r.Nickname) > 24) {
		return fail(400, "닉네임은 1자 이상 24자 이하로 입력해 주세요.")
	}
	if len(r.Password) < 4 || len(r.Password) > 72 {
		return fail(400, "비밀번호는 4바이트 이상 72바이트 이하로 입력해 주세요.")
	}
	return nil
}
func (s *CommentsService) List(ctx context.Context, postID int64, page int) (CommentPage, error) {
	if page < 0 {
		page = 0
	}
	out := CommentPage{Content: []Comment{}, Number: page}
	var exists int
	if err := s.repository.db.QueryRowContext(ctx, `SELECT post_id FROM post WHERE post_id=?`, postID).Scan(&exists); err != nil {
		return out, fail(404, "글을 찾을 수 없습니다.")
	}
	if err := s.repository.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM post_comment WHERE post_id=?`, postID).Scan(&out.TotalElements); err != nil {
		return out, err
	}
	out.TotalPages = int((out.TotalElements + 19) / 20)
	rows, err := s.repository.db.QueryContext(ctx, `SELECT id,nickname,content,created_at,updated_at FROM post_comment WHERE post_id=? ORDER BY id LIMIT 20 OFFSET ?`, postID, page*20)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c Comment
		var a, b time.Time
		if err = rows.Scan(&c.ID, &c.Nickname, &c.Content, &a, &b); err != nil {
			return out, err
		}
		c.CreatedAt = a.Format(time.RFC3339)
		c.UpdatedAt = b.Format(time.RFC3339)
		out.Content = append(out.Content, c)
	}
	return out, rows.Err()
}
func (s *CommentsService) Create(ctx context.Context, v Visitor, postID int64, r CommentRequest) error {
	if err := checkComment(r, true); err != nil {
		return err
	}
	if err := s.limit(ctx, v, "comment-create", 5); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(r.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	result, err := s.repository.db.ExecContext(ctx, `INSERT INTO post_comment(post_id,nickname,content,password_hash) SELECT post_id,?,?,? FROM post WHERE post_id=?`, strings.TrimSpace(r.Nickname), strings.TrimSpace(r.Content), string(hash), postID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fail(404, "글을 찾을 수 없습니다.")
	}
	return nil
}
func (s *CommentsService) Change(ctx context.Context, v Visitor, id int64, r CommentRequest, remove bool) error {
	if len(r.Password) > 72 {
		return fail(400, "비밀번호가 너무 깁니다.")
	}
	if !remove {
		if err := checkComment(r, false); err != nil {
			return err
		}
	}
	if err := s.limit(ctx, v, "comment-manage", 10); err != nil {
		return err
	}
	var hash string
	if err := s.repository.db.QueryRowContext(ctx, `SELECT password_hash FROM post_comment WHERE id=?`, id).Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fail(404, "댓글을 찾을 수 없습니다.")
		}
		return err
	}
	admin := false
	if remove && v.UserID > 0 {
		var role string
		_ = s.repository.db.QueryRowContext(ctx, "SELECT role FROM `user` WHERE user_id=?", v.UserID).Scan(&role)
		admin = role == "ROLE_ADMIN"
	}
	if !admin && bcrypt.CompareHashAndPassword([]byte(hash), []byte(r.Password)) != nil {
		return fail(403, "비밀번호가 일치하지 않습니다.")
	}
	var result sql.Result
	var err error
	if remove {
		result, err = s.repository.db.ExecContext(ctx, `DELETE FROM post_comment WHERE id=? AND password_hash=?`, id, hash)
	} else {
		result, err = s.repository.db.ExecContext(ctx, `UPDATE post_comment SET content=?,updated_at=CURRENT_TIMESTAMP(6) WHERE id=? AND password_hash=?`, strings.TrimSpace(r.Content), id, hash)
	}
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fail(404, "댓글을 찾을 수 없습니다.")
	}
	return nil
}
func (s *CommentsService) Reactions(ctx context.Context, v Visitor, postID int64) (ReactionState, error) {
	out := ReactionState{Users: []model.UserResponse{}}
	var found int
	if err := s.repository.db.QueryRowContext(ctx, `SELECT post_id FROM post WHERE post_id=?`, postID).Scan(&found); err != nil {
		return out, fail(404, "글을 찾을 수 없습니다.")
	}
	err := s.repository.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(user_id IS NULL),0),COALESCE(SUM(visitor_hash=? OR (user_id=? AND ?>0)),0) FROM post_reaction WHERE post_id=? AND active=1`, v.Key, v.UserID, v.UserID, postID).Scan(&out.Count, &out.AnonymousCount, &found)
	if err != nil {
		return out, err
	}
	out.Liked = found > 0
	rows, err := s.repository.db.QueryContext(ctx, "SELECT u.user_id,u.username,COALESCE(u.profile_image_url,'') FROM post_reaction r JOIN `user` u ON u.user_id=r.user_id WHERE r.post_id=? AND r.active=1 ORDER BY r.id", postID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var u model.UserResponse
		if err = rows.Scan(&u.ID, &u.Username, &u.ProfileImageURL); err != nil {
			return out, err
		}
		out.Users = append(out.Users, u)
	}
	return out, rows.Err()
}
func (s *CommentsService) React(ctx context.Context, v Visitor, postID int64, active bool) (ReactionState, error) {
	if err := s.limit(ctx, v, "like", 60); err != nil {
		return ReactionState{}, err
	}
	tx, err := s.repository.db.BeginTx(ctx, nil)
	if err != nil {
		return ReactionState{}, err
	}
	defer tx.Rollback()
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT post_id FROM post WHERE post_id=? FOR UPDATE`, postID).Scan(&id); err != nil {
		return ReactionState{}, fail(404, "글을 찾을 수 없습니다.")
	}
	// 같은 브라우저의 익명 반응과 로그인 계정 반응이 함께 존재하면 계정 행으로 합칩니다.
	if v.UserID > 0 {
		var accountID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM post_reaction WHERE post_id=? AND user_id=?`, postID, v.UserID).Scan(&accountID)
		if err == nil {
			if _, err = tx.ExecContext(ctx, `DELETE FROM post_reaction WHERE post_id=? AND visitor_hash=? AND id<>?`, postID, v.Key, accountID); err != nil {
				return ReactionState{}, err
			}
			_, err = tx.ExecContext(ctx, `UPDATE post_reaction SET visitor_hash=?,active=? WHERE id=?`, v.Key, active, accountID)
		} else if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.ExecContext(ctx, `INSERT INTO post_reaction(post_id,user_id,visitor_hash,active) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE user_id=VALUES(user_id),active=VALUES(active)`, postID, v.UserID, v.Key, active)
		}
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO post_reaction(post_id,visitor_hash,active) VALUES (?,?,?) ON DUPLICATE KEY UPDATE active=VALUES(active)`, postID, v.Key, active)
	}
	if err != nil {
		return ReactionState{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE post SET like_count=(SELECT COUNT(*) FROM post_reaction WHERE post_id=? AND active=1) WHERE post_id=?`, postID, postID)
	if err != nil {
		return ReactionState{}, err
	}
	if err = tx.Commit(); err != nil {
		return ReactionState{}, err
	}
	return s.Reactions(ctx, v, postID)
}
