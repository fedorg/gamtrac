package api

import (
	"context"
	"log"
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
		FileHistories []FileHistory `json:"file_history"`
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
				data
			}
		  }
		}
	}
	`
	if err := gg.Run(query, &respData, nil); err != nil {
		return nil, err
	}
	return respData.FileHistories, nil
}

func (gg *GamtracGql) RunInsertFileHistory(files []FileHistory) ([]int64, error) {
	var respData struct {
		InsertFileHistory struct {
			FileHistories []FileHistory `json:"returning"`
		} `json:"insert_file_history"`
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
		"files": files,
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
		rules {
			principal
			priority
			rule
			rule_id
			ignore
		}
	}
	`
	if err := gg.Run(query, &respData, nil); err != nil {
		return nil, err
	}
	return respData.Rules, nil
}
