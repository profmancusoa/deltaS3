# Changelog

All notable changes to this project will be documented in this file.

## [0.1.0] - 2026-04-22

### Added
- Initial `deltaS3` CLI with `chunk`, `push`, `restore`, and `version` commands
- Fixed-size chunking of local files with `BLAKE3` content hashing
- Manifest generation for complete logical artifacts
- Local restore with per-chunk and full-artifact integrity verification
- S3 push workflow for chunk objects and versioned/latest manifests
- External JSON configuration for S3 credentials and connection settings
- Terminal progress reporting for `chunk` and `push`
- Build and local smoke-test workflow through `Makefile`
- Project documentation for architecture, configuration, and usage
