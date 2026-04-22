// Package backup turns a local file into content-addressed chunks plus a
// manifest that describes how to reconstruct the original artifact.
package backup

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zeebo/blake3"

	"deltas3/internal/manifest"
	"deltas3/internal/progress"
)

type Config struct {
	InputPath   string
	OutputDir   string
	ChunkSize   int64
	Bucket      string
	Prefix      string
	LogicalName string
}

type Result struct {
	ManifestPath  string
	ArtifactHash  string
	ChunksWritten int
	ChunksReused  int
}

func (c Config) Validate() error {
	if c.InputPath == "" {
		return fmt.Errorf("input is required")
	}
	if c.OutputDir == "" {
		return fmt.Errorf("output-dir is required")
	}
	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk-size must be > 0")
	}

	return nil
}

func Run(cfg Config) (Result, error) {
	inputInfo, err := os.Stat(cfg.InputPath)
	if err != nil {
		return Result{}, fmt.Errorf("stat input: %w", err)
	}
	if !inputInfo.Mode().IsRegular() {
		return Result{}, fmt.Errorf("input path must be a regular file")
	}

	if err := os.MkdirAll(filepath.Join(cfg.OutputDir, "chunks"), 0o755); err != nil {
		return Result{}, fmt.Errorf("create chunks dir: %w", err)
	}

	in, err := os.Open(cfg.InputPath)
	if err != nil {
		return Result{}, fmt.Errorf("open input: %w", err)
	}
	defer in.Close()

	artifactHasher := blake3.New()
	buffer := make([]byte, cfg.ChunkSize)
	chunks := make([]manifest.Chunk, 0)
	reporter := progress.New("chunk", inputInfo.Size(), os.Stderr)

	var (
		offset        int64
		index         int64
		chunksWritten int
		chunksReused  int
	)

	for {
		n, readErr := io.ReadFull(in, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return Result{}, fmt.Errorf("read input: %w", readErr)
		}
		if n == 0 {
			break
		}

		chunkBytes := buffer[:n]
		// Keep the artifact hash in lockstep with chunk processing so the final
		// manifest can validate the fully reconstructed file.
		if _, err := artifactHasher.Write(chunkBytes); err != nil {
			return Result{}, fmt.Errorf("hash artifact: %w", err)
		}

		chunkSum := blake3.Sum256(chunkBytes)
		chunkHash := hex.EncodeToString(chunkSum[:])
		chunkPath := filepath.Join(cfg.OutputDir, "chunks", chunkHash)

		// Reuse an existing local chunk file when the same content appears more
		// than once, either across runs or within the same artifact.
		if _, err := os.Stat(chunkPath); err == nil {
			chunksReused++
		} else if os.IsNotExist(err) {
			if err := os.WriteFile(chunkPath, chunkBytes, 0o644); err != nil {
				return Result{}, fmt.Errorf("write chunk %s: %w", chunkHash, err)
			}
			chunksWritten++
		} else {
			return Result{}, fmt.Errorf("stat chunk %s: %w", chunkHash, err)
		}

		chunks = append(chunks, manifest.Chunk{
			Index:       index,
			OffsetBytes: offset,
			SizeBytes:   int64(n),
			Hash: manifest.Hash{
				Algorithm: manifest.DefaultHashAlgorithm,
				Value:     chunkHash,
			},
			ObjectKey: objectKey(cfg.Prefix, chunkHash),
		})

		offset += int64(n)
		index++
		reporter.Update(
			offset,
			fmt.Sprintf("chunks %d | written %d | reused %d", index, chunksWritten, chunksReused),
		)

		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	artifactHash := hex.EncodeToString(artifactHasher.Sum(nil))
	now := time.Now().UTC()
	backupID := now.Format("2006-01-02T15-04-05Z")
	logicalName := cfg.LogicalName
	if logicalName == "" {
		logicalName = filepath.Base(cfg.InputPath)
	}

	doc := manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		BackupID:      backupID,
		CreatedAt:     now.Format(time.RFC3339),
		Bucket:        cfg.Bucket,
		Prefix:        strings.Trim(cfg.Prefix, "/"),
		Artifact: manifest.Artifact{
			LogicalName: logicalName,
			SizeBytes:   inputInfo.Size(),
			Hash: manifest.Hash{
				Algorithm: manifest.DefaultHashAlgorithm,
				Value:     artifactHash,
			},
		},
		Chunking: manifest.Chunking{
			Strategy:       manifest.ChunkingFixed,
			ChunkSizeBytes: cfg.ChunkSize,
			Hash: manifest.HashAlgorithm{
				Algorithm: manifest.DefaultHashAlgorithm,
			},
		},
		Chunks: chunks,
	}

	payload, err := doc.Marshal()
	if err != nil {
		return Result{}, fmt.Errorf("marshal manifest: %w", err)
	}

	manifestPath := filepath.Join(cfg.OutputDir, manifest.DefaultManifest)
	if err := os.WriteFile(manifestPath, append(payload, '\n'), 0o644); err != nil {
		return Result{}, fmt.Errorf("write manifest: %w", err)
	}

	reporter.Finish(
		inputInfo.Size(),
		fmt.Sprintf("chunks %d | written %d | reused %d", len(chunks), chunksWritten, chunksReused),
	)

	return Result{
		ManifestPath:  manifestPath,
		ArtifactHash:  artifactHash,
		ChunksWritten: chunksWritten,
		ChunksReused:  chunksReused,
	}, nil
}

func objectKey(prefix, hash string) string {
	cleanPrefix := strings.Trim(prefix, "/")
	if cleanPrefix == "" {
		return filepath.ToSlash(filepath.Join("chunks", hash))
	}

	// Store chunks under the logical prefix so manifests can be moved around
	// without rewriting per-chunk keys.
	return filepath.ToSlash(filepath.Join(cleanPrefix, "chunks", hash))
}
