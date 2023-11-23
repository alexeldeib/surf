package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	int := make(chan os.Signal, 1)
	signal.Notify(int, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-int; cancel(); os.Exit(1) }()

	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/proxy", proxy)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
	}

	return srv.ListenAndServe()
}

type payload struct {
	Path   string
	Target string
	Local  string
	Remote string
}

func proxy(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body of request", http.StatusInternalServerError)
		return
	}
	var userInput payload

	if err := json.Unmarshal(body, &userInput); err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal json: %s", err), http.StatusInternalServerError)
	}

	log.Println(string(body))

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/%s", userInput.Target, userInput.Path), nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create proxied request: %s", err), http.StatusBadRequest)
		return
	}

	d := &net.Dialer{
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Duration(3000) * time.Millisecond,
				}
				return d.DialContext(ctx, network, address)
			},
		},
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	c := &http.Client{
		Transport: &http.Transport{
			Dial:                  (d).Dial,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			// Note: this disables H2 in some cases. We're not using it.
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	log.Println("Sending request...")
	resp, err := c.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send proxied request: %s", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	log.Println("dumping response")
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read proxied response body: %s", err), http.StatusInternalServerError)
		return
	}
	log.Println(string(respBody))
	log.Println("dumped response")

	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
