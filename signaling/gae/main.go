package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/nobonobo/ssh-p2p/signaling"
)

var (
	// Sets your Google Cloud Platform project ID.
	projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	res       = map[string]chan signaling.ConnectInfo{}
	mu        sync.RWMutex
)

func main() {
	http.Handle("/pull/", http.StripPrefix("/pull/", pullData()))
	http.Handle("/push/", http.StripPrefix("/push/", pushData()))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}

func pushData() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var info signaling.ConnectInfo
		if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
			log.Print("json decode failed:", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		select {
		default:
		case res[r.URL.Path] <- info:
		}
	})
}

func pullData() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ch := res[r.URL.Path]
		if ch == nil {
			ch = make(chan signaling.ConnectInfo)
			res[r.URL.Path] = ch
		}
		mu.Unlock()
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		select {
		case <-ctx.Done():
			http.Error(w, ``, http.StatusRequestTimeout)
			return
		case v := <-ch:
			w.Header().Add("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(v); err != nil {
				log.Print("json encode failed:", err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
		}
	})
}
