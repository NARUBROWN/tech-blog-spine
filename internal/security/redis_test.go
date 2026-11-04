package security

import (
	"context"
	"io"
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

func testRedisStore(t *testing.T) *RedisRefreshTokenStore {
	t.Helper()
	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server가 없어 로컬 Redis 회귀 검사를 생략합니다. HTTP 통합 검사는 별도로 실행합니다.")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "--bind", "127.0.0.1", "--port", port, "--save", "", "--appendonly", "no", "--dir", t.TempDir())
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	for attempt := 0; attempt < 100; attempt++ {
		connection, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return NewRedisRefreshTokenStore(config.Config{RedisAddress: address})
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("테스트용 Redis의 시작을 확인하지 못했습니다")
	return nil
}

func TestRedisRefreshSessionsAreIndependent(t *testing.T) {
	store := testRedisStore(t)
	ctx := context.Background()
	const first, second = "first-browser-token", "second-browser-token"
	for _, token := range []string{first, second} {
		if err := store.Save(ctx, 7, token, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	for _, token := range []string{first, second} {
		if stored, err := store.Get(ctx, 7, token); err != nil || stored != token {
			t.Fatalf("다른 로그인 때문에 저장된 세션을 잃었습니다: %v", err)
		}
		ttl, err := store.command(ctx, "TTL", refreshSessionKey(7, token))
		if err != nil {
			t.Fatal(err)
		}
		if seconds, ok := ttl.(int64); !ok || seconds <= 0 || seconds > 60 {
			t.Fatalf("세션 만료 시간이 올바르지 않습니다: %v", ttl)
		}
	}
	if stored, err := store.Get(ctx, 8, first); err != nil || stored != "" {
		t.Fatalf("다른 계정의 세션을 조회했습니다: %v", err)
	}
	if err := store.Delete(ctx, 7, "unknown-token"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, 7, first); err != nil {
		t.Fatal(err)
	}
	if stored, err := store.Get(ctx, 7, first); err != nil || stored != "" {
		t.Fatalf("로그아웃한 세션이 남았습니다: %v", err)
	}
	if stored, err := store.Get(ctx, 7, second); err != nil || stored != second {
		t.Fatalf("다른 브라우저의 로그아웃이 현재 세션을 삭제했습니다: %v", err)
	}
}

func TestRedisRefreshSessionsPreserveLegacyLogins(t *testing.T) {
	store := testRedisStore(t)
	ctx := context.Background()
	const legacy, current = "legacy-browser-token", "current-browser-token"
	if _, err := store.command(ctx, "SET", refreshKey(7), legacy, "EX", "60"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, 7, current, time.Minute); err != nil {
		t.Fatal(err)
	}
	if stored, err := store.Get(ctx, 7, legacy); err != nil || stored != legacy {
		t.Fatalf("기존 로그인 세션을 읽지 못했습니다: %v", err)
	}
	if err := store.Delete(ctx, 7, current); err != nil {
		t.Fatal(err)
	}
	if stored, err := store.Get(ctx, 7, legacy); err != nil || stored != legacy {
		t.Fatalf("새 세션의 로그아웃이 기존 로그인을 삭제했습니다: %v", err)
	}
	if err := store.Save(ctx, 7, current, time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, 7, legacy); err != nil {
		t.Fatal(err)
	}
	if stored, err := store.Get(ctx, 7, current); err != nil || stored != current {
		t.Fatalf("기존 세션의 로그아웃이 새 로그인을 삭제했습니다: %v", err)
	}
	if stored, err := store.Get(ctx, 7, legacy); err != nil || stored != "" {
		t.Fatalf("로그아웃한 기존 세션이 남았습니다: %v", err)
	}
	if _, err := store.command(ctx, "SET", refreshKey(7), current, "EX", "60"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, 7, legacy); err != nil {
		t.Fatal(err)
	}
	if stored, err := store.command(ctx, "GET", refreshKey(7)); err != nil || stored != current {
		t.Fatalf("이전 토큰이 최신 기존 형식 세션을 삭제했습니다: %v", err)
	}
}
