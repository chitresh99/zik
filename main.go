package main

import (
	"log"
	"net/http"
	"strings"
	"zik/internal/handler"
	"zik/internal/store"
)

func main() {
	s := store.NewMemoryStore()
	h := handler.NewConfigHandler(s)

	mux := http.NewServeMux()

	mux.HandleFunc("/namespaces/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")

		// /namespaces/{ns}/configs
		if len(parts) == 4 {
			h.HandleList(w, r)
			return
		}

		// /namespaces/{ns}/configs/{key}/rollback
		if len(parts) == 6 && parts[5] == "rollback" {
			h.HandleRollback(w, r)
			return
		}

		// /namespaces/{ns}/configs/{key}
		if len(parts) == 5 {
			switch r.Method {
			case http.MethodGet:
				h.HandleGet(w, r)
			case http.MethodPost:
				h.HandleSet(w, r)
			case http.MethodDelete:
				h.HandleDelete(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	})

	log.Println("config service starting on :8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
