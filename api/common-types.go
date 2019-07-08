package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	mapset "github.com/deckarep/golang-set"
	"github.com/fatih/structs"
)

type HashDigest struct {
	Algorithm string
	Value     []byte
}

func (h HashDigest) String() string {
	return fmt.Sprintf("%s:%s", h.Algorithm, hex.EncodeToString(h.Value))
	// return fmt.Sprintf(base64.StdEncoding.EncodeToString(h.Value))
}

func (h *HashDigest) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	chonk := strings.SplitN(s, ":", 2)
	if len(chonk) < 2 {
		return fmt.Errorf("Invalid hash format")
	}
	h.Algorithm = chonk[0]
	h.Value = []byte(chonk[1])
	return nil
}

func (h HashDigest) MarshalJSON() ([]byte, error) {
	return []byte(h.String()), nil
}

type FileError struct {
	// Filename  string
	Error     error
	CreatedAt time.Time
}

func NewFileError(err error) FileError {
	fmt.Fprintln(os.Stderr, err)
	return FileError{
		// Filename:  filename,
		Error:     err,
		CreatedAt: time.Now(),
	}
}

// utility convert anything to api.RuleResult
func ToJSONMap(r interface{}) (map[string]string, error) {
	data := structs.Map(r)
	ret := map[string]string{}
	for key, value := range data {
		js, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		ret[key] = string(js)
	}
	return ret, nil
}

type AnnotResultConfig struct {
	IgnoredProps mapset.Set
	MetaProps    mapset.Set
	RuleID       int
	Path         string
}

type AnnotResult interface {
	toPropsMap() (map[string]string, error)
	GetConfig() AnnotResultConfig
}

// type AnnotError struct {
// 	err error
// }

// func (r *AnnotError) GetConfig() AnnotResultConfig {
// 	return AnnotResultConfig{
// 		IgnoredProps: mapset.NewSet("MountDir", "RuleID", "Path"),
// 		MetaProps:    mapset.NewSet("Errors", "QueuedAt", "ProcessedAt"),
// 		RuleID:       r.RuleID,
// 		Path:         r.Path,
// 	}
// }
// func (r *AnnotError) toPropsMap() (map[string]string, error) {
// 	return ToJSONMap(r)
// }

// TODO: implement the binary file data agregator

func ToChangeset(a AnnotResult) (map[string]string, error) {
	fields, err := a.toPropsMap()
	if err != nil {
		return nil, err
	}
	curConfig := a.GetConfig()
	changeset := map[string]string{}
	for key, val := range fields {
		isIgnored := curConfig.IgnoredProps.Contains(key)
		isMeta := curConfig.MetaProps.Contains(key)
		if isIgnored || isMeta {
			continue
		}
		changeset[key] = val
	}
	return changeset, nil
}

func ToRuleResult(a AnnotResult) []*RuleResults {
	ret := []*RuleResults{}
	annots, err := a.toPropsMap()
	if err != nil {
		panic(err) // TODO: do something with errors
	}
	config := a.GetConfig()
	for tag, value := range annots {
		if config.IgnoredProps.Contains(tag) {
			continue
		}
		meta := config.MetaProps.Contains(tag)
		ruleid := config.RuleID
		tag := tag // copying is crucial
		value := value
		rr := &RuleResults{
			Tag:    &tag,
			Value:  &value,
			RuleID: &ruleid,
			Meta:   &meta,
		}
		ret = append(ret, rr)
	}
	return ret
}

type FilePropsResult struct {
	RuleID      int
	Path        string
	MountDir    string
	Size        int64
	Mode        os.FileMode
	ModTime     time.Time
	QueuedAt    time.Time
	ProcessedAt time.Time
	IsDir       bool
	OwnerUID    *string
	Hash        *HashDigest
	Errors      []FileError
}

func (r *FilePropsResult) GetConfig() AnnotResultConfig {
	return AnnotResultConfig{
		IgnoredProps: mapset.NewSet("MountDir", "RuleID", "Path"),
		MetaProps:    mapset.NewSet("Errors", "QueuedAt", "ProcessedAt"),
		RuleID:       r.RuleID,
		Path:         r.Path,
	}
}
func (r *FilePropsResult) toPropsMap() (map[string]string, error) {
	return ToJSONMap(r)
}

type PathTagsResult struct {
	Values map[string]string
	RuleID int
	Path   string
}

func (r *PathTagsResult) GetConfig() AnnotResultConfig {
	return AnnotResultConfig{
		IgnoredProps: mapset.NewSet("RuleID", "Path", "Values"),
		MetaProps:    mapset.NewSet("Errors"),
		RuleID:       r.RuleID,
		Path:         r.Path,
	}
}
func (r *PathTagsResult) toPropsMap() (map[string]string, error) {
	return r.Values, nil
}
