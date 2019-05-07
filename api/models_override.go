package api

type Files struct {
	Data     map[string]interface{} `json:"data"`
	FileID   int    `json:"file_id,omitempty"`
	Filename string `json:"filename"`
	Revision int    `json:"revision"`
}
