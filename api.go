package main

import (
	"log"
	"net/http"
)

// Serve ... run the server and persistence layer
func Serve() {
	http.HandleFunc("/api/index", indexHandler)
	http.HandleFunc("/api/repo/", repoHandler)
	log.Fatal(http.ListenAndServe("localhost:8000", nil))
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	//...
	w.WriteHeader(400)
	w.Write([]byte("hi, this is patrick"))
}

func repoHandler(w http.ResponseWriter, r *http.Request) {
	//...
	w.WriteHeader(200)
	w.Write([]byte("Bye"))
}
