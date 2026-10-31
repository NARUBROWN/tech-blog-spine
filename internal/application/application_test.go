package application

import "testing"

func TestBuildApplicationIsValid(t *testing.T) {
	t.Setenv("MYSQL_DATABASE", "tech_blog")
	t.Setenv("MYSQL_USER", "test")
	t.Setenv("MYSQL_PASSWORD", "test-password")
	t.Setenv("JWT_SECRET", "test-secret-that-is-at-least-32-bytes-long")
	t.Setenv("IMAGE_UPLOAD_DIR", t.TempDir())
	app, options, err := BuildApplication()
	if err != nil {
		t.Fatalf("generated application registration is invalid: %v", err)
	}
	if err := app.Validate(options); err != nil {
		t.Fatalf("generated application is invalid: %v", err)
	}
}
