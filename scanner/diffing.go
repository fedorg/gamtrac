package scanner

import (
	"github.com/r3labs/diff"
	"strings"
)
// find missing and new file paths
// if exists: mark changed fields (hash, owner, mod date etc)

func things() (*string, error) {
	changes, err := diff.Diff("asdifoaifjasdif", "asdioaisd9fs8f9a8sf9jdf")
	if err != nil {
		return nil, err
	}
	ch := changes[0]
	changepath := strings.Join(ch.Path, ".")
	return &changepath, nil
}

type Change struct {
	Field string
	Mode uint8
	Old string
	New string
}

// download the db
type Changeset struct {
	change uint8
	isOld bool
	newIdx int
	oldIdx int
	changes []Change
}
// created := map[string] for old db
// for each rec in old:
// if not in new, add del record
// if in new, add stay record
// if not marked => new record