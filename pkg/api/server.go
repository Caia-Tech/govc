package api

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/pkg/object"
	"github.com/gorilla/mux"
)

type Server struct {
	repo   *govc.Repository
	router *mux.Router
}

func NewServer(repo *govc.Repository) *Server {
	s := &Server{
		repo:   repo,
		router: mux.NewRouter(),
	}
	s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() {
	s.router.HandleFunc("/{repo}/info/refs", s.handleInfoRefs).Methods("GET")
	s.router.HandleFunc("/{repo}/git-upload-pack", s.handleUploadPack).Methods("POST")
	s.router.HandleFunc("/{repo}/git-receive-pack", s.handleReceivePack).Methods("POST")

	s.router.HandleFunc("/api/status", s.handleAPIStatus).Methods("GET")
	s.router.HandleFunc("/api/branches", s.handleAPIBranches).Methods("GET")
	s.router.HandleFunc("/api/branches", s.handleAPICreateBranch).Methods("POST")
	s.router.HandleFunc("/api/branches/{branch}", s.handleAPIDeleteBranch).Methods("DELETE")
	s.router.HandleFunc("/api/commits", s.handleAPICommits).Methods("GET")
	s.router.HandleFunc("/api/commits", s.handleAPICreateCommit).Methods("POST")
	s.router.HandleFunc("/api/checkout", s.handleAPICheckout).Methods("POST")
	s.router.HandleFunc("/api/merge", s.handleAPIMerge).Methods("POST")
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")

	if service == "" {
		s.handleDumbInfoRefs(w, r)
		return
	}

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	packet := fmt.Sprintf("# service=%s\n", service)
	fmt.Fprintf(w, "%04x%s0000", len(packet)+4, packet)

	branches, err := s.repo.ListBranchesDetailed()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i, branch := range branches {
		line := fmt.Sprintf("%s %s", branch.Hash, branch.Name)
		if i == 0 {
			line += "\x00capabilities^{}\x00multi_ack thin-pack side-band ofs-delta shallow no-progress include-tag multi_ack_detailed allow-tip-sha1-in-want allow-reachable-sha1-in-want no-done filter"
		}
		fmt.Fprintf(w, "%04x%s\n", len(line)+5, line)
	}

	fmt.Fprint(w, "0000")
}

func (s *Server) handleDumbInfoRefs(w http.ResponseWriter, r *http.Request) {
	branches, err := s.repo.ListBranchesDetailed()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	for _, branch := range branches {
		fmt.Fprintf(w, "%s\t%s\n", branch.Hash, branch.Name)
	}
}

func (s *Server) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-git-upload-pack-request" {
		http.Error(w, "Invalid content type", http.StatusBadRequest)
		return
	}

	encoding := r.Header.Get("Content-Encoding")

	if encoding == "gzip" {
		// In a full implementation, we would decompress and process the request
		gzReader, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer gzReader.Close()
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	fmt.Fprint(w, "0008NAK\n")
}

func (s *Server) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/x-git-receive-pack-request" {
		http.Error(w, "Invalid content type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	fmt.Fprint(w, "0000")
}

func (s *Server) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.repo.Status()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleAPIBranches(w http.ResponseWriter, r *http.Request) {
	branches, err := s.repo.ListBranchesDetailed()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type BranchInfo struct {
		Name    string `json:"name"`
		Hash    string `json:"hash"`
		Current bool   `json:"current"`
	}

	currentBranch, _ := s.repo.CurrentBranch()

	branchInfos := make([]BranchInfo, 0, len(branches))
	for _, branch := range branches {
		name := strings.TrimPrefix(branch.Name, "refs/heads/")
		branchInfos = append(branchInfos, BranchInfo{
			Name:    name,
			Hash:    branch.Hash,
			Current: name == currentBranch,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(branchInfos)
}

func (s *Server) handleAPICreateBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		From string `json:"from"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Branch name required", http.StatusBadRequest)
		return
	}

	builder := s.repo.Branch(req.Name)
	if err := builder.Create(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"name":   req.Name,
		"status": "created",
	})
}

func (s *Server) handleAPIDeleteBranch(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	branch := vars["branch"]

	builder := s.repo.Branch(branch)
	if err := builder.Delete(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAPICommits(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	commits, err := s.repo.Log(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type CommitInfo struct {
		Hash      string `json:"hash"`
		Author    string `json:"author"`
		Email     string `json:"email"`
		Message   string `json:"message"`
		Timestamp int64  `json:"timestamp"`
		Parent    string `json:"parent,omitempty"`
	}

	commitInfos := make([]CommitInfo, 0, len(commits))
	for _, commit := range commits {
		hash := commit.Hash()
		commitInfos = append(commitInfos, CommitInfo{
			Hash:      hash,
			Author:    commit.Author.Name,
			Email:     commit.Author.Email,
			Message:   commit.Message,
			Timestamp: commit.Author.Time.Unix(),
			Parent:    commit.ParentHash,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commitInfos)
}

func (s *Server) handleAPICreateCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string   `json:"message"`
		Files   []string `json:"files"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Message == "" {
		http.Error(w, "Commit message required", http.StatusBadRequest)
		return
	}

	if len(req.Files) > 0 {
		if err := s.repo.Add(req.Files...); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	commit, err := s.repo.Commit(req.Message)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"hash":    commit.Hash(),
		"message": commit.Message,
	})
}

func (s *Server) handleAPICheckout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ref string `json:"ref"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Ref == "" {
		http.Error(w, "Ref required", http.StatusBadRequest)
		return
	}

	if err := s.repo.Checkout(req.Ref); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "checked out",
		"ref":    req.Ref,
	})
}

func (s *Server) handleAPIMerge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string `json:"branch"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Branch == "" {
		http.Error(w, "Branch required", http.StatusBadRequest)
		return
	}

	currentBranch, _ := s.repo.CurrentBranch()
	if err := s.repo.Merge(req.Branch, currentBranch); err != nil {
		if strings.Contains(err.Error(), "conflicts") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error":  err.Error(),
				"status": "conflict",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "merged",
		"branch": req.Branch,
	})
}

func StartServer(repo *govc.Repository, addr string) error {
	server := NewServer(repo)
	return http.ListenAndServe(addr, server)
}

type WebSocketHandler struct {
	repo *govc.Repository
}

func NewWebSocketHandler(repo *govc.Repository) *WebSocketHandler {
	return &WebSocketHandler{repo: repo}
}

func parsePacketLine(data []byte) ([]byte, []byte) {
	if len(data) < 4 {
		return nil, data
	}

	var length int
	fmt.Sscanf(string(data[:4]), "%04x", &length)

	if length == 0 {
		return []byte{}, data[4:]
	}

	if length < 4 || len(data) < length {
		return nil, data
	}

	return data[4:length], data[length:]
}

func formatPacketLine(data []byte) []byte {
	if len(data) == 0 {
		return []byte("0000")
	}

	length := len(data) + 4
	return []byte(fmt.Sprintf("%04x%s", length, data))
}

type GitProtocolHandler struct {
	repo *govc.Repository
}

func NewGitProtocolHandler(repo *govc.Repository) *GitProtocolHandler {
	return &GitProtocolHandler{repo: repo}
}

func (h *GitProtocolHandler) handleWants(wants []string) ([]object.Object, error) {
	objects := make([]object.Object, 0)

	for _, want := range wants {
		obj, err := h.repo.GetObject(want)
		if err != nil {
			continue
		}
		objects = append(objects, obj)
	}

	return objects, nil
}

func (h *GitProtocolHandler) handleHaves(haves []string) map[string]bool {
	hasMap := make(map[string]bool)

	for _, have := range haves {
		if h.repo.HasObject(have) {
			hasMap[have] = true
		}
	}

	return hasMap
}

// Repository extension methods are defined in the main repository.go file
