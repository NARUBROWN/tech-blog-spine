package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/go-sql-driver/mysql"
)

type MySQLStore struct {
	db *sql.DB
}

const userTable = "`user`"

// NewMySQLStore는 연결 풀만 준비하며 실제 연결은 첫 요청에서 확인합니다.
func NewMySQLStore(config config.Config) *MySQLStore {
	seoul, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		panic(fmt.Errorf("Asia/Seoul 시간대 로드 실패: %w", err))
	}
	driverConfig := mysql.Config{
		User:                 config.MySQLUser,
		Passwd:               config.MySQLPassword,
		Net:                  "tcp",
		Addr:                 config.MySQLHost + ":" + config.MySQLPort,
		DBName:               config.MySQLDatabase,
		ParseTime:            true,
		Loc:                  seoul,
		AllowNativePasswords: true,
		ClientFoundRows:      true,
		Params: map[string]string{
			"charset":   "utf8mb4",
			"time_zone": "'+09:00'",
		},
	}
	db, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		panic(fmt.Errorf("MySQL 연결 풀 생성 실패: %w", err))
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	return &MySQLStore{db: db}
}

func (s *MySQLStore) CreateUser(ctx context.Context, user model.User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO `+userTable+`
			(username, email, hashed_password, profile_image_url, role, bio, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		user.Username, user.Email, user.HashedPassword, user.ProfileImageURL, user.Role, user.Bio,
	)
	return classifyWriteError(err)
}

func (s *MySQLStore) UserByID(ctx context.Context, id int64) (model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
		SELECT user_id, username, email, hashed_password,
		       COALESCE(profile_image_url, ''), role, COALESCE(bio, '')
		FROM `+userTable+` WHERE user_id = ?`, id))
}

func (s *MySQLStore) UserByUsername(ctx context.Context, username string) (model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `
		SELECT user_id, username, email, hashed_password,
		       COALESCE(profile_image_url, ''), role, COALESCE(bio, '')
		FROM `+userTable+` WHERE username = ?`, username))
}

func (s *MySQLStore) UpdateUser(ctx context.Context, user model.User) (model.User, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE `+userTable+`
		SET username = ?, email = ?, hashed_password = ?, profile_image_url = ?, bio = ?, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = ?`,
		user.Username, user.Email, user.HashedPassword, user.ProfileImageURL, user.Bio, user.ID,
	)
	if err != nil {
		return model.User{}, classifyWriteError(err)
	}
	if err := requireAffected(result); err != nil {
		return model.User{}, err
	}
	return s.UserByID(ctx, user.ID)
}

func (s *MySQLStore) CreateCategory(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO category(name) VALUES (?)`, name)
	return classifyWriteError(err)
}

func (s *MySQLStore) ListCategories(ctx context.Context) ([]model.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT category_id, name FROM category ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]model.Category, 0)
	for rows.Next() {
		var category model.Category
		if err := rows.Scan(&category.ID, &category.Name); err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

const postSelect = `
	SELECT p.post_id, p.title, p.slug, p.content,
	       COALESCE(p.published_at, p.created_at, CURRENT_TIMESTAMP),
	       COALESCE(p.like_count, 0), COALESCE(p.view_count, 0),
	       COALESCE(p.thumbnail_url, ''), COALESCE(p.seo_title, ''),
	       COALESCE(p.seo_description, ''), COALESCE(p.seo_keywords, ''),
	       u.user_id, u.username, u.email, u.hashed_password,
	       COALESCE(u.profile_image_url, ''), u.role, COALESCE(u.bio, ''),
	       c.category_id, c.name
	FROM post p
	JOIN ` + userTable + ` u ON u.user_id = p.user_id
	JOIN category c ON c.category_id = p.category_id`

func (s *MySQLStore) CreatePost(ctx context.Context, params CreatePostParams) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var userID int64
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM `+userTable+` WHERE username = ?`, params.Username).Scan(&userID); err != nil {
		return classifyReadError(err)
	}
	var categoryID int64
	if err := tx.QueryRowContext(ctx, `SELECT category_id FROM category WHERE category_id = ?`, params.CategoryID).Scan(&categoryID); err != nil {
		return classifyReadError(err)
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO post
			(title, slug, content, published_at, thumbnail_url, like_count, view_count,
			 seo_title, seo_description, seo_keywords, category_id, user_id, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?, 0, 0, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		params.Request.Title, params.Slug, params.Request.Content, params.Request.ThumbnailURL,
		params.Request.SEOTitle, params.Request.SEODescription, params.Request.SEOKeywords,
		categoryID, userID,
	)
	if err != nil {
		return classifyWriteError(err)
	}
	postID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	if params.Request.Tags != nil {
		if err := replacePostTags(ctx, tx, postID, params.Request.Tags.TagNames); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *MySQLStore) UpdatePost(ctx context.Context, params UpdatePostParams) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var categoryID int64
	if err := tx.QueryRowContext(ctx, `SELECT category_id FROM category WHERE category_id = ?`, params.CategoryID).Scan(&categoryID); err != nil {
		return "", classifyReadError(err)
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE post
		SET title = ?, slug = ?, content = ?, thumbnail_url = ?, category_id = ?,
		    seo_title = ?, seo_description = ?, seo_keywords = ?, updated_at = CURRENT_TIMESTAMP
		WHERE post_id = ?`,
		params.Request.Title, params.Slug, params.Request.Content, params.Request.ThumbnailURL,
		categoryID, params.Request.SEOTitle, params.Request.SEODescription,
		params.Request.SEOKeywords, params.PostID,
	)
	if err != nil {
		return "", classifyWriteError(err)
	}
	if err := requireAffected(result); err != nil {
		return "", err
	}
	if params.Request.Tags != nil {
		if err := replacePostTags(ctx, tx, params.PostID, params.Request.Tags.TagNames); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return params.Slug, nil
}

func (s *MySQLStore) PostByID(ctx context.Context, id int64) (model.Post, error) {
	post, err := scanPost(s.db.QueryRowContext(ctx, postSelect+` WHERE p.post_id = ?`, id))
	if err != nil {
		return model.Post{}, err
	}
	post.Tags, err = s.postTags(ctx, id)
	return post, err
}

func (s *MySQLStore) PostBySlug(ctx context.Context, slug string) (model.Post, error) {
	post, err := scanPost(s.db.QueryRowContext(ctx, postSelect+` WHERE p.slug = ?`, slug))
	if err != nil {
		return model.Post{}, err
	}
	post.Tags, err = s.postTags(ctx, post.ID)
	return post, err
}

func (s *MySQLStore) ListPosts(ctx context.Context, categoryName string, page, size int) ([]model.Post, int64, error) {
	where := ""
	args := make([]any, 0, 3)
	countArgs := make([]any, 0, 1)
	if categoryName != "" {
		where = ` WHERE c.name = ?`
		args = append(args, categoryName)
		countArgs = append(countArgs, categoryName)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM post p JOIN category c ON c.category_id = p.category_id`+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, size, page*size)
	posts, err := s.queryPosts(ctx, postSelect+where+` ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, args...)
	return posts, total, err
}

func (s *MySQLStore) SearchPosts(ctx context.Context, keyword string, page, size int) ([]model.Post, int64, error) {
	pattern := "%" + keyword + "%"
	// EXISTS로 태그 여러 개가 일치해도 게시글을 중복 집계하지 않습니다.
	where := ` WHERE (LOWER(p.title) LIKE LOWER(?) OR p.content LIKE ?
 OR LOWER(COALESCE(p.seo_title, '')) LIKE LOWER(?)
 OR LOWER(COALESCE(p.seo_description, '')) LIKE LOWER(?)
 OR EXISTS (SELECT 1 FROM post_tag pt JOIN tag t ON t.tag_id=pt.tag_id WHERE pt.post_id=p.post_id AND LOWER(t.name) LIKE LOWER(?)))`
	args := []any{pattern, pattern, pattern, pattern, pattern}
	var total int64
	countQuery := `SELECT COUNT(*) FROM post p JOIN ` + userTable + ` u ON u.user_id=p.user_id JOIN category c ON c.category_id=p.category_id` + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	posts, err := s.queryPosts(ctx, postSelect+where+` ORDER BY p.published_at DESC, p.post_id DESC LIMIT ? OFFSET ?`, append(args, size, page*size)...)
	return posts, total, err
}

func (s *MySQLStore) queryPosts(ctx context.Context, query string, args ...any) ([]model.Post, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	posts := make([]model.Post, 0)
	for rows.Next() {
		post, err := scanPost(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		posts = append(posts, post)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range posts {
		posts[index].Tags, err = s.postTags(ctx, posts[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return posts, nil
}

func (s *MySQLStore) DeletePost(ctx context.Context, postID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`DELETE FROM post_tag WHERE post_id = ?`,
		`DELETE FROM likes WHERE post_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, query, postID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM post WHERE post_id = ?`, postID)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MySQLStore) IncrementViewCount(ctx context.Context, postID int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE post SET view_count = COALESCE(view_count, 0) + 1 WHERE post_id = ?`, postID)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (s *MySQLStore) AddLike(ctx context.Context, postID, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM likes WHERE post_id = ? AND user_id = ?`, postID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO likes(post_id, user_id, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, postID, userID); err != nil {
		return classifyWriteError(err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE post SET like_count = COALESCE(like_count, 0) + 1 WHERE post_id = ?`, postID)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MySQLStore) RemoveLike(ctx context.Context, postID, userID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM likes WHERE post_id = ? AND user_id = ?`, postID, userID)
	if err != nil {
		return err
	}
	if err := requireAffected(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE post SET like_count = GREATEST(COALESCE(like_count, 0) - 1, 0) WHERE post_id = ?`, postID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MySQLStore) UsersWhoLiked(ctx context.Context, postID int64) ([]model.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.user_id, u.username, u.email, u.hashed_password,
		       COALESCE(u.profile_image_url, ''), u.role, COALESCE(u.bio, '')
		FROM likes l JOIN `+userTable+` u ON u.user_id = l.user_id
		WHERE l.post_id = ? ORDER BY l.created_at`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := make([]model.User, 0)
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *MySQLStore) SaveImageMetadata(ctx context.Context, image model.ImageMetadata) (model.ImageMetadata, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO image_metadata(public_id, stored_name, original_name, content_type, size)
		VALUES (?, ?, ?, ?, ?)`, image.PublicID, image.StoredName, image.OriginalName, image.ContentType, image.Size)
	if err != nil {
		return model.ImageMetadata{}, classifyWriteError(err)
	}
	image.ID, err = result.LastInsertId()
	return image, err
}

func (s *MySQLStore) ImageByPublicID(ctx context.Context, publicID string) (model.ImageMetadata, error) {
	var image model.ImageMetadata
	err := s.db.QueryRowContext(ctx, `
		SELECT id, public_id, stored_name, original_name, content_type, size
		FROM image_metadata WHERE public_id = ?`, publicID).Scan(
		&image.ID, &image.PublicID, &image.StoredName, &image.OriginalName, &image.ContentType, &image.Size,
	)
	if err != nil {
		return model.ImageMetadata{}, classifyReadError(err)
	}
	return image, nil
}

type scanner interface {
	Scan(...any) error
}

func scanUser(row scanner) (model.User, error) {
	var user model.User
	if err := row.Scan(
		&user.ID, &user.Username, &user.Email, &user.HashedPassword,
		&user.ProfileImageURL, &user.Role, &user.Bio,
	); err != nil {
		return model.User{}, classifyReadError(err)
	}
	return user, nil
}

func scanPost(row scanner) (model.Post, error) {
	var post model.Post
	if err := row.Scan(
		&post.ID, &post.Title, &post.Slug, &post.Content, &post.PublishedAt,
		&post.LikeCount, &post.ViewCount, &post.ThumbnailURL, &post.SEOTitle,
		&post.SEODescription, &post.SEOKeywords,
		&post.Author.ID, &post.Author.Username, &post.Author.Email,
		&post.Author.HashedPassword, &post.Author.ProfileImageURL,
		&post.Author.Role, &post.Author.Bio,
		&post.Category.ID, &post.Category.Name,
	); err != nil {
		return model.Post{}, classifyReadError(err)
	}
	return post, nil
}

func (s *MySQLStore) postTags(ctx context.Context, postID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.name FROM post_tag pt JOIN tag t ON t.tag_id = pt.tag_id
		WHERE pt.post_id = ? ORDER BY t.name`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func replacePostTags(ctx context.Context, tx *sql.Tx, postID int64, names []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM post_tag WHERE post_id = ?`, postID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(names))
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tag(name) VALUES (?) ON DUPLICATE KEY UPDATE tag_id = LAST_INSERT_ID(tag_id)`, name); err != nil {
			return err
		}
		var tagID int64
		if err := tx.QueryRowContext(ctx, `SELECT tag_id FROM tag WHERE name = ? ORDER BY tag_id LIMIT 1`, name).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO post_tag(post_id, tag_id) VALUES (?, ?)`, postID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func classifyReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func classifyWriteError(err error) error {
	if err == nil {
		return nil
	}
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) && mysqlError.Number == 1062 {
		return ErrConflict
	}
	return err
}

func requireAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
