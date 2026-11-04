package blog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/spine/pkg/httpx"
	"github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
	"github.com/NARUBROWN/tech-blog-spine/internal/store"
	"golang.org/x/crypto/bcrypt"
)

var whitespace = regexp.MustCompile(`\s+`)

type Service struct {
	store      store.Store
	tokenStore security.RefreshTokenStore
	tokens     *security.TokenManager
	images     *ImageStorage
}

func NewService(mysql *store.MySQLStore, tokenStore *security.RedisRefreshTokenStore, tokens *security.TokenManager, images *ImageStorage) *Service {
	return &Service{store: mysql, tokenStore: tokenStore, tokens: tokens, images: images}
}

func (s *Service) Login(ctx context.Context, request model.LoginRequest) (model.LoginResponse, []httpx.Cookie, error) {
	user, err := s.store.UserByUsername(ctx, strings.TrimSpace(request.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.HashedPassword), []byte(request.Password)) != nil {
		return model.LoginResponse{}, nil, &httperr.HTTPError{Status: http.StatusForbidden, Message: "아이디 또는 패스워드가 일치하지 않습니다."}
	}
	accessToken, err := s.tokens.CreateAccessToken(user.ID, user.Username)
	if err != nil {
		return model.LoginResponse{}, nil, internalError(err)
	}
	refreshToken, err := s.tokens.CreateRefreshToken(user.ID, user.Username)
	if err != nil {
		return model.LoginResponse{}, nil, internalError(err)
	}
	if err := s.tokenStore.Save(ctx, user.ID, refreshToken, s.tokens.RefreshTTL()); err != nil {
		return model.LoginResponse{}, nil, internalError(err)
	}
	response := model.LoginResponse{
		ID: user.ID, Username: user.Username, Email: user.Email,
		ProfileImageURL: user.ProfileImageURL, Role: user.Role,
	}
	return response, []httpx.Cookie{s.tokens.AccessCookie(accessToken), s.tokens.RefreshCookie(refreshToken)}, nil
}

func (s *Service) Logout(ctx context.Context, refreshToken string) []httpx.Cookie {
	if claims, err := s.tokens.ParseSigned(refreshToken, security.RefreshToken); err == nil {
		_ = s.tokenStore.Delete(ctx, claims.UserID, refreshToken)
	}
	return s.tokens.ClearCookies()
}

func (s *Service) CreateUser(ctx context.Context, request model.UserRequest, role model.Role) error {
	if err := validateUserRequest(request); err != nil {
		return err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.OriginalPassword), bcrypt.DefaultCost)
	if err != nil {
		return internalError(err)
	}
	err = s.store.CreateUser(ctx, model.User{
		Username: strings.TrimSpace(request.Username), Email: strings.TrimSpace(request.Email),
		HashedPassword: string(hashedPassword), ProfileImageURL: strings.TrimSpace(request.ProfileImageURL),
		Bio: strings.TrimSpace(request.Bio), Role: role,
	})
	return translateStoreError(err, "유저를 찾을 수 없습니다.")
}

func (s *Service) UpdateUser(ctx context.Context, identity security.Identity, userID int64, request model.UserRequest) (model.UserResponse, error) {
	if err := validateUserRequest(request); err != nil {
		return model.UserResponse{}, err
	}
	actor, err := s.store.UserByID(ctx, identity.UserID)
	if err != nil {
		return model.UserResponse{}, translateStoreError(err, "유저를 찾을 수 없습니다.")
	}
	if actor.ID != userID && actor.Role != model.RoleAdmin {
		return model.UserResponse{}, &httperr.HTTPError{Status: http.StatusForbidden, Message: "다른 유저의 정보를 수정할 수 없습니다."}
	}
	target, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return model.UserResponse{}, translateStoreError(err, "유저를 찾을 수 없습니다.")
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(request.OriginalPassword), bcrypt.DefaultCost)
	if err != nil {
		return model.UserResponse{}, internalError(err)
	}
	target.Username = strings.TrimSpace(request.Username)
	target.Email = strings.TrimSpace(request.Email)
	target.HashedPassword = string(hashedPassword)
	target.ProfileImageURL = strings.TrimSpace(request.ProfileImageURL)
	target.Bio = strings.TrimSpace(request.Bio)
	updated, err := s.store.UpdateUser(ctx, target)
	if err != nil {
		return model.UserResponse{}, translateStoreError(err, "유저를 찾을 수 없습니다.")
	}
	response := model.UserView(updated)
	response.Role = updated.Role
	return response, nil
}

func (s *Service) CreateCategory(ctx context.Context, identity security.Identity, name string) error {
	if _, err := s.requireRole(ctx, identity, model.RoleAdmin); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 50 {
		return httperr.BadRequest("카테고리 이름은 1자 이상 50자 이하여야 합니다.")
	}
	return translateStoreError(s.store.CreateCategory(ctx, name), "카테고리를 찾을 수 없습니다.")
}

func (s *Service) ListCategories(ctx context.Context) ([]model.Category, error) {
	categories, err := s.store.ListCategories(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	return categories, nil
}

func (s *Service) CreatePost(ctx context.Context, identity security.Identity, categoryID int64, request model.PostRequest) error {
	actor, err := s.requireRole(ctx, identity, model.RoleAuthor, model.RoleAdmin)
	if err != nil {
		return err
	}
	if err := validatePostRequest(request); err != nil {
		return err
	}
	err = s.store.CreatePost(ctx, store.CreatePostParams{
		Request: request, CategoryID: categoryID, Username: actor.Username, Slug: Slugify(request.Title),
	})
	return translateStoreError(err, "카테고리 또는 유저를 찾을 수 없습니다.")
}

func (s *Service) UpdatePost(ctx context.Context, identity security.Identity, postID, categoryID int64, request model.PostRequest) (model.PostEditResponse, error) {
	if err := s.authorizePost(ctx, identity, postID); err != nil {
		return model.PostEditResponse{}, err
	}
	if err := validatePostRequest(request); err != nil {
		return model.PostEditResponse{}, err
	}
	slug, err := s.store.UpdatePost(ctx, store.UpdatePostParams{
		PostID: postID, CategoryID: categoryID, Request: request, Slug: Slugify(request.Title),
	})
	if err != nil {
		return model.PostEditResponse{}, translateStoreError(err, "포스트 또는 카테고리를 찾을 수 없습니다.")
	}
	return model.PostEditResponse{Slug: slug}, nil
}

func (s *Service) PostByID(ctx context.Context, id int64) (model.PostResponse, error) {
	post, err := s.store.PostByID(ctx, id)
	if err != nil {
		return model.PostResponse{}, translateStoreError(err, "포스트를 찾을 수 없습니다.")
	}
	return model.PostView(post), nil
}

func (s *Service) PostBySlug(ctx context.Context, slug string) (model.PostResponse, error) {
	post, err := s.store.PostBySlug(ctx, slug)
	if err != nil {
		return model.PostResponse{}, translateStoreError(err, "포스트를 찾을 수 없습니다.")
	}
	return model.PostView(post), nil
}

func (s *Service) ListPosts(ctx context.Context, categoryName string, page, size int) (model.PageResponse[model.PostResponse], error) {
	page, size = normalizePage(page, size)
	posts, total, err := s.store.ListPosts(ctx, strings.TrimSpace(categoryName), page, size)
	if err != nil {
		return model.PageResponse[model.PostResponse]{}, internalError(err)
	}
	content := make([]model.PostResponse, 0, len(posts))
	for _, post := range posts {
		content = append(content, model.PostView(post))
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(size) - 1) / int64(size))
	}
	return model.PageResponse[model.PostResponse]{
		Content: content, Number: page, Size: size, TotalElements: total,
		TotalPages: totalPages, First: page == 0, Last: totalPages == 0 || page >= totalPages-1,
		Empty: len(content) == 0,
	}, nil
}

func (s *Service) SearchPosts(ctx context.Context, keyword string, page, size int) (model.PageResponse[model.PostResponse], error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return model.PageResponse[model.PostResponse]{}, httperr.BadRequest("검색어가 필요합니다.")
	}
	page, size = normalizePage(page, size)
	posts, total, err := s.store.SearchPosts(ctx, keyword, page, size)
	if err != nil {
		return model.PageResponse[model.PostResponse]{}, internalError(err)
	}
	content := make([]model.PostResponse, 0, len(posts))
	for _, post := range posts {
		content = append(content, model.PostView(post))
	}
	pages := int((total + int64(size) - 1) / int64(size))
	return model.PageResponse[model.PostResponse]{Content: content, Number: page, Size: size, TotalElements: total, TotalPages: pages, First: page == 0, Last: pages == 0 || page >= pages-1, Empty: len(content) == 0}, nil
}

// 권한은 토큰의 이름이 아니라 DB의 현재 역할과 불변 사용자 ID로 판단합니다.
func (s *Service) requireRole(ctx context.Context, identity security.Identity, roles ...model.Role) (model.User, error) {
	if identity.UserID <= 0 {
		return model.User{}, httperr.Unauthorized("로그인이 필요합니다.")
	}
	actor, err := s.store.UserByID(ctx, identity.UserID)
	if err != nil {
		return model.User{}, translateStoreError(err, "사용자를 찾을 수 없습니다.")
	}
	for _, role := range roles {
		if actor.Role == role {
			return actor, nil
		}
	}
	return model.User{}, &httperr.HTTPError{Status: http.StatusForbidden, Message: "권한이 없습니다."}
}

func (s *Service) authorizePost(ctx context.Context, identity security.Identity, postID int64) error {
	if identity.UserID <= 0 {
		return httperr.Unauthorized("로그인이 필요합니다.")
	}
	actor, err := s.store.UserByID(ctx, identity.UserID)
	if err != nil {
		return translateStoreError(err, "사용자를 찾을 수 없습니다.")
	}
	post, err := s.store.PostByID(ctx, postID)
	if err != nil {
		return translateStoreError(err, "포스트를 찾을 수 없습니다.")
	}
	if actor.Role != model.RoleAdmin && actor.ID != post.Author.ID {
		return &httperr.HTTPError{Status: http.StatusForbidden, Message: "작성자 또는 관리자만 변경할 수 있습니다."}
	}
	return nil
}

func (s *Service) CreatePrivilegedUser(ctx context.Context, identity security.Identity, request model.UserRequest, role model.Role) error {
	if _, err := s.requireRole(ctx, identity, model.RoleAdmin); err != nil {
		return err
	}
	return s.CreateUser(ctx, request, role)
}

func (s *Service) DeletePost(ctx context.Context, identity security.Identity, postID int64) error {
	if err := s.authorizePost(ctx, identity, postID); err != nil {
		return err
	}
	return translateStoreError(s.store.DeletePost(ctx, postID), "포스트를 찾을 수 없습니다.")
}

func (s *Service) IncrementViewCount(ctx context.Context, postID int64) error {
	return translateStoreError(s.store.IncrementViewCount(ctx, postID), "포스트를 찾을 수 없습니다.")
}

func (s *Service) LikePost(ctx context.Context, postID, userID int64) error {
	err := s.store.AddLike(ctx, postID, userID)
	if errors.Is(err, store.ErrConflict) {
		return &httperr.HTTPError{Status: http.StatusConflict, Message: "이미 좋아요를 눌렀습니다."}
	}
	return translateStoreError(err, "포스트 또는 유저를 찾을 수 없습니다.")
}

func (s *Service) UnlikePost(ctx context.Context, postID, userID int64) error {
	return translateStoreError(s.store.RemoveLike(ctx, postID, userID), "좋아요를 찾을 수 없습니다.")
}

func (s *Service) UsersWhoLiked(ctx context.Context, postID int64) ([]model.UserResponse, error) {
	users, err := s.store.UsersWhoLiked(ctx, postID)
	if err != nil {
		return nil, internalError(err)
	}
	result := make([]model.UserResponse, 0, len(users))
	for _, user := range users {
		result = append(result, model.UserView(user))
	}
	return result, nil
}

func (s *Service) UploadImage(ctx context.Context, files multipart.UploadedFiles) (model.ImageUploadResponse, error) {
	if len(files.Files) != 1 {
		return model.ImageUploadResponse{}, httperr.BadRequest("이미지 파일 하나를 업로드해야 합니다.")
	}
	metadata, err := s.images.Store(files.Files[0])
	if err != nil {
		return model.ImageUploadResponse{}, httperr.BadRequest(err.Error())
	}
	saved, err := s.store.SaveImageMetadata(ctx, metadata)
	if err != nil {
		s.images.Remove(metadata.StoredName)
		return model.ImageUploadResponse{}, translateStoreError(err, "이미지 메타데이터를 저장할 수 없습니다.")
	}
	return model.ImageUploadResponse{SavedID: saved.ID, URL: "/api/v1/image/" + saved.PublicID}, nil
}

func (s *Service) LoadImage(ctx context.Context, publicID string) ([]byte, string, error) {
	metadata, err := s.store.ImageByPublicID(ctx, publicID)
	if err != nil {
		return nil, "", translateStoreError(err, "이미지 메타데이터를 찾을 수 없습니다.")
	}
	data, err := s.images.Load(metadata.StoredName)
	if err != nil {
		return nil, "", internalError(err)
	}
	return data, metadata.ContentType, nil
}

func Slugify(title string) string {
	return whitespace.ReplaceAllString(strings.TrimSpace(title), "-")
}

func normalizePage(page, size int) (int, int) {
	if page < 0 {
		page = 0
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

func validateUserRequest(request model.UserRequest) error {
	if strings.TrimSpace(request.Username) == "" || len(request.Username) > 30 {
		return httperr.BadRequest("유저 이름은 1자 이상 30자 이하여야 합니다.")
	}
	if strings.TrimSpace(request.Email) == "" || !strings.Contains(request.Email, "@") || len(request.Email) > 100 {
		return httperr.BadRequest("이메일 형식이 올바르지 않습니다.")
	}
	if len(request.OriginalPassword) < 8 {
		return httperr.BadRequest("패스워드는 8자 이상이어야 합니다.")
	}
	return nil
}

func validatePostRequest(request model.PostRequest) error {
	if strings.TrimSpace(request.Title) == "" || len(request.Title) > 200 {
		return httperr.BadRequest("포스트 제목은 1자 이상 200자 이하여야 합니다.")
	}
	if strings.TrimSpace(request.Content) == "" {
		return httperr.BadRequest("포스트 내용이 필요합니다.")
	}
	return nil
}

func translateStoreError(err error, notFoundMessage string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return httperr.NotFound(notFoundMessage)
	case errors.Is(err, store.ErrConflict):
		return &httperr.HTTPError{Status: http.StatusConflict, Message: "이미 존재하는 값입니다."}
	default:
		return internalError(err)
	}
}

func internalError(err error) error {
	return &httperr.HTTPError{Status: http.StatusInternalServerError, Message: "요청을 처리할 수 없습니다.", Cause: fmt.Errorf("내부 처리 실패: %w", err)}
}
