package blog

import (
	"context"
	"errors"
	"testing"

	"github.com/NARUBROWN/spine/pkg/httperr"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
	"github.com/NARUBROWN/tech-blog-spine/internal/security"
	"github.com/NARUBROWN/tech-blog-spine/internal/store"
)

type policyStore struct {
	store.Store
	actor      model.User
	post       model.Post
	writes     int
	page, size int
}

func (s *policyStore) UserByID(context.Context, int64) (model.User, error) { return s.actor, nil }
func (s *policyStore) PostByID(context.Context, int64) (model.Post, error) { return s.post, nil }
func (s *policyStore) UpdatePost(context.Context, store.UpdatePostParams) (string, error) {
	s.writes++
	return "updated", nil
}
func (s *policyStore) DeletePost(context.Context, int64) error { s.writes++; return nil }
func (s *policyStore) CreatePost(_ context.Context, p store.CreatePostParams) error {
	if p.Username != s.actor.Username {
		panic("stale owner")
	}
	s.writes++
	return nil
}
func (s *policyStore) CreateCategory(context.Context, string) error { s.writes++; return nil }
func (s *policyStore) CreateUser(context.Context, model.User) error { s.writes++; return nil }
func (s *policyStore) UpdateUser(_ context.Context, u model.User) (model.User, error) {
	s.writes++
	return u, nil
}
func (s *policyStore) SearchPosts(_ context.Context, _ string, page, size int) ([]model.Post, int64, error) {
	s.page = page
	s.size = size
	return []model.Post{{ID: 3}}, 25, nil
}
func status(err error) int {
	var h *httperr.HTTPError
	if errors.As(err, &h) {
		return h.Status
	}
	if err != nil {
		return 500
	}
	return 200
}

func TestPostMutationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		role      model.Role
		id, owner int64
		want      int
	}{
		{"owner", model.RoleAuthor, 7, 7, 200}, {"admin", model.RoleAdmin, 8, 7, 200},
		{"other author", model.RoleAuthor, 8, 7, 403}, {"other user", model.RoleUser, 8, 7, 403},
		{"guest", model.Role("ROLE_GUEST"), 8, 7, 403}, {"anonymous", model.RoleUser, 0, 7, 401},
	} {
		for _, op := range []string{"update", "delete"} {
			t.Run(tc.name+op, func(t *testing.T) {
				db := &policyStore{actor: model.User{ID: tc.id, Username: "current", Role: tc.role}, post: model.Post{Author: model.User{ID: tc.owner, Username: "forged"}}}
				svc := &Service{store: db}
				identity := security.Identity{UserID: tc.id, Username: "forged"}
				var err error
				if op == "update" {
					_, err = svc.UpdatePost(context.Background(), identity, 1, 1, model.PostRequest{Title: "title", Content: "body"})
				} else {
					err = svc.DeletePost(context.Background(), identity, 1)
				}
				if status(err) != tc.want {
					t.Fatalf("status=%d want=%d: %v", status(err), tc.want, err)
				}
				if (db.writes > 0) != (tc.want == 200) {
					t.Fatalf("unauthorized write count=%d", db.writes)
				}
			})
		}
	}
}
func TestRoleRestrictedOperations(t *testing.T) {
	for _, role := range []model.Role{model.RoleAdmin, model.RoleAuthor, model.RoleUser, model.Role("ROLE_GUEST")} {
		db := &policyStore{actor: model.User{ID: 7, Username: "current", Role: role}}
		svc := &Service{store: db}
		id := security.Identity{UserID: 7, Username: "stale"}
		err := svc.CreatePost(context.Background(), id, 1, model.PostRequest{Title: "t", Content: "b"})
		allowed := role == model.RoleAdmin || role == model.RoleAuthor
		if (err == nil) != allowed {
			t.Fatalf("publish role=%s: %v", role, err)
		}
		err = svc.CreateCategory(context.Background(), id, "category")
		if (err == nil) != (role == model.RoleAdmin) {
			t.Fatalf("category role=%s: %v", role, err)
		}
		before := db.writes
		err = svc.CreatePrivilegedUser(context.Background(), id, model.UserRequest{Username: "new", Email: "new@example.com", OriginalPassword: "test-password"}, model.RoleAdmin)
		if (err == nil) != (role == model.RoleAdmin) {
			t.Fatalf("provision role=%s: %v", role, err)
		}
		if role != model.RoleAdmin && db.writes != before {
			t.Fatal("privilege escalation write")
		}
	}
}
func TestUpdateProfileRetainsRole(t *testing.T) {
	db := &policyStore{actor: model.User{ID: 7, Role: model.RoleAdmin}}
	svc := &Service{store: db}
	response, err := svc.UpdateUser(context.Background(), security.Identity{UserID: 7}, 7, model.UserRequest{Username: "admin", Email: "admin@example.com", OriginalPassword: "test-password"})
	if err != nil || response.Role != model.RoleAdmin {
		t.Fatalf("lost role: %+v %v", response, err)
	}
	if model.UserView(db.actor).Role != "" {
		t.Fatal("public user view exposes role")
	}
}
func TestSearchPagination(t *testing.T) {
	db := &policyStore{}
	svc := &Service{store: db}
	page, err := svc.SearchPosts(context.Background(), "topic", 1, 12)
	if err != nil || db.page != 1 || db.size != 12 || page.TotalElements != 25 || page.TotalPages != 3 || page.First || page.Last || len(page.Content) != 1 {
		t.Fatalf("invalid page: %+v %v", page, err)
	}
}
