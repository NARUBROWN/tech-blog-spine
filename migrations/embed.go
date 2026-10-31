package migrations

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed 002_feedback.sql
var feedbackSchema string

// Feedback는 기존 테이블을 변경하지 않고 신규 테이블만 준비합니다.
func Feedback(db *sql.DB) error {
	for _, statement := range strings.Split(feedbackSchema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		name := strings.Fields(statement)[5]
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err := db.Exec(statement); err != nil {
				return fmt.Errorf("댓글 테이블 준비 실패: %w", err)
			}
		}
	}
	_, err := db.Exec(`INSERT IGNORE INTO post_reaction(post_id,user_id,active) SELECT DISTINCT post_id,user_id,1 FROM likes WHERE user_id IS NOT NULL`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE post p SET like_count=(SELECT COUNT(*) FROM post_reaction r WHERE r.post_id=p.post_id AND r.active=1)`)
	return err
}

//go:embed 003_analytics.sql
var analyticsSchema string

func Analytics(db *sql.DB) error {
	for _, statement := range strings.Split(analyticsSchema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		name := strings.Fields(statement)[5]
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?`, name).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			if _, err := db.Exec(statement); err != nil {
				return err
			}
		}
	}
	return nil
}
