package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"zik/internal/handler"
	"zik/internal/store"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, relying on environment variables")
	}

	dbURL := os.Getenv("DATABASE_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var s store.Store

	if dbURL != "" {
		log.Println("connecting to Postgres")
		pgStore, err := store.NewPostgresStore(context.Background(), dbURL)
		if err != nil {
			log.Fatalf("failed to connect to postgres: %v", err)
		}
		defer pgStore.Close()
		s = pgStore
		log.Println("connected to Postgres")
	} else {
		log.Println("DATABASE_URL not set, using in-memory store")
		s = store.NewMemoryStore()
	}

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

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("config-service starting on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
