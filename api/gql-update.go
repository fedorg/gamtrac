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

func (gg *GamtracGql) RunFetchFiles(rev int) ([]Files, error) {
	var respData struct {
		Files []Files `json:"files"`
	}

	query := `
	query ($revision: Int) {
		files(where: {revision_id: {_eq: $revision}}) {
		  file_id
		  filename
		  revision_id
		  data
		}
	}
	`
	vars := map[string]interface{}{ // what the fuck, this is clearly map[string]int
		"revision": rev,
	}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}
	return respData.Files, nil
}

func (gg *GamtracGql) RunUpsertFiles(files []Files) ([]Files, error) {
	var respData struct {
		InsertFiles struct {
			Files []Files `json:"returning"`
		} `json:"insert_files"`
	}

	query := `
	mutation ($files: [files_insert_input!]!) {
		insert_files(objects: $files,
		  on_conflict: {
			constraint: files_filename_key,
			update_columns: [revision_id, data]
		  }
		)
		{
		  returning {
			file_id
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
	return respData.InsertFiles.Files, nil
}

func (gg *GamtracGql) RunInsertFiles(files []Files) ([]Files, error) {
	var respData struct {
		InsertFiles struct {
			Files []Files `json:"returning"`
		} `json:"insert_files"`
	}

	query := `
	mutation ($files: [files_insert_input!]!) {
		insert_files(objects: $files) {
		  returning {
			file_id
		  }
		}
	  }
	`
	vars := map[string]interface{}{ // what the fuck, this is clearly map[string]int
		"files": files,
	}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}
	return respData.InsertFiles.Files, nil
}

func (gg *GamtracGql) RunDeleteFiles(currentRevision int) ([]Files, error) {
	var respData struct {
		DeleteFiles struct {
			Files []Files `json:"returning"`
		} `json:"update_files"`
	}

	query := `
	mutation ($cur_rev: Int) {
		update_files(
			where: {revision_id: {_lt: $cur_rev}},
			_set: {filename: null}
			) {
			returning {
				filename
				file_id
				revision_id
			}
		}
	}
	`
	vars := map[string]interface{}{ // what the fuck, this is clearly map[string]int
		"cur_rev": currentRevision,
	}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}
	return respData.DeleteFiles.Files, nil
}

func (gg *GamtracGql) RunCreateRevision() (*int, error) {
	var respData struct {
		CreateRevision struct {
			Revisions []Revisions `json:"returning"`
		} `json:"insert_revisions"`
	}

	query := `
	mutation {
		insert_revisions(objects: [{}]) {
		  returning {
			revision_id
		  }
		}
	  }
	`
	vars := map[string]interface{}{}
	if err := gg.Run(query, &respData, vars); err != nil {
		return nil, err
	}

	return &(respData.CreateRevision.Revisions[0].RevisionID), nil
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
