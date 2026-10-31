// integration은 격리된 테스트 DB에서 인증, 권한, 검색과 본문 왕복을 확인합니다.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	if !strings.HasSuffix(c.MySQLDatabase, "_test") {
		return fmt.Errorf("통합 테스트는 _test DB에서만 실행할 수 있습니다")
	}
	d := mysql.Config{User: c.MySQLUser, Passwd: c.MySQLPassword, Net: "tcp", Addr: c.MySQLHost + ":" + c.MySQLPort, DBName: c.MySQLDatabase, AllowNativePasswords: true}
	db, err := sql.Open("mysql", d.FormatDSN())
	if err != nil {
		return err
	}
	defer db.Close()
	// 운영에서 사용하던 enum을 재현한 후 실제 마이그레이션 프로그램을 두 번 실행합니다.
	if _, err = db.Exec("ALTER TABLE `user` MODIFY role ENUM('ROLE_ADMIN','ROLE_USER','ROLE_GUEST') NULL"); err != nil {
		return err
	}
	for i := 0; i < 2; i++ {
		cmd := exec.Command("./dist/migrate")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("integration-password"), bcrypt.MinCost)
	if _, err = db.Exec("INSERT INTO `user` (username,email,hashed_password,role) VALUES (?,?,?,?)", "integration-admin", "admin@example.com", hash, "ROLE_ADMIN"); err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "spine-image-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cmd := exec.Command("./dist/tech-blog-spine")
	cmd.Env = append(os.Environ(), "IMAGE_UPLOAD_DIR="+dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		return err
	}
	defer func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() }()
	base := "http://127.0.0.1:" + os.Getenv("SPINE_PORT")
	for i := 0; i < 60; i++ {
		r, e := http.Get(base + "/health")
		if e == nil {
			r.Body.Close()
			break
		}
		if i == 59 {
			return e
		}
		time.Sleep(100 * time.Millisecond)
	}
	jar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	jar2, _ := cookiejar.New(nil)
	author := &http.Client{Jar: jar2, Timeout: 10 * time.Second}
	jar3, _ := cookiejar.New(nil)
	other := &http.Client{Jar: jar3, Timeout: 10 * time.Second}
	call := func(client *http.Client, method, path string, body any, want int) ([]byte, error) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(context.Background(), method, base+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		r, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer r.Body.Close()
		v, _ := io.ReadAll(r.Body)
		if r.StatusCode != want {
			return nil, fmt.Errorf("%s %s status %d want %d: %s", method, path, r.StatusCode, want, v)
		}
		return v, nil
	}
	reqUser := func(name string) map[string]any {
		return map[string]any{"username": name, "email": name + "@example.com", "originalPassword": "integration-password"}
	}
	login := func(name string) map[string]any {
		return map[string]any{"username": name, "password": "integration-password"}
	}
	check := func(client *http.Client, method, path string, body any, want int) error {
		_, e := call(client, method, path, body, want)
		return e
	}
	if err = check(other, "POST", "/api/v1/user/admin-signup", reqUser("evil"), 401); err != nil {
		return err
	}
	if err = check(admin, "POST", "/api/v1/auth/login", login("integration-admin"), 200); err != nil {
		return err
	}
	if err = check(admin, "POST", "/api/v1/user/author-signup", reqUser("integration-author"), 200); err != nil {
		return err
	}
	if err = check(other, "POST", "/api/v1/user/normal-signup", reqUser("integration-user"), 200); err != nil {
		return err
	}
	if err = check(author, "POST", "/api/v1/auth/login", login("integration-author"), 200); err != nil {
		return err
	}
	if err = check(other, "POST", "/api/v1/auth/login", login("integration-user"), 200); err != nil {
		return err
	}
	if err = check(other, "POST", "/api/v1/user/admin-signup", reqUser("evil"), 403); err != nil {
		return err
	}
	if err = check(other, "POST", "/api/v1/category?name=forbidden", nil, 403); err != nil {
		return err
	}
	if err = check(admin, "POST", "/api/v1/category?name=integration", nil, 200); err != nil {
		return err
	}
	var categoryID int64
	if err = db.QueryRow("SELECT category_id FROM category WHERE name='integration'").Scan(&categoryID); err != nil {
		return err
	}
	body := `<h3>제목</h3><ol><li>목록</li></ol><div class="ql-mermaid-block" data-mermaid="sequenceDiagram%0AA-%3E%3EB%3Ahello"><pre>sequenceDiagram</pre></div>`
	post := map[string]any{"title": "integration-topic", "content": body, "tags": map[string]any{"tagNames": []string{"topic", "matching-topic"}}}
	createPath := fmt.Sprintf("/api/v1/post?categoryId=%d", categoryID)
	if err = check(other, "POST", createPath, post, 403); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
		post["title"] = fmt.Sprintf("integration-topic-%d", i)
		if err = check(author, "POST", createPath, post, 200); err != nil {
			return err
		}
	}
	var postID int64
	_ = db.QueryRow("SELECT MIN(post_id) FROM post").Scan(&postID)
	patch := fmt.Sprintf("/api/v1/post?categoryId=%d&postId=%d", categoryID, postID)
	if err = check(other, "PATCH", patch, post, 403); err != nil {
		return err
	}
	if err = check(other, "DELETE", fmt.Sprintf("/api/v1/post/%d", postID), nil, 403); err != nil {
		return err
	}
	if err = check(author, "PATCH", patch, post, 200); err != nil {
		return err
	}
	data, err := call(author, "GET", fmt.Sprintf("/api/v1/post/id/%d", postID), nil, 200)
	if err != nil {
		return err
	}
	var detail struct{ Content string }
	_ = json.Unmarshal(data, &detail)
	if detail.Content != body {
		return fmt.Errorf("Quill/Mermaid 본문이 변경됐습니다")
	}
	for page := 0; page < 2; page++ {
		data, err = call(other, "GET", fmt.Sprintf("/api/v1/post/search/recent?keyword=integration-topic&page=%d&size=2", page), nil, 200)
		if err != nil {
			return err
		}
		var result struct {
			Content       []json.RawMessage
			TotalElements int
			TotalPages    int
			Number        int
		}
		_ = json.Unmarshal(data, &result)
		n := 2
		if page == 1 {
			n = 1
		}
		if result.TotalElements != 3 || result.TotalPages != 2 || result.Number != page || len(result.Content) != n {
			return fmt.Errorf("검색 페이지네이션 실패: %s", data)
		}
	}
	var adminID int64
	_ = db.QueryRow("SELECT user_id FROM user WHERE username='integration-admin'").Scan(&adminID)
	data, err = call(admin, "PATCH", fmt.Sprintf("/api/v1/user/%d", adminID), reqUser("integration-admin"), 200)
	if err != nil {
		return err
	}
	var profile struct{ Role string }
	_ = json.Unmarshal(data, &profile)
	if profile.Role != "ROLE_ADMIN" {
		return fmt.Errorf("프로필 role 유실")
	}
	if err = analyticsIntegration(base, admin, other, postID); err != nil {
		return err
	}
	if err = feedbackIntegration(base, postID); err != nil {
		return err
	}
	if err = check(admin, "DELETE", fmt.Sprintf("/api/v1/post/%d", postID), nil, 200); err != nil {
		return err
	}
	fmt.Println("통합 검증 통과: 역할 마이그레이션, 로그인, 권한 차단, 작성자 수정, 관리자 삭제, 프로필 역할, 검색 중복 및 페이지네이션, Quill/Mermaid 본문 보존")
	return nil
}

// 실제 HTTP와 MySQL에서 익명 쿠키, 댓글 비밀번호, 동시 중복 요청을 확인합니다.
func feedbackIntegration(base string, postID int64) error {
	jar, _ := cookiejar.New(nil)
	anon := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	call := func(method, path string, body any, want int) ([]byte, error) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, base+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		r, err := anon.Do(req)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		data, _ := io.ReadAll(r.Body)
		if r.StatusCode != want {
			return nil, fmt.Errorf("feedback %s %s: status=%d want=%d body=%s", method, path, r.StatusCode, want, data)
		}
		return data, nil
	}
	likes := fmt.Sprintf("/api/v1/like/%d", postID)
	comments := fmt.Sprintf("/api/v1/comments/post/%d", postID)
	if _, err := call("POST", likes, nil, 409); err != nil {
		return err
	}
	if _, err := call("GET", likes+"/state", nil, 200); err != nil {
		return err
	}
	if _, err := call("POST", likes, nil, 200); err != nil {
		return err
	}
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { _, err := call("POST", likes, nil, 200); errors <- err }()
	}
	for i := 0; i < 12; i++ {
		if err := <-errors; err != nil {
			return err
		}
	}
	data, err := call("GET", likes+"/state", nil, 200)
	if err != nil {
		return err
	}
	var state struct {
		Count int
		Liked bool
	}
	_ = json.Unmarshal(data, &state)
	if state.Count != 1 || !state.Liked {
		return fmt.Errorf("중복 좋아요 집계: %s", data)
	}
	for i := 0; i < 2; i++ {
		if _, err = call("DELETE", likes, nil, 200); err != nil {
			return err
		}
	}
	data, err = call("GET", likes+"/state", nil, 200)
	if err != nil {
		return err
	}
	_ = json.Unmarshal(data, &state)
	if state.Count != 0 || state.Liked {
		return fmt.Errorf("중복 취소 처리: %s", data)
	}
	body := map[string]any{"nickname": "익명", "password": "test-password", "content": "<script>alert(1)</script>\n일반 텍스트"}
	if _, err = call("POST", comments, body, 200); err != nil {
		return err
	}
	data, err = call("GET", comments, nil, 200)
	if err != nil {
		return err
	}
	var page struct {
		Content []struct {
			ID      int64
			Content string
		}
		TotalElements int
	}
	_ = json.Unmarshal(data, &page)
	if len(page.Content) != 1 || strings.Contains(string(data), "test-password") || strings.Contains(string(data), "password_hash") {
		return fmt.Errorf("댓글 공개 DTO 실패: %s", data)
	}
	id := page.Content[0].ID
	manage := fmt.Sprintf("/api/v1/comments/%d", id)
	if _, err = call("PATCH", manage, map[string]string{"content": "변조", "password": "wrong-password"}, 403); err != nil {
		return err
	}
	if _, err = call("DELETE", manage, map[string]string{"password": "wrong-password"}, 403); err != nil {
		return err
	}
	if _, err = call("PATCH", manage, map[string]string{"content": "수정 완료", "password": "test-password"}, 200); err != nil {
		return err
	}
	if _, err = call("DELETE", manage, map[string]string{"password": "test-password"}, 200); err != nil {
		return err
	}
	data, err = call("GET", comments, nil, 200)
	if err != nil {
		return err
	}
	_ = json.Unmarshal(data, &page)
	if page.TotalElements != 0 {
		return fmt.Errorf("댓글 삭제 실패")
	}
	// 서명 위조는 쓰기 요청에서 거부됩니다.
	u, _ := http.NewRequest("GET", base, nil)
	jar.SetCookies(u.URL, []*http.Cookie{{Name: "feedback_visitor", Value: strings.Repeat("a", 64) + ".forged", Path: "/"}})
	if _, err = call("POST", likes, nil, 409); err != nil {
		return err
	}
	fmt.Println("익명 HTTP 검증 통과: 12개 동시 좋아요 1표, 중복 취소, 댓글 생성/수정/삭제, 비밀번호 실패, 비밀값 비노출, 쿠키 위조 거부")
	return nil
}

func analyticsIntegration(base string, admin, normal *http.Client, postID int64) error {
	jar, _ := cookiejar.New(nil)
	anon := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	otherJar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: otherJar, Timeout: 10 * time.Second}
	uuid := func(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }
	call := func(client *http.Client, method, path string, body any, want int) ([]byte, error) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, base+path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		r, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()
		data, _ := io.ReadAll(r.Body)
		if r.StatusCode != want {
			return nil, fmt.Errorf("analytics %s %s: %d want %d: %s", method, path, r.StatusCode, want, data)
		}
		return data, nil
	}
	if _, err := call(anon, "GET", "/api/v1/admin/analytics?days=30", nil, 401); err != nil {
		return err
	}
	if _, err := call(normal, "GET", "/api/v1/admin/analytics?days=30", nil, 403); err != nil {
		return err
	}
	visit := func(n int, path string) map[string]any {
		return map[string]any{"id": uuid(n), "sessionId": uuid(100), "path": path, "referrer": "https://search.example/query?token=private", "device": "mobile"}
	}
	for _, item := range []struct {
		n    int
		path string
	}{{1, "/about"}, {1, "/about"}, {2, "/about"}, {3, "/"}, {4, fmt.Sprintf("/post/share/%d", postID)}} {
		if _, err := call(anon, "POST", "/api/v1/analytics/visits", visit(item.n, item.path), 200); err != nil {
			return err
		}
	}
	data, err := call(admin, "POST", "/api/v1/analytics/visits", visit(5, "/about"), 200)
	if err != nil {
		return err
	}
	var started struct{ Tracked bool }
	_ = json.Unmarshal(data, &started)
	if started.Tracked {
		return fmt.Errorf("관리자 방문이 집계됐습니다")
	}
	if _, err = call(other, "POST", "/api/v1/analytics/visits", visit(6, "/"), 200); err != nil {
		return err
	}
	batch := map[string]any{"visitId": uuid(1), "activeMs": 500, "scrollDepth": 75, "events": []map[string]any{
		{"id": uuid(201), "type": "resume_selection", "text": "이력서에서 관심을 가진 공개 문장입니다", "context": "이 문장의 주변 문맥입니다", "section": "experience", "durationMs": 850},
		{"id": uuid(202), "type": "snackbar_resume", "label": "자기소개서 보기"},
		{"id": uuid(203), "type": "snackbar_linkedin", "label": "LinkedIn으로 연락하기"},
		{"id": uuid(204), "type": "resume_outbound", "label": "Spine", "url": "https://github.com/NARUBROWN/spine?token=private#secret", "section": "personal-projects"},
	}}
	for i := 0; i < 2; i++ {
		if _, err = call(anon, "POST", "/api/v1/analytics/events", batch, 200); err != nil {
			return err
		}
	}
	if _, err = call(other, "POST", "/api/v1/analytics/events", batch, 404); err != nil {
		return err
	}
	invalid := map[string]any{"visitId": uuid(3), "events": []map[string]any{{"id": uuid(205), "type": "resume_selection", "text": "다른 페이지에서 선택한 문장입니다", "durationMs": 900}}}
	if _, err = call(anon, "POST", "/api/v1/analytics/events", invalid, 400); err != nil {
		return err
	}
	data, err = call(anon, "GET", "/api/v1/analytics/interest", nil, 200)
	if err != nil {
		return err
	}
	var interest struct{ Count int }
	_ = json.Unmarshal(data, &interest)
	if interest.Count != 1 {
		return fmt.Errorf("이력서 방문자 중복 집계: %s", data)
	}
	data, err = call(admin, "GET", "/api/v1/admin/analytics?days=30", nil, 200)
	if err != nil {
		return err
	}
	var overview struct {
		Summary struct{ Views, Visitors, Sessions, AverageActiveMs, ResumeViews, ResumeVisitors, ResumeClicks, LinkedInClicks, Selections, OutboundClicks int }
		Daily   []any
		Groups  []any
	}
	_ = json.Unmarshal(data, &overview)
	m := overview.Summary
	if m.Views != 5 || m.Visitors != 2 || m.Sessions != 2 || m.AverageActiveMs != 100 || m.ResumeViews != 2 || m.ResumeVisitors != 1 || m.ResumeClicks != 1 || m.LinkedInClicks != 1 || m.Selections != 1 || m.OutboundClicks != 1 || len(overview.Daily) != 30 || len(overview.Groups) != 4 {
		return fmt.Errorf("계측 집계 불일치: %s", data)
	}
	data, err = call(admin, "GET", "/api/v1/admin/analytics/events?days=30&type=resume_selection&page=0", nil, 200)
	if err != nil {
		return err
	}
	var events struct {
		TotalElements int
		Content       []struct{ Text, Visitor string }
	}
	_ = json.Unmarshal(data, &events)
	if events.TotalElements != 1 || len(events.Content) != 1 || events.Content[0].Text != "이력서에서 관심을 가진 공개 문장입니다" || len(events.Content[0].Visitor) != 10 {
		return fmt.Errorf("문장 선택 조회 실패: %s", data)
	}
	data, err = call(admin, "GET", "/api/v1/admin/analytics/events?days=30&page=0", nil, 200)
	if err != nil {
		return err
	}
	if strings.Contains(string(data), "token=private") || strings.Contains(string(data), "#secret") {
		return fmt.Errorf("링크 비밀값이 기록됐습니다")
	}
	fmt.Println("계측 HTTP 검증 통과: 관리자 API 401/403, 익명 방문자 중복 제거, 관리자 방문 제외, 평균 체류, 문장 선택/클릭 집계, 이벤트 재전송 중복 방지, 다른 방문자 접근 거부, URL 비밀값 제거")
	return nil
}
