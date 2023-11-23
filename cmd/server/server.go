package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
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
	mux.HandleFunc("/bad", bad)

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

type request struct {
	Path   string
	Target string
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

	var userInput request
	if err := json.Unmarshal(body, &userInput); err != nil {
		http.Error(w, fmt.Sprintf("failed to parse request body: %s", err), http.StatusBadRequest)
		return
	}

	c := &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			// Note: this disables H2 in some cases. We're not using it.
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	server, err := net.ResolveTCPAddr("tcp", userInput.Target)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to resolve server addr: %s", err), http.StatusInternalServerError)
		return
	}

	conn, err := net.DialTCP("tcp", nil, server)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to dial tcp: %s", err), http.StatusInternalServerError)
		return
	}

	fmt.Println(conn.LocalAddr())
	fmt.Println(conn.RemoteAddr())

	p := payload{
		Path:   userInput.Path,
		Target: userInput.Target,
		Local:  conn.LocalAddr().String(),
		Remote: conn.RemoteAddr().String(),
	}

	pd, err := json.Marshal(p)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal json: %s", err), http.StatusInternalServerError)
	}

	log.Println(string(pd))

	req, err := http.NewRequest("GET", "http://proxy/proxy", bytes.NewBuffer(pd))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create user-targeted request: %s", err), http.StatusBadRequest)
		return
	}

	log.Println("Sending request...")
	resp, err := c.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to send user-targeted request: %s", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	log.Println("dumping response")
	d, _ := httputil.DumpResponse(resp, true)
	log.Println(string(d))
	log.Println("dumping again response")

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

func bad(w http.ResponseWriter, r *http.Request) {
	log.Println("dumping request")
	d, _ := httputil.DumpRequest(r, true)
	log.Println(string(d))
	log.Println("dumped request")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("should not work"))
}
