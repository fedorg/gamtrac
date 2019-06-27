package api

import (
	"time"
)

// columns and relationships of "rule_results"
type RuleResults struct {
	CreatedAt *time.Time `json:"created_at,omitempty"`
	// An object relationship
	FileHistory   *FileHistory `json:"file_history,omitempty"`
	FileHistoryID *int          `json:"file_history_id,omitempty"`
	// An object relationship
	Rule         *Rules `json:"rule,omitempty"`
	RuleID       *int    `json:"rule_id,omitempty"`
	RuleResultID *int    `json:"rule_result_id,omitempty"`
	Tag          *string `json:"tag,omitempty"`
	Value        *string `json:"value,omitempty"`
}


// columns and relationships of "file_history"
type FileHistory struct {
	Action        string    `json:"action,omitempty"`
	ActionTstamp  *time.Time `json:"action_tstamp,omitempty"`
	FileHistoryID int64     `json:"file_history_id,omitempty"`
	Filename      string    `json:"filename,omitempty"`
	// An object relationship
	Prev   *FileHistory `json:"prev,omitempty"`
	PrevID int          `json:"prev_id,omitempty"`
	// An array relationship
	RuleResults []*RuleResults `json:"rule_results,omitempty"`
	// An aggregated array relationship
	RuleResultsAggregate *RuleResultsAggregate `json:"rule_results_aggregate,omitempty"`
	// An object relationship
	Scan   *Scans `json:"scan,omitempty"`
	ScanID int    `json:"scan_id,omitempty"`
}
