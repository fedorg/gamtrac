package api

import (
	"context"
	"encoding/json"
	"log"
	"fmt"
	"time"

	"github.com/machinebox/graphql"
)

type GamtracGql struct {
	Client  *graphql.Client
	Timeout time.Duration
}

func NewGamtracGql(endpoint string, timeout_ms uint32, debugLog bool) *GamtracGql {
	gg := GamtracGql{
		Client:  graphql.NewClient(endpoint),
		Timeout: time.Millisecond * time.Duration(timeout_ms),
	}
	if debugLog {
		gg.Client.Log = func(s string) { log.Println(s) }
	}
	return &gg
}

func (gg *GamtracGql) Run(query string, rslt interface{}, vars map[string]interface{}) error {
	client := gg.Client
	timeout := gg.Timeout
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := graphql.NewRequest(query)
	req.Header.Set("Cache-Control", "no-cache")
	for k, v := range vars {
		req.Var(k, v)
	}

	if err := client.Run(ctx, req, rslt); err != nil {
		return err
	}
	return nil
}

func (gg *GamtracGql) RunFetchFiles() ([]FileHistory, error) {
	var respData struct {
		FileHistories [] struct {
			File FileHistory `json:"file_history"`
		} `json:"files"`
	}

	query := `
	query {
		files {
		  file_history {
			file_history_id
			action
			action_tstamp
			filename
			prev_id
			scan_id
			rule_results {
				file_history_id
				rule_result_id
				rule_id
				created_at
				tag
				value
				meta
			}
		  }
		}
	}
	`
	if err := gg.Run(query, &respData, nil); err != nil {
		return nil, err
	}
	files := make([]FileHistory, len(respData.FileHistories))
	for i := range respData.FileHistories {
		files[i] = respData.FileHistories[i].File
		if (files[i].Filename == "") {
			return nil, fmt.Errorf("Internal error: empty filename in old file history #%v, id %v", i, files[i].FileHistoryID)
		}
	}
	return files, nil
}

func fillStruct(data interface{}, recv interface{}) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, recv)
	return err
}

type JSON map[string]interface{}

func ToNestedInsert(props []string, objects []interface{}) []JSON {
	// Hasura needs a bit of reshaping for nested inserts, namely
	// what normally would be a [rule_results] it needs a {data: [rule_results], on_conflict: {...}}
	toInsert := make([]JSON, len(objects))
	for i := range objects {
		err := fillStruct(&objects[i], &toInsert[i])
		if err != nil {
			panic(err)
		}
	}
	for i := range toInsert {
		ret := toInsert[i]
		for _, prop := range props {
			if ret[prop] != nil {
				ret[prop] = JSON{"data": ret[prop]}
			}
		}
	}
	return toInsert
}

func (gg *GamtracGql) RunInsertFileHistory(files []FileHistory) ([]int64, error) {
	var respData struct {
		InsertFileHistory struct {
			FileHistories []FileHistory `json:"returning"`
		} `json:"insert_file_history"`
	}

	pfiles := make([]interface{}, len(files))
	for i := range files {
		pfiles[i] = &files[i]
	}

	insertData := ToNestedInsert([]string{"rule_results"}, pfiles)
	for i, id := range insertData {
		if s, ok := id["filename"].(string); !ok {
			println("File ", i, "is not ok", s)
			if ss, err := json.Marshal(insertData[i]); err == nil {
				println(string(ss))
			}
		}
	}
	query := `
	mutation ($files: [file_history_insert_input!]!) {
		insert_file_history(objects: $files)
		{
		  returning {
			file_history_id
		  }
		}
	}
	`

	vars := map[string]interface{}{
		"files": insertData,
	}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}
	ret := []int64{}
	for _, fh := range respData.InsertFileHistory.FileHistories {
		ret = append(ret, fh.FileHistoryID)
	}
	return ret, nil
}

func (gg *GamtracGql) RunInsertRuleResults(results []*RuleResults) ([]int, error) {
	var respData struct {
		InsertRuleResults struct {
			RuleResults []RuleResults `json:"returning"`
		} `json:"insert_rule_results"`
	}
	query := `
	mutation ($results: [rule_results_insert_input!]!) {
		insert_rule_results(objects: $results)
		{
		  returning {
			rule_results_id
		  }
		}
	}
	`
	vars := map[string]interface{}{
		"results": results,
	}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}
	ret := []int{}
	for _, rr := range respData.InsertRuleResults.RuleResults {
		ret = append(ret, *rr.RuleResultID)
	}
	return ret, nil
}

func (gg *GamtracGql) RunCreateScan() (*int, error) {
	var respData struct {
		CreateScan struct {
			Scans []Scans `json:"returning"`
		} `json:"insert_scans"`
	}

	query := `
	mutation {
		insert_scans(objects: [{
			completed_at: null
		}]) {
		  returning {
			scan_id
		  }
		}
	  }
	`
	vars := map[string]interface{}{}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}

	return &(respData.CreateScan.Scans[0].ScanID), nil
}

func (gg *GamtracGql) RunFinishScan(scan int) (*Scans, error) {
	var respData struct {
		FinishScan struct {
			Scans []Scans `json:"returning"`
		} `json:"update_scans"`
	}

	query := `
	mutation ($scan_id: Int!) {
		update_scans (where: {scan_id :{_eq: $scan_id}},
		_set: {completed_at: "now()" }) {
		  returning {
			scan_id
			started_at
			completed_at
			file_histories_aggregate {
			  aggregate {
				 count
			  }
			}
		  }
		}
	  }
	`
	vars := map[string]interface{}{
		"scan_id": scan,
	}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}

	return &(respData.FinishScan.Scans[0]), nil
}

func (gg *GamtracGql) RunInsertDomainUsers(users []DomainUsers) error {
	// var respData struct {
	// 	InsertDomainUsers struct {
	// 		DomainUsers []DomainUsers `json:"returning"`
	// 	} `json:"insert_domain_users"`
	// }

	query := `
	mutation ($users: [domain_users_insert_input!]!) {
		insert_domain_users(objects: $users) {
			affected_rows
		}
	  }
	`
	vars := map[string]interface{}{
		"users": users,
	}
	return gg.Run(query, nil, vars)
}

func (gg *GamtracGql) RunDeleteDomainUsers() error {
	// var respData struct {
	// 	DeleteFiles struct {
	// 		Files []Files `json:"returning"`
	// 	} `json:"delete_domain_users"`
	// }

	query := `
	mutation {
		delete_domain_users(where: {}) {
			affected_rows
		}
	}
	`
	vars := map[string]interface{}{}
	return gg.Run(query, nil, vars)
}

func (gg *GamtracGql) RunFetchRules() ([]Rules, error) {
	var respData struct {
		Rules []Rules `json:"rules"`
	}

	query := `
	query {
		rules(where:{rule_type:{_eq: "pathtags"}}) {
			rule_id
			principal
			priority
			rule
			ignore
			rule_type
		}
	}
	`
	if err := gg.Run(query, &respData, nil); err != nil {
		return nil, err
	}
	return respData.Rules, nil
}
