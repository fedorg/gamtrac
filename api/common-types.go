
package api

import (
	"time"
	"os"
	"fmt"
	"encoding/hex"
)

type HashDigest struct {
	Algorithm string
	Value     []byte
}

func (h HashDigest) String() string {
	return fmt.Sprintf("%s:%s", h.Algorithm, hex.EncodeToString(h.Value))
	// return fmt.Sprintf(base64.StdEncoding.EncodeToString(h.Value))
}

type FileError struct {
	// Filename  string
	Error     error
	CreatedAt time.Time
}

type AnnotResult struct {
	Path        string             // required
	Size        int64              // required
	Mode        os.FileMode        // required
	ModTime     time.Time          // required
	QueuedAt    time.Time          // required
	ProcessedAt time.Time          // required
	IsDir       bool               // required
	OwnerUID    *string            // optional
	Hash        *HashDigest        // optional
	Pattern     *string            // optional
	Parsed      *map[string]string // optional
	Errors      []FileError        // required
}

func NewAnnotResult(
	Path string,
	Size int64,
	Mode os.FileMode,
	ModTime time.Time,
	QueuedAt time.Time,
	ProcessedAt time.Time,
	IsDir bool,
	OwnerUID *string,
	Hash *HashDigest,
	Pattern *string,
	Parsed *map[string]string,
	Errors []FileError,
) AnnotResult {
	return AnnotResult{
		Path:        Path,
		Size:        Size,
		Mode:        Mode,
		ModTime:     ModTime,
		QueuedAt:    QueuedAt,
		ProcessedAt: ProcessedAt,
		IsDir:       IsDir,
		OwnerUID:    OwnerUID,
		Hash:        Hash,
		Pattern:     Pattern,
		Parsed:      Parsed,
		Errors:      Errors,
	}
}
