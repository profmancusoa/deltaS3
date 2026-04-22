// Package s3store wraps the AWS SDK client with the small set of operations
// deltaS3 needs to query and upload objects.
package s3store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type Client struct {
	api *s3.Client
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	loadOptions := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
		// deltaS3 uses explicit static credentials from its JSON config instead of
		// relying on ambient AWS profiles or environment variables.
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.Credentials.AccessKeyID,
			cfg.Credentials.SecretAccessKey,
			cfg.Credentials.SessionToken,
		)),
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// Path-style mode is useful for S3-compatible endpoints that do not support
		// bucket names as DNS subdomains.
		o.UsePathStyle = cfg.ForcePathStyle
	})

	return &Client{api: client}, nil
}

func (c *Client) ObjectExists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := c.api.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	if isNotFound(err) {
		return false, nil
	}

	return false, fmt.Errorf("head object %s: %w", key, err)
}

func (c *Client) PutBytes(ctx context.Context, bucket, key, contentType string, payload []byte) error {
	_, err := c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(payload),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}

	return nil
}

func (c *Client) PutFile(ctx context.Context, bucket, key, contentType, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	_, err = c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object %s: %w", key, err)
	}

	return nil
}

func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		// Different S3-compatible providers report missing objects with slightly
		// different codes, so normalize the common ones here.
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey", "404":
			return true
		}
	}

	return false
}
