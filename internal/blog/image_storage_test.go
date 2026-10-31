package blog

import (
	"bytes"
	"io"
	"testing"

	"github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
)

func TestImageStorageUsesDetectedTypeAndGeneratedName(t *testing.T) {
	storage, err := NewImageStorage(config.Config{ImageUploadDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	metadata, err := storage.Store(multipart.UploadedFile{
		FieldName: "file", Filename: "../../avatar.exe", ContentType: "application/octet-stream", Size: int64(len(png)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(png)), nil },
	})
	if err != nil {
		t.Fatalf("PNG 저장 실패: %v", err)
	}
	if metadata.ContentType != "image/png" || metadata.OriginalName != "avatar.exe" {
		t.Fatalf("서버 검증 메타데이터가 올바르지 않습니다: %+v", metadata)
	}
	loaded, err := storage.Load(metadata.StoredName)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, png) {
		t.Fatal("저장한 이미지와 읽은 이미지가 다릅니다")
	}
}

func TestImageStorageRejectsNonImage(t *testing.T) {
	storage, err := NewImageStorage(config.Config{ImageUploadDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("plain text")
	_, err = storage.Store(multipart.UploadedFile{
		FieldName: "file", Filename: "note.png", ContentType: "image/png", Size: int64(len(data)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	})
	if err == nil {
		t.Fatal("내용이 이미지가 아닌 파일을 허용했습니다")
	}
}
