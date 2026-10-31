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
	if err = check(admin, "DELETE", fmt.Sprintf("/api/v1/post/%d", postID), nil, 200); err != nil {
		return err
	}
	fmt.Println("통합 검증 통과: 역할 마이그레이션, 로그인, 권한 차단, 작성자 수정, 관리자 삭제, 프로필 역할, 검색 중복 및 페이지네이션, Quill/Mermaid 본문 보존")
	return nil
}
