package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"localdream-v2/internal/generation"
)

type API struct {
	server *generation.Service
}

func NewHandler(server *generation.Service) http.Handler {
	api := &API{
		server: server,
	}

	//路由：匹配路径和方法
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/generations", api.createGeneration)
	mux.HandleFunc("GET /api/generations/{id}", api.getGeneration)

	return mux
}

func (a *API) createGeneration(
	w http.ResponseWriter,
	r *http.Request,
) {
	var input struct {
		Prompt string `json:"prompt"`
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid JSON",
		})
		return
	}

	task, err := a.server.Create(input.Prompt)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, task)
}

func (a *API) getGeneration(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")

	task, ok := a.server.Get(id)

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "task not found",
		})
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	value any,
) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
