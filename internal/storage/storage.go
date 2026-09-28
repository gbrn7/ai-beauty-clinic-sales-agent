package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/raygbrn/ai-sales-agent-go/internal/config"
)

type StorageService interface {
	SaveMedia(phone string, data []byte, mime string) (string, error)
	GetMedia(fname string) ([]byte, string, error)
}

// Storage is an alias for StorageService to maintain compatibility.
type Storage = StorageService

type s3ClientAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type storageImpl struct {
	cfg      *config.Config
	mediaDir string
	s3Client s3ClientAPI
}

var extMap = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

func NewStorageService(cfg *config.Config, mediaDir string) StorageService {
	_ = os.MkdirAll(mediaDir, 0755)

	var client s3ClientAPI
	if cfg != nil && cfg.IsR2Enabled() {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL: cfg.R2Endpoint,
			}, nil
		})
		awsCfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
			awsconfig.WithEndpointResolverWithOptions(customResolver),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2SecretAccessKey, "")),
			awsconfig.WithRegion("auto"),
		)
		if err == nil {
			client = s3.NewFromConfig(awsCfg)
		}
	}

	return &storageImpl{
		cfg:      cfg,
		mediaDir: mediaDir,
		s3Client: client,
	}
}

func (s *storageImpl) randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *storageImpl) SaveMedia(phone string, data []byte, mime string) (string, error) {
	ext := extMap[mime]
	if ext == "" {
		ext = "jpg"
	}
	fname := fmt.Sprintf("%s_%s.%s", phone, s.randomHex(4), ext)

	if s.s3Client != nil {
		bucket := ""
		if s.cfg != nil {
			bucket = s.cfg.R2Bucket
		}
		_, err := s.s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(fname),
			Body:        bytes.NewReader(data),
			ContentType: aws.String(mime),
		})
		if err != nil {
			return "", err
		}
	} else {
		target := filepath.Join(s.mediaDir, fname)
		if err := os.WriteFile(target, data, 0644); err != nil {
			return "", err
		}
	}

	return fmt.Sprintf("media/%s", fname), nil
}

func (s *storageImpl) GetMedia(fname string) ([]byte, string, error) {
	fname = filepath.Base(fname) // prevent directory traversal

	if s.s3Client != nil {
		bucket := ""
		if s.cfg != nil {
			bucket = s.cfg.R2Bucket
		}
		out, err := s.s3Client.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fname),
		})
		if err != nil {
			return nil, "", err
		}
		defer out.Body.Close()
		content, err := io.ReadAll(out.Body)
		if err != nil {
			return nil, "", err
		}
		mime := "image/jpeg"
		if out.ContentType != nil && *out.ContentType != "" {
			mime = *out.ContentType
		}
		return content, mime, nil
	}

	target := filepath.Join(s.mediaDir, fname)
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, "", err
	}
	mime := http.DetectContentType(data)
	return data, mime, nil
}
