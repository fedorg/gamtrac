
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
	Path        string             `diff:"Path,identifier"`// required
	MountDir    string             `diff:"-" json:"-"`
	Size        int64              `diff:"Size"`// required
	Mode        os.FileMode        `diff:"Mode"`// required
	ModTime     time.Time          `diff:"ModTime"`// required
	QueuedAt    time.Time          `diff:"-"`// required
	ProcessedAt time.Time          `diff:"-"`// required
	IsDir       bool               `diff:"IsDir"`// required
	OwnerUID    *string            `diff:"OwnerUID"`// optional
	Hash        *HashDigest        `diff:"Hash"`// optional
	Pattern     *string            `diff:"Pattern"`// optional
	Parsed      *map[string]string `diff:"Parsed"`// optional
	Errors      []FileError        `diff:"-" json:"-"`// required
}

func NewAnnotResult(
	Path string,
	MountDir string,
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
		MountDir: MountDir,
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
