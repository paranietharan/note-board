package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type storedValue struct {
	value     string
	timestamp time.Time
}

type ValueStore struct {
	mu     sync.RWMutex
	values map[string]storedValue
	ttl    time.Duration
}

func NewValueStore(ttl time.Duration) *ValueStore {
	vs := &ValueStore{
		values: make(map[string]storedValue),
		ttl:    ttl,
	}

	go vs.startCleanupRoutine(1 * time.Hour)

	return vs
}

func (vs *ValueStore) Set(id string, value string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.values[id] = storedValue{
		value:     value,
		timestamp: time.Now(),
	}
}

func (vs *ValueStore) Get(id string) string {
	vs.mu.RLock()
	val, exists := vs.values[id]
	vs.mu.RUnlock()

	if !exists || time.Since(val.timestamp) > vs.ttl {
		vs.mu.Lock()
		delete(vs.values, id)
		vs.mu.Unlock()
		return ""
	}

	return val.value
}

func (vs *ValueStore) startCleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()

		vs.mu.Lock()
		for id, val := range vs.values {
			if now.Sub(val.timestamp) > vs.ttl {
				delete(vs.values, id)
			}
		}
		vs.mu.Unlock()
	}
}

func main() {
	store := NewValueStore(24 * time.Hour)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			id := r.URL.Query().Get("id")

			if id == "" {
				http.Error(w, "missing ?id parameter", http.StatusBadRequest)
				return
			}

			val := store.Get(id)
			if val == "" {
				http.Error(w, "not found or expired", http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{
					"message": "Clipboard not found",
				})
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]string{"id": id, "value": val}); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}

		case http.MethodPost:
			id := r.URL.Query().Get("id")
			val := r.URL.Query().Get("value")

			if id == "" || val == "" {
				http.Error(w, "`id` and `value` required", http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{
					"message": "Sorry something went wrong",
				})
				return
			}

			store.Set(id, val)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"message": "Clip board recorded successfully",
				"id":      id,
				"value":   val,
			})

		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Println("Clipboard server listening on :8080 ...")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
