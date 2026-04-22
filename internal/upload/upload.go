// Package upload publishes a locally generated manifest and its referenced
// chunks to the target S3 bucket layout used by deltaS3.
package upload

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"deltas3/internal/manifest"
	"deltas3/internal/progress"
	s3store "deltas3/internal/store/s3"
)

const (
	manifestContentType = "application/json"
	chunkContentType    = "application/octet-stream"
)

type Config struct {
	ManifestPath string
	ChunksDir    string
	S3ConfigPath string
}

type Result struct {
	Bucket               string
	Prefix               string
	ChunksUploaded       int
	ChunksReused         int
	VersionedManifestKey string
	LatestManifestKey    string
}

func (c Config) Validate() error {
	if c.ManifestPath == "" {
		return fmt.Errorf("manifest is required")
	}
	if c.ChunksDir == "" {
		return fmt.Errorf("chunks-dir is required")
	}
	if c.S3ConfigPath == "" {
		return fmt.Errorf("s3-config is required")
	}

	return nil
}

func Run(cfg Config) (Result, error) {
	doc, err := manifest.LoadFile(cfg.ManifestPath)
	if err != nil {
		return Result{}, fmt.Errorf("load manifest: %w", err)
	}
	if doc.Bucket == "" {
		return Result{}, fmt.Errorf("manifest bucket is required for upload")
	}

	s3cfg, err := s3store.LoadConfig(cfg.S3ConfigPath)
	if err != nil {
		return Result{}, fmt.Errorf("load s3 config: %w", err)
	}

	client, err := s3store.New(context.Background(), s3cfg)
	if err != nil {
		return Result{}, fmt.Errorf("create s3 client: %w", err)
	}

	// Upload decisions are based on unique chunk hashes because a manifest can
	// reference the same chunk content multiple times.
	uniqueChunks := uniqueChunks(doc.Chunks)
	reporter := progress.New("push", totalChunkBytes(uniqueChunks), os.Stderr)

	var uploaded, reused int
	var processedBytes int64
	for index, chunk := range uniqueChunks {
		chunkPath := filepath.Join(cfg.ChunksDir, chunk.Hash.Value)
		if _, err := os.Stat(chunkPath); err != nil {
			return Result{}, fmt.Errorf("stat chunk %s: %w", chunk.Hash.Value, err)
		}

		exists, err := client.ObjectExists(context.Background(), doc.Bucket, chunk.ObjectKey)
		if err != nil {
			return Result{}, err
		}
		if exists {
			reused++
		} else {
			if err := client.PutFile(context.Background(), doc.Bucket, chunk.ObjectKey, chunkContentType, chunkPath); err != nil {
				return Result{}, err
			}
			uploaded++
		}

		processedBytes += chunk.SizeBytes
		reporter.Update(
			processedBytes,
			fmt.Sprintf("chunks %d/%d | uploaded %d | reused %d", index+1, len(uniqueChunks), uploaded, reused),
		)
	}

	payload, err := os.ReadFile(cfg.ManifestPath)
	if err != nil {
		return Result{}, fmt.Errorf("read manifest payload: %w", err)
	}

	// Publish the immutable manifest first, then update latest.json to point to
	// the newest successful backup state.
	versionedKey := manifestObjectKey(doc.Prefix, doc.BackupID+".json")
	if err := client.PutBytes(context.Background(), doc.Bucket, versionedKey, manifestContentType, payload); err != nil {
		return Result{}, err
	}

	latestKey := manifestObjectKey(doc.Prefix, "latest.json")
	if err := client.PutBytes(context.Background(), doc.Bucket, latestKey, manifestContentType, payload); err != nil {
		return Result{}, err
	}

	reporter.Finish(
		totalChunkBytes(uniqueChunks),
		fmt.Sprintf("chunks %d/%d | uploaded %d | reused %d", len(uniqueChunks), len(uniqueChunks), uploaded, reused),
	)

	return Result{
		Bucket:               doc.Bucket,
		Prefix:               doc.Prefix,
		ChunksUploaded:       uploaded,
		ChunksReused:         reused,
		VersionedManifestKey: versionedKey,
		LatestManifestKey:    latestKey,
	}, nil
}

func manifestObjectKey(prefix, name string) string {
	cleanPrefix := strings.Trim(prefix, "/")
	if cleanPrefix == "" {
		return path.Join("manifests", name)
	}

	return path.Join(cleanPrefix, "manifests", name)
}

func uniqueChunks(chunks []manifest.Chunk) []manifest.Chunk {
	seen := make(map[string]struct{}, len(chunks))
	unique := make([]manifest.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		// The content hash is the canonical identity of a chunk, so duplicate
		// references inside the same manifest only need one remote existence check.
		if _, ok := seen[chunk.Hash.Value]; ok {
			continue
		}
		seen[chunk.Hash.Value] = struct{}{}
		unique = append(unique, chunk)
	}

	return unique
}

func totalChunkBytes(chunks []manifest.Chunk) int64 {
	var total int64
	for _, chunk := range chunks {
		total += chunk.SizeBytes
	}

	return total
}
