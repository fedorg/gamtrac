package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
)

// TODO: remove this global state

type ServerState struct {
	sync.Mutex
	files map[string]*AnnotResult
}

func NewServerState() *ServerState {
	srvState := ServerState{
		files: make(map[string]*AnnotResult),
	}
	return &srvState
}

func (ss *ServerState) Update(newFiles map[string]*AnnotResult) *ServerState {
	ss.Lock()
	ss.files = newFiles
	ss.Unlock()
	return ss
}

var srvState *ServerState

// Serve ... run the server and persistence layer
func Serve(state *ServerState) {
	srvState = state
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/getall/", repoHandler)
	log.Fatal(http.ListenAndServe("localhost:8000", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	//...
	w.WriteHeader(200)
	w.Write([]byte("hi, this is patrick"))
}

func repoHandler(w http.ResponseWriter, r *http.Request) {
	//...
	srvState.Lock()
	jsvals := map[string]AnnotResult{}
	for _, val := range srvState.files {
		jsvals[val.Path] = *val
	}
	ret, err := json.MarshalIndent(jsvals, "", " ")
	srvState.Unlock()

	if err != nil {
		w.WriteHeader(400)
		w.Write([]byte(fmt.Sprintf("400 Unhandled exception: %v", err)))
		return
	}
	w.WriteHeader(200)
	w.Write([]byte(ret))
}
