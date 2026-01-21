package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})

	addr := ":8080"
	log.Printf("ct-archive-serve: starting placeholder server on %s", addr)
	log.Printf("ct-archive-serve: NOTE: implementation is not complete yet")

	if err := http.ListenAndServe(addr, mux); err != nil { //nolint:gosec // placeholder server; hardened server is specified in specs
		_, _ = fmt.Fprintf(os.Stderr, "ct-archive-serve: server error: %v\n", err)
		os.Exit(1)
	}
}

