// Package manifest defines the canonical backup metadata format used to
// describe artifacts, chunk layout, and validation rules across the system.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

const (
	SchemaVersion        = 1
	DefaultHashAlgorithm = "blake3"
	ChunkingFixed        = "fixed"
	DefaultManifest      = "manifest.json"
)

type Hash struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type HashAlgorithm struct {
	Algorithm string `json:"algorithm"`
}

type Artifact struct {
	LogicalName string `json:"logical_name"`
	SizeBytes   int64  `json:"size_bytes"`
	Hash        Hash   `json:"hash"`
}

type Chunking struct {
	Strategy       string        `json:"strategy"`
	ChunkSizeBytes int64         `json:"chunk_size_bytes"`
	Hash           HashAlgorithm `json:"hash"`
}

type Chunk struct {
	Index       int64  `json:"index"`
	OffsetBytes int64  `json:"offset_bytes"`
	SizeBytes   int64  `json:"size_bytes"`
	Hash        Hash   `json:"hash"`
	ObjectKey   string `json:"object_key"`
}

type Manifest struct {
	SchemaVersion int               `json:"schema_version"`
	BackupID      string            `json:"backup_id"`
	CreatedAt     string            `json:"created_at"`
	Bucket        string            `json:"bucket"`
	Prefix        string            `json:"prefix"`
	Artifact      Artifact          `json:"artifact"`
	Chunking      Chunking          `json:"chunking"`
	Chunks        []Chunk           `json:"chunks"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

func (m Manifest) Marshal() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	return json.MarshalIndent(m, "", "  ")
}

func LoadFile(path string) (Manifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	var doc Manifest
	if err := json.Unmarshal(payload, &doc); err != nil {
		return Manifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate manifest: %w", err)
	}

	return doc, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", m.SchemaVersion)
	}
	if m.BackupID == "" {
		return errors.New("backup_id is required")
	}
	if m.CreatedAt == "" {
		return errors.New("created_at is required")
	}
	if m.Artifact.LogicalName == "" {
		return errors.New("artifact.logical_name is required")
	}
	if m.Artifact.SizeBytes < 0 {
		return errors.New("artifact.size_bytes must be >= 0")
	}
	if err := validateHash("artifact.hash", m.Artifact.Hash); err != nil {
		return err
	}
	if m.Chunking.Strategy != ChunkingFixed {
		return fmt.Errorf("unsupported chunking.strategy %q", m.Chunking.Strategy)
	}
	if m.Chunking.ChunkSizeBytes <= 0 {
		return errors.New("chunking.chunk_size_bytes must be > 0")
	}
	if err := validateHashAlgorithm("chunking.hash", m.Chunking.Hash); err != nil {
		return err
	}
	if m.Artifact.SizeBytes == 0 && len(m.Chunks) != 0 {
		return errors.New("chunks must be empty for zero-sized artifacts")
	}
	if m.Artifact.SizeBytes > 0 && len(m.Chunks) == 0 {
		return errors.New("chunks must not be empty for non-zero artifact")
	}

	var expectedOffset int64
	for i, chunk := range m.Chunks {
		// Chunks must describe a complete file layout with no gaps or overlap so
		// restore can stream them back in order without extra metadata.
		if chunk.Index != int64(i) {
			return fmt.Errorf("chunk %d has unexpected index %d", i, chunk.Index)
		}
		if chunk.OffsetBytes != expectedOffset {
			return fmt.Errorf("chunk %d has unexpected offset %d", i, chunk.OffsetBytes)
		}
		if chunk.SizeBytes <= 0 {
			return fmt.Errorf("chunk %d size must be > 0", i)
		}
		if chunk.SizeBytes > m.Chunking.ChunkSizeBytes {
			return fmt.Errorf("chunk %d exceeds configured chunk size", i)
		}
		if err := validateHash(fmt.Sprintf("chunks[%d].hash", i), chunk.Hash); err != nil {
			return err
		}
		if chunk.ObjectKey == "" {
			return fmt.Errorf("chunks[%d].object_key is required", i)
		}
		if err := validateObjectKey(m.Prefix, chunk.ObjectKey); err != nil {
			return fmt.Errorf("chunks[%d].object_key invalid: %w", i, err)
		}
		expectedOffset += chunk.SizeBytes
	}

	if expectedOffset != m.Artifact.SizeBytes {
		return fmt.Errorf("chunk sizes sum to %d, want %d", expectedOffset, m.Artifact.SizeBytes)
	}

	return nil
}

func validateHash(field string, h Hash) error {
	if err := validateHashAlgorithm(field, HashAlgorithm{Algorithm: h.Algorithm}); err != nil {
		return err
	}
	if h.Value == "" {
		return fmt.Errorf("%s.value is required", field)
	}
	return nil
}

func validateHashAlgorithm(field string, h HashAlgorithm) error {
	if h.Algorithm != DefaultHashAlgorithm {
		return fmt.Errorf("%s.algorithm must be %q", field, DefaultHashAlgorithm)
	}

	return nil
}

func validateObjectKey(prefix, objectKey string) error {
	cleanPrefix := strings.Trim(prefix, "/")
	expectedPrefix := "chunks/"
	if cleanPrefix != "" {
		expectedPrefix = cleanPrefix + "/chunks/"
	}

	// Restrict chunk references to the configured prefix so manifests cannot
	// accidentally point at unrelated objects in the same bucket.
	if !strings.HasPrefix(objectKey, expectedPrefix) {
		return fmt.Errorf("must start with %q", expectedPrefix)
	}

	base := path.Base(objectKey)
	if base == "." || base == "/" || base == "" {
		return errors.New("must end with a chunk hash")
	}

	return nil
}
