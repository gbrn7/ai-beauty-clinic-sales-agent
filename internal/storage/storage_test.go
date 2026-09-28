package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/raygbrn/ai-sales-agent-go/internal/config"
)

type mockS3Client struct {
	putObjectFunc func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	getObjectFunc func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.putObjectFunc != nil {
		return m.putObjectFunc(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader([]byte{})),
	}, nil
}

func TestLocalStorageSaveAndGet(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "media_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.Config{} // R2 disabled
	svc := NewStorageService(cfg, tempDir)

	// Valid PNG header so DetectContentType detects image/png
	data := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4")
	path, err := svc.SaveMedia("628123", data, "image/png")
	if err != nil {
		t.Fatalf("failed to save media: %v", err)
	}

	if !strings.HasPrefix(path, "media/628123_") || !strings.HasSuffix(path, ".png") {
		t.Errorf("unexpected path format: %s", path)
	}

	fname := filepath.Base(path)
	gotBytes, mime, err := svc.GetMedia(fname)
	if err != nil {
		t.Fatalf("failed to get media: %v", err)
	}
	if !bytes.Equal(gotBytes, data) {
		t.Errorf("data mismatch")
	}
	if mime != "image/png" {
		t.Errorf("expected mime image/png, got %s", mime)
	}
}

func TestMimeTypeExtensions(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "media_mime_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	svc := NewStorageService(&config.Config{}, tempDir)

	tests := []struct {
		mime string
		ext  string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/webp", ".webp"},
		{"application/pdf", ".jpg"},
		{"unknown", ".jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			path, err := svc.SaveMedia("123", []byte("data"), tt.mime)
			if err != nil {
				t.Fatalf("SaveMedia failed: %v", err)
			}
			if !strings.HasSuffix(path, tt.ext) {
				t.Errorf("expected suffix %s for mime %s, got %s", tt.ext, tt.mime, path)
			}
		})
	}
}

func TestDirectoryTraversalProtection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "media_traversal_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	parentFile := filepath.Join(filepath.Dir(tempDir), "secret.txt")
	_ = os.WriteFile(parentFile, []byte("sensitive info"), 0644)
	defer os.Remove(parentFile)

	svc := NewStorageService(&config.Config{}, tempDir)

	// Attempting directory traversal should look for "secret.txt" inside tempDir, NOT parentDir
	_, _, err = svc.GetMedia("../secret.txt")
	if err == nil {
		t.Errorf("expected error when accessing file outside media dir, but got nil")
	}

	// If a file with the base name exists in mediaDir, it should find that one safely
	inDirFile := filepath.Join(tempDir, "secret.txt")
	_ = os.WriteFile(inDirFile, []byte("safe info"), 0644)
	data, _, err := svc.GetMedia("../secret.txt")
	if err != nil {
		t.Fatalf("failed to get media: %v", err)
	}
	if string(data) != "safe info" {
		t.Errorf("expected 'safe info', got '%s'", string(data))
	}
}

func TestLocalStorageGetMediaNotFound(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "media_not_found_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	svc := NewStorageService(&config.Config{}, tempDir)
	_, _, err = svc.GetMedia("nonexistent.png")
	if err == nil {
		t.Errorf("expected error for nonexistent file, got nil")
	}
}

func TestR2StorageSaveAndGet(t *testing.T) {
	cfg := &config.Config{
		R2Endpoint:        "https://test.r2.cloudflarestorage.com",
		R2AccessKeyID:     "access_key",
		R2SecretAccessKey: "secret_key",
		R2Bucket:          "my-bucket",
		R2PublicURL:       "https://pub.example.com",
	}

	var savedKey string
	var savedBucket string
	var savedBody []byte
	var savedContentType string

	mock := &mockS3Client{
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			savedBucket = *params.Bucket
			savedKey = *params.Key
			savedContentType = *params.ContentType
			b, _ := io.ReadAll(params.Body)
			savedBody = b
			return &s3.PutObjectOutput{}, nil
		},
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if *params.Bucket != "my-bucket" {
				return nil, fmt.Errorf("unexpected bucket: %s", *params.Bucket)
			}
			if *params.Key != savedKey {
				return nil, fmt.Errorf("unexpected key: %s", *params.Key)
			}
			return &s3.GetObjectOutput{
				Body:        io.NopCloser(bytes.NewReader(savedBody)),
				ContentType: aws.String(savedContentType),
			}, nil
		},
	}

	svc := &storageImpl{
		cfg:      cfg,
		mediaDir: "media",
		s3Client: mock,
	}

	testData := []byte("image binary data")
	path, err := svc.SaveMedia("0812345678", testData, "image/webp")
	if err != nil {
		t.Fatalf("SaveMedia failed: %v", err)
	}

	if !strings.HasPrefix(path, "media/0812345678_") || !strings.HasSuffix(path, ".webp") {
		t.Errorf("unexpected saved path: %s", path)
	}

	if savedBucket != "my-bucket" {
		t.Errorf("expected bucket my-bucket, got %s", savedBucket)
	}
	if savedContentType != "image/webp" {
		t.Errorf("expected content type image/webp, got %s", savedContentType)
	}
	if !bytes.Equal(savedBody, testData) {
		t.Errorf("saved body mismatch")
	}

	// Test GetMedia with full path or basename
	data, mime, err := svc.GetMedia(path)
	if err != nil {
		t.Fatalf("GetMedia failed: %v", err)
	}
	if !bytes.Equal(data, testData) {
		t.Errorf("retrieved data mismatch")
	}
	if mime != "image/webp" {
		t.Errorf("expected mime image/webp, got %s", mime)
	}
}

func TestR2StorageErrors(t *testing.T) {
	cfg := &config.Config{
		R2Bucket: "my-bucket",
	}

	mock := &mockS3Client{
		putObjectFunc: func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, errors.New("s3 put failed")
		},
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return nil, errors.New("s3 get failed")
		},
	}

	svc := &storageImpl{
		cfg:      cfg,
		mediaDir: "media",
		s3Client: mock,
	}

	_, err := svc.SaveMedia("123", []byte("data"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "s3 put failed") {
		t.Errorf("expected s3 put failed error, got %v", err)
	}

	_, _, err = svc.GetMedia("test.jpg")
	if err == nil || !strings.Contains(err.Error(), "s3 get failed") {
		t.Errorf("expected s3 get failed error, got %v", err)
	}
}

func TestR2StorageDefaultContentType(t *testing.T) {
	cfg := &config.Config{
		R2Bucket: "my-bucket",
	}

	mock := &mockS3Client{
		getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{
				Body:        io.NopCloser(bytes.NewReader([]byte("data"))),
				ContentType: nil, // ContentType not set in S3 response
			}, nil
		},
	}

	svc := &storageImpl{
		cfg:      cfg,
		mediaDir: "media",
		s3Client: mock,
	}

	data, mime, err := svc.GetMedia("test.jpg")
	if err != nil {
		t.Fatalf("GetMedia failed: %v", err)
	}
	if string(data) != "data" {
		t.Errorf("expected data, got %s", string(data))
	}
	if mime != "image/jpeg" {
		t.Errorf("expected default mime image/jpeg, got %s", mime)
	}
}

func TestNewStorageServiceWithR2Config(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "media_cfg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := &config.Config{
		R2Endpoint:        "https://test.r2.cloudflarestorage.com",
		R2AccessKeyID:     "access_key",
		R2SecretAccessKey: "secret_key",
		R2Bucket:          "my-bucket",
		R2PublicURL:       "https://pub.example.com",
	}

	svc := NewStorageService(cfg, tempDir)
	if svc == nil {
		t.Fatal("expected non-nil StorageService")
	}

	impl, ok := svc.(*storageImpl)
	if !ok {
		t.Fatal("expected *storageImpl type")
	}
	if impl.s3Client == nil {
		t.Error("expected s3Client to be initialized when R2 is enabled")
	}
}
