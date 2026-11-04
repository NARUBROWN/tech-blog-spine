package security

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

type RefreshTokenStore interface {
	Save(context.Context, int64, string, time.Duration) error
	Get(context.Context, int64, string) (string, error)
	Delete(context.Context, int64, string) error
}

type RedisRefreshTokenStore struct {
	address  string
	password string
	database int
	useTLS   bool
}

func NewRedisRefreshTokenStore(config config.Config) *RedisRefreshTokenStore {
	return &RedisRefreshTokenStore{
		address: config.RedisAddress, password: config.RedisPassword,
		database: config.RedisDB, useTLS: config.RedisTLS,
	}
}

func (s *RedisRefreshTokenStore) Save(ctx context.Context, userID int64, token string, ttl time.Duration) error {
	_, err := s.command(ctx, "SET", refreshSessionKey(userID, token), token, "EX", strconv.FormatInt(int64(ttl.Seconds()), 10))
	return err
}

func (s *RedisRefreshTokenStore) Get(ctx context.Context, userID int64, token string) (string, error) {
	value, err := s.command(ctx, "GET", refreshSessionKey(userID, token))
	if err != nil {
		return "", err
	}
	if value == nil {
		// 배포 전에 발급된 토큰은 기존 계정별 키에서 만료 시점까지 확인합니다.
		value, err = s.command(ctx, "GET", refreshKey(userID))
		if err != nil {
			return "", err
		}
	}
	if value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("Redis GET 응답 형식이 올바르지 않습니다")
	}
	return text, nil
}

func (s *RedisRefreshTokenStore) Delete(ctx context.Context, userID int64, token string) error {
	// 비교와 삭제를 한 번에 실행해 다른 로그인에서 저장한 토큰을 삭제하지 않습니다.
	const script = `
local deleted = 0
for _, key in ipairs(KEYS) do
    if redis.call('GET', key) == ARGV[1] then
        deleted = deleted + redis.call('DEL', key)
    end
end
return deleted`
	_, err := s.command(ctx, "EVAL", script, "2", refreshSessionKey(userID, token), refreshKey(userID), token)
	return err
}

func refreshKey(userID int64) string {
	return "spine:refreshToken:" + strconv.FormatInt(userID, 10)
}

func refreshSessionKey(userID int64, token string) string {
	return fmt.Sprintf("%s:%x", refreshKey(userID), sha256.Sum256([]byte(token)))
}

func (s *RedisRefreshTokenStore) command(ctx context.Context, args ...string) (any, error) {
	connection, err := s.dial(ctx)
	if err != nil {
		return nil, fmt.Errorf("Redis 연결 실패: %w", err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
	}
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	if s.password != "" {
		if err := writeRESP(writer, "AUTH", s.password); err != nil {
			return nil, err
		}
		if _, err := readRESP(reader); err != nil {
			return nil, fmt.Errorf("Redis AUTH 실패: %w", err)
		}
	}
	if s.database != 0 {
		if err := writeRESP(writer, "SELECT", strconv.Itoa(s.database)); err != nil {
			return nil, err
		}
		if _, err := readRESP(reader); err != nil {
			return nil, fmt.Errorf("Redis SELECT 실패: %w", err)
		}
	}
	if err := writeRESP(writer, args...); err != nil {
		return nil, err
	}
	return readRESP(reader)
}

func (s *RedisRefreshTokenStore) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	if !s.useTLS {
		return dialer.DialContext(ctx, "tcp", s.address)
	}
	host, _, err := net.SplitHostPort(s.address)
	if err != nil {
		return nil, err
	}
	tlsDialer := tls.Dialer{NetDialer: dialer, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}}
	return tlsDialer.DialContext(ctx, "tcp", s.address)
}

func writeRESP(writer *bufio.Writer, args ...string) error {
	if _, err := fmt.Fprintf(writer, "*%d\r\n", len(args)); err != nil {
		return err
	}
	for _, arg := range args {
		if _, err := fmt.Fprintf(writer, "$%d\r\n%s\r\n", len(arg), arg); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func readRESP(reader *bufio.Reader) (any, error) {
	prefix, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	line, err := readRESPLine(reader)
	if err != nil {
		return nil, err
	}
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, fmt.Errorf("Redis 오류: %s", line)
	case ':':
		return strconv.ParseInt(line, 10, 64)
	case '$':
		length, err := strconv.Atoi(line)
		if err != nil {
			return nil, err
		}
		if length == -1 {
			return nil, nil
		}
		buffer := make([]byte, length+2)
		if _, err := io.ReadFull(reader, buffer); err != nil {
			return nil, err
		}
		if string(buffer[length:]) != "\r\n" {
			return nil, fmt.Errorf("Redis bulk 응답 구분자가 올바르지 않습니다")
		}
		return string(buffer[:length]), nil
	default:
		return nil, fmt.Errorf("지원하지 않는 Redis 응답: %q", prefix)
	}
}

func readRESPLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(line, "\r\n") {
		return "", fmt.Errorf("Redis 응답 줄바꿈이 올바르지 않습니다")
	}
	return strings.TrimSuffix(line, "\r\n"), nil
}
