package main

import (
	"log"
	"net/http"
	"time"

	"localdream-v2/internal/generation"
	"localdream-v2/internal/httpapi"
)

func main() {
	//service := generation.NewService()
	executor := &generation.FakeExecutor{
		Delay: 3 * time.Second,
	}
	service := generation.NewService(executor)
	handler := httpapi.NewHandler(service)

	server := &http.Server{
		Addr:        "127.0.0.1:8791",
		Handler:     handler,
		ReadTimeout: 5 * time.Second,
	}

	log.Printf("Local Dream v2 listening on http://%s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
