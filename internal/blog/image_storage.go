package blog

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/NARUBROWN/spine/pkg/multipart"
	"github.com/NARUBROWN/tech-blog-spine/internal/config"
	"github.com/NARUBROWN/tech-blog-spine/internal/model"
)

const maxImageSize = 20 << 20

type ImageStorage struct {
	root string
}

func NewImageStorage(config config.Config) (*ImageStorage, error) {
	root, err := filepath.Abs(config.ImageUploadDir)
	if err != nil {
		return nil, fmt.Errorf("이미지 경로 확인 실패: %w", err)
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("이미지 디렉터리 생성 실패: %w", err)
	}
	return &ImageStorage{root: root}, nil
}

func (s *ImageStorage) Store(file multipart.UploadedFile) (model.ImageMetadata, error) {
	if file.FieldName != "file" {
		return model.ImageMetadata{}, fmt.Errorf("file 필드가 필요합니다")
	}
	if file.Size <= 0 {
		return model.ImageMetadata{}, fmt.Errorf("빈 파일은 업로드할 수 없습니다")
	}
	if file.Size > maxImageSize {
		return model.ImageMetadata{}, fmt.Errorf("이미지는 20MB 이하여야 합니다")
	}
	source, err := file.Open()
	if err != nil {
		return model.ImageMetadata{}, fmt.Errorf("업로드 파일 열기 실패: %w", err)
	}
	defer source.Close()

	header := make([]byte, 512)
	read, readErr := io.ReadFull(source, header)
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return model.ImageMetadata{}, fmt.Errorf("이미지 읽기 실패: %w", readErr)
	}
	header = header[:read]
	contentType := http.DetectContentType(header)
	extension, ok := map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp",
	}[contentType]
	if !ok {
		return model.ImageMetadata{}, fmt.Errorf("PNG, JPEG, WebP 이미지만 업로드할 수 있습니다")
	}
	publicID, err := randomUUID()
	if err != nil {
		return model.ImageMetadata{}, err
	}
	storedName := publicID + extension
	targetPath := filepath.Join(s.root, storedName)
	target, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return model.ImageMetadata{}, fmt.Errorf("이미지 파일 생성 실패: %w", err)
	}
	written, copyErr := io.Copy(target, io.LimitReader(io.MultiReader(bytes.NewReader(header), source), maxImageSize+1))
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil || written > maxImageSize {
		_ = os.Remove(targetPath)
		if written > maxImageSize {
			return model.ImageMetadata{}, fmt.Errorf("이미지는 20MB 이하여야 합니다")
		}
		return model.ImageMetadata{}, fmt.Errorf("이미지 저장 실패: %w", errorsJoin(copyErr, closeErr))
	}
	return model.ImageMetadata{
		PublicID: publicID, StoredName: storedName, OriginalName: filepath.Base(file.Filename),
		ContentType: contentType, Size: written,
	}, nil
}

func (s *ImageStorage) Load(storedName string) ([]byte, error) {
	if storedName == "" || filepath.Base(storedName) != storedName || strings.ContainsAny(storedName, `/\\`) {
		return nil, fmt.Errorf("저장된 이미지 이름이 올바르지 않습니다")
	}
	return os.ReadFile(filepath.Join(s.root, storedName))
}

func (s *ImageStorage) Remove(storedName string) {
	if filepath.Base(storedName) == storedName && !strings.ContainsAny(storedName, `/\\`) {
		_ = os.Remove(filepath.Join(s.root, storedName))
	}
}

func randomUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("이미지 식별자 생성 실패: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

func errorsJoin(first, second error) error {
	if first != nil {
		return first
	}
	return second
}
