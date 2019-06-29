package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/fatih/structs"
	"os"
	"strings"
	"time"
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

type AnnotResult struct {
	Path        string             `diff:"Path,identifier"` // required
	RuleID      int                `diff:"RuleID"`
	MountDir    string             `diff:"-" json:"-"`
	Size        int64              `diff:"Size"`     // required
	Mode        os.FileMode        `diff:"Mode"`     // required
	ModTime     time.Time          `diff:"ModTime"`  // required
	QueuedAt    time.Time          `diff:"-"`        // required
	ProcessedAt time.Time          `diff:"-"`        // required
	IsDir       bool               `diff:"IsDir"`    // required
	OwnerUID    *string            `diff:"OwnerUID"` // optional
	Hash        *HashDigest        `diff:"Hash"`     // optional
	Pattern     *string            `diff:"Pattern"`  // optional
	Parsed      *map[string]string `diff:"Parsed"`   // optional
	Errors      []FileError        `diff:"-"`        // required
}

func (a AnnotResult) ToJSONMap() map[string]string {
	if ret, err := ToJSONMap(a); err != nil {
		panic(err)
	} else {
		return ret
	}
}

// TODO: convert to interface
// RuleID *int
// RuleType string
// meta // ignored on diff, write
// writeMeta() // which of meta to write
// toruleinsert(ruleid)



func (a AnnotResult) ToRuleInsert() []*RuleResults {
	ret := []*RuleResults{}
	for tag, value := range a.ToJSONMap() {
		if (tag == "Path" || tag == "MountDir" || tag == "RuleID" || tag == "Pattern") {
			continue
		}
		// copy this stuff
		ruleid := a.RuleID // todo: move to param or interface
		tag := tag
		value := value
		rr := &RuleResults{
			Tag:           &tag,
			Value:         &value,
			RuleID:        &ruleid,
		}
		ret = append(ret, rr)
	}
	return ret
}