// migrate는 기존 권한 값을 보존하면서 작성자 역할만 추가합니다.
package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/go-sql-driver/mysql"
)

func main() {
	c, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	d := mysql.Config{User: c.MySQLUser, Passwd: c.MySQLPassword, Net: "tcp", Addr: c.MySQLHost + ":" + c.MySQLPort, DBName: c.MySQLDatabase, AllowNativePasswords: true}
	db, err := sql.Open("mysql", d.FormatDSN())
	if err != nil {
		log.Fatal("DB 설정 실패")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var typ string
	if err = db.QueryRowContext(ctx, `SELECT COLUMN_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='user' AND COLUMN_NAME='role'`).Scan(&typ); err != nil {
		log.Fatal("권한 스키마 조회 실패")
	}
	if typ == "varchar(30)" {
		log.Print("문자열 권한 컬럼: 변경이 필요하지 않습니다")
		return
	}
	if !strings.HasPrefix(typ, "enum(") {
		log.Fatal("예상하지 않은 권한 컬럼 형식: 자동 변경 중단")
	}
	if strings.Contains(typ, "'ROLE_AUTHOR'") {
		log.Print("작성자 역할이 이미 존재합니다")
		return
	}
	// 기존 enum 순서를 유지해야 기존 역할의 내부 인덱스가 바뀌지 않습니다.
	next := strings.TrimSuffix(typ, ")") + ",'ROLE_AUTHOR')"
	if _, err = db.ExecContext(ctx, "ALTER TABLE `user` MODIFY COLUMN role "+next+" NULL"); err != nil {
		log.Fatal("작성자 역할 추가 실패")
	}
	log.Print("기존 역할을 유지하고 ROLE_AUTHOR를 추가했습니다")
}
