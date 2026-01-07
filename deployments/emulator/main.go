package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

var (
	sockPath = flag.String("sock", "/var/lib/kubelet/device-plugins/nvidia-gpu.sock.status", "unix socket path to listen on")
)

type Status map[string]int // deviceID -> remaining percent

type server struct {
	mu     sync.Mutex
	status Status
}

func newServer() *server {
	s := &server{status: make(Status)}
	if env := os.Getenv("EMULATOR_DEVICES"); env != "" {
		for i, d := range strings.Split(env, ",") {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			s.status[d] = 100 - i*20
		}
	}
	if len(s.status) == 0 {
		s.status["GPU-0"] = 100
		s.status["GPU-1"] = 80
	}
	return s
}

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.status)
}

func readJSON(r io.Reader, v interface{}) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *server) handleReserve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Devices []string `json:"devices"`
		Percent int      `json:"percent"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range req.Devices {
		rem, ok := s.status[d]
		if !ok || rem < req.Percent {
			http.Error(w, "insufficient", http.StatusConflict)
			return
		}
	}
	for _, d := range req.Devices {
		s.status[d] -= req.Percent
		if s.status[d] < 0 {
			s.status[d] = 0
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleUnreserve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Devices []string `json:"devices"`
		Percent int      `json:"percent"`
	}
	if err := readJSON(r.Body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range req.Devices {
		if _, ok := s.status[d]; !ok {
			s.status[d] = 0
		}
		s.status[d] += req.Percent
		if s.status[d] > 100 {
			s.status[d] = 100
		}
	}
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()
	if *sockPath == "" {
		log.Fatal("sock path required")
	}
	s := newServer()
	d := path.Dir(*sockPath)
	if err := os.MkdirAll(d, 0755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	_ = os.Remove(*sockPath)
	ln, err := net.Listen("unix", *sockPath)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	h := http.NewServeMux()
	h.HandleFunc("/status", s.handleStatus)
	h.HandleFunc("/reserve", s.handleReserve)
	h.HandleFunc("/unreserve", s.handleUnreserve)
	log.Printf("listening on unix socket %s", *sockPath)
	if err := http.Serve(ln, h); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
