package api

import "time"

// Request types

type CreateRepoRequest struct {
	ID         string `json:"id" binding:"required"`
	MemoryOnly bool   `json:"memory_only"`
}

type AddFileRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type WriteFileRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type CommitRequest struct {
	Message string `json:"message" binding:"required"`
	Author  string `json:"author"`
	Email   string `json:"email"`
}

type CreateBranchRequest struct {
	Name string `json:"name" binding:"required"`
	From string `json:"from"`
}

type CheckoutRequest struct {
	Branch string `json:"branch" binding:"required"`
}

type MergeRequest struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to" binding:"required"`
}

type ParallelRealitiesRequest struct {
	Branches []string `json:"branches" binding:"required"`
}

type TransactionAddRequest struct {
	Path    string `json:"path" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type TransactionCommitRequest struct {
	Message string `json:"message" binding:"required"`
}

// Response types

type RepoResponse struct {
	ID           string    `json:"id"`
	Path         string    `json:"path"`
	CurrentBranch string   `json:"current_branch,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type CommitResponse struct {
	Hash      string    `json:"hash"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
	Parent    string    `json:"parent,omitempty"`
}

type BranchResponse struct {
	Name      string `json:"name"`
	Commit    string `json:"commit"`
	IsCurrent bool   `json:"is_current"`
}

type FileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Size    int    `json:"size"`
}

type StatusResponse struct {
	Branch    string   `json:"branch"`
	Staged    []string `json:"staged"`
	Modified  []string `json:"modified"`
	Untracked []string `json:"untracked"`
	Clean     bool     `json:"clean"`
}

type TransactionResponse struct {
	ID        string    `json:"id"`
	RepoID    string    `json:"repo_id"`
	CreatedAt time.Time `json:"created_at"`
}

type RealityResponse struct {
	Name      string    `json:"name"`
	Isolated  bool      `json:"isolated"`
	Ephemeral bool      `json:"ephemeral"`
	CreatedAt time.Time `json:"created_at"`
}

type ErrorResponse struct {
	Error   string      `json:"error"`
	Code    string      `json:"code,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// Additional response types for tests

type LogResponse struct {
	Commits []CommitResponse `json:"commits"`
	Total   int              `json:"total"`
}

type BranchListResponse struct {
	Branches []BranchResponse `json:"branches"`
	Current  string           `json:"current"`
}

// File operation types

type ReadFileRequest struct {
	Path string `json:"path"`
	Ref  string `json:"ref,omitempty"`
}

type TreeEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Type    string    `json:"type"` // "file" or "dir"
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	Modified time.Time `json:"modified,omitempty"`
}

type TreeResponse struct {
	Path      string      `json:"path"`
	Entries   []TreeEntry `json:"entries"`
	Total     int         `json:"total"`
}

type MoveFileRequest struct {
	From string `json:"from" binding:"required"`
	To   string `json:"to" binding:"required"`
}

type ReadFileResponse struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"` // "utf-8" or "base64"
	Size     int64  `json:"size"`
}

// Diff operation types

type DiffRequest struct {
	From   string `json:"from" binding:"required"`
	To     string `json:"to" binding:"required"`
	Format string `json:"format,omitempty"`
}

type DiffResponse struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Format string `json:"format"`
	Diff   string `json:"diff"`
}

type FileDiffResponse struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
	Diff string `json:"diff"`
}

type FileDiff struct {
	Path      string `json:"path"`
	OldPath   string `json:"old_path,omitempty"`
	Status    string `json:"status"` // added, modified, deleted, renamed
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch"`
}

type WorkingDiff struct {
	Staged   []FileDiff `json:"staged"`
	Unstaged []FileDiff `json:"unstaged"`
}

type WorkingDiffResponse struct {
	Staged   []FileDiff `json:"staged"`
	Unstaged []FileDiff `json:"unstaged"`
	Total    int        `json:"total"`
}

// Blame operation types

type BlameLine struct {
	LineNumber int       `json:"line_number"`
	Content    string    `json:"content"`
	CommitHash string    `json:"commit_hash"`
	Author     string    `json:"author"`
	Email      string    `json:"email"`
	Timestamp  time.Time `json:"timestamp"`
	Message    string    `json:"message"`
}

type BlameResponse struct {
	Path  string      `json:"path"`
	Ref   string      `json:"ref"`
	Lines []BlameLine `json:"lines"`
	Total int         `json:"total"`
}

// Stash operation types

type StashRequest struct {
	Message       string `json:"message,omitempty"`
	IncludeUntracked bool `json:"include_untracked,omitempty"`
}

type StashResponse struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
	Files     []string  `json:"files"`
}

type StashListResponse struct {
	Stashes []StashResponse `json:"stashes"`
	Count   int             `json:"count"`
}

type StashApplyRequest struct {
	Drop bool `json:"drop,omitempty"`
}

// Cherry-pick operation types

type CherryPickRequest struct {
	Commit string `json:"commit" binding:"required"`
}

type CherryPickResponse struct {
	OriginalCommit string    `json:"original_commit"`
	NewCommit      string    `json:"new_commit"`
	Message        string    `json:"message"`
	Author         string    `json:"author"`
	Email          string    `json:"email"`
	Timestamp      time.Time `json:"timestamp"`
	FilesChanged   []string  `json:"files_changed"`
}

// Revert operation types

type RevertRequest struct {
	Commit string `json:"commit" binding:"required"`
}

type RevertResponse struct {
	RevertedCommit string    `json:"reverted_commit"`
	NewCommit      string    `json:"new_commit"`
	Message        string    `json:"message"`
	Author         string    `json:"author"`
	Email          string    `json:"email"`
	Timestamp      time.Time `json:"timestamp"`
	FilesChanged   []string  `json:"files_changed"`
}

// Rebase operation types

type RebaseRequest struct {
	Onto string `json:"onto" binding:"required"`
}

type RebaseResponse struct {
	OldHead      string   `json:"old_head"`
	NewHead      string   `json:"new_head"`
	RebasedCount int      `json:"rebased_count"`
	Commits      []string `json:"commits"`
}

// Reset operation types

type ResetRequest struct {
	Target string `json:"target" binding:"required"`
	Mode   string `json:"mode,omitempty"` // soft, mixed (default), hard
}

type ResetResponse struct {
	OldHead string `json:"old_head"`
	NewHead string `json:"new_head"`
	Mode    string `json:"mode"`
}

// Search operation types

type SearchCommitsRequest struct {
	Query  string `json:"query" form:"query" binding:"required"`
	Author string `json:"author,omitempty" form:"author"`
	Since  string `json:"since,omitempty" form:"since"`   // RFC3339 format
	Until  string `json:"until,omitempty" form:"until"`   // RFC3339 format
	Limit  int    `json:"limit,omitempty" form:"limit"`
	Offset int    `json:"offset,omitempty" form:"offset"`
}

type SearchCommitsResponse struct {
	Query     string            `json:"query"`
	Results   []CommitResponse  `json:"results"`
	Total     int               `json:"total"`
	Limit     int               `json:"limit"`
	Offset    int               `json:"offset"`
	Matches   []SearchMatch     `json:"matches"`
}

type SearchMatch struct {
	Field     string `json:"field"`     // "message", "author", "email"
	Line      int    `json:"line"`      // for content matches
	Column    int    `json:"column"`    // for content matches
	Preview   string `json:"preview"`   // highlighted preview
}

type SearchContentRequest struct {
	Query    string `json:"query" form:"query" binding:"required"`
	Path     string `json:"path,omitempty" form:"path"`      // path pattern
	Ref      string `json:"ref,omitempty" form:"ref"`        // commit/branch to search
	CaseSensitive bool `json:"case_sensitive,omitempty" form:"case_sensitive"`
	Regex    bool   `json:"regex,omitempty" form:"regex"`
	Limit    int    `json:"limit,omitempty" form:"limit"`
	Offset   int    `json:"offset,omitempty" form:"offset"`
}

type SearchContentResponse struct {
	Query     string          `json:"query"`
	Results   []ContentMatch  `json:"results"`
	Total     int             `json:"total"`
	Limit     int             `json:"limit"`
	Offset    int             `json:"offset"`
}

type ContentMatch struct {
	Path      string        `json:"path"`
	Ref       string        `json:"ref"`
	Line      int           `json:"line"`
	Column    int           `json:"column"`
	Content   string        `json:"content"`
	Preview   string        `json:"preview"`
	Matches   []MatchRange  `json:"matches"`
}

type MatchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type SearchFilesRequest struct {
	Query    string `json:"query" form:"query" binding:"required"`
	Ref      string `json:"ref,omitempty" form:"ref"`        // commit/branch to search
	CaseSensitive bool `json:"case_sensitive,omitempty" form:"case_sensitive"`
	Regex    bool   `json:"regex,omitempty" form:"regex"`
	Limit    int    `json:"limit,omitempty" form:"limit"`
	Offset   int    `json:"offset,omitempty" form:"offset"`
}

type SearchFilesResponse struct {
	Query     string        `json:"query"`
	Results   []FileMatch   `json:"results"`
	Total     int           `json:"total"`
	Limit     int           `json:"limit"`
	Offset    int           `json:"offset"`
}

type FileMatch struct {
	Path    string        `json:"path"`
	Ref     string        `json:"ref"`
	Size    int64         `json:"size"`
	Mode    string        `json:"mode"`
	Matches []MatchRange  `json:"matches"`
}

type GrepRequest struct {
	Pattern   string `json:"pattern" binding:"required"`
	Path      string `json:"path,omitempty"`                   // path pattern
	Ref       string `json:"ref,omitempty"`                    // commit/branch to search
	CaseSensitive bool `json:"case_sensitive,omitempty"`
	Regex     bool   `json:"regex,omitempty"`
	InvertMatch bool `json:"invert_match,omitempty"`           // -v flag
	WordRegexp  bool `json:"word_regexp,omitempty"`            // -w flag
	LineRegexp  bool `json:"line_regexp,omitempty"`            // -x flag
	ContextBefore int `json:"context_before,omitempty"`        // -B flag
	ContextAfter  int `json:"context_after,omitempty"`         // -A flag
	Context       int `json:"context,omitempty"`               // -C flag
	MaxCount      int `json:"max_count,omitempty"`             // -m flag
	Limit         int `json:"limit,omitempty"`
	Offset        int `json:"offset,omitempty"`
}

type GrepResponse struct {
	Pattern   string        `json:"pattern"`
	Results   []GrepMatch   `json:"results"`
	Total     int           `json:"total"`
	Limit     int           `json:"limit"`
	Offset    int           `json:"offset"`
}

type GrepMatch struct {
	Path      string        `json:"path"`
	Ref       string        `json:"ref"`
	Line      int           `json:"line"`
	Column    int           `json:"column"`
	Content   string        `json:"content"`
	Before    []string      `json:"before,omitempty"`    // context lines before
	After     []string      `json:"after,omitempty"`     // context lines after
	Matches   []MatchRange  `json:"matches"`
}

// Hooks & Events operation types

type WebhookEvent string

const (
	EventPush        WebhookEvent = "push"
	EventCommit      WebhookEvent = "commit"
	EventBranch      WebhookEvent = "branch"
	EventTag         WebhookEvent = "tag"
	EventMerge       WebhookEvent = "merge"
	EventStash       WebhookEvent = "stash"
	EventReset       WebhookEvent = "reset"
	EventRebase      WebhookEvent = "rebase"
	EventCherryPick  WebhookEvent = "cherry-pick"
	EventRevert      WebhookEvent = "revert"
)

type HookRequest struct {
	URL         string         `json:"url" binding:"required"`
	Events      []WebhookEvent `json:"events" binding:"required"`
	Secret      string         `json:"secret,omitempty"`
	ContentType string         `json:"content_type,omitempty"` // application/json, application/x-www-form-urlencoded
	Active      bool           `json:"active"`
	InsecureSSL bool           `json:"insecure_ssl,omitempty"`
}

type HookResponse struct {
	ID          string         `json:"id"`
	URL         string         `json:"url"`
	Events      []WebhookEvent `json:"events"`
	ContentType string         `json:"content_type"`
	Active      bool           `json:"active"`
	InsecureSSL bool           `json:"insecure_ssl"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	LastResponse *HookDelivery `json:"last_response,omitempty"`
}

type HookListResponse struct {
	Hooks []HookResponse `json:"hooks"`
	Count int            `json:"count"`
}

type HookDelivery struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Event       WebhookEvent `json:"event"`
	StatusCode  int       `json:"status_code"`
	Duration    int64     `json:"duration_ms"`
	Request     string    `json:"request"`
	Response    string    `json:"response"`
	Delivered   bool      `json:"delivered"`
	CreatedAt   time.Time `json:"created_at"`
}

// Event payload structures

type EventPayload struct {
	Event      WebhookEvent    `json:"event"`
	Repository string          `json:"repository"`
	Timestamp  time.Time       `json:"timestamp"`
	Actor      EventActor      `json:"actor"`
	Data       interface{}     `json:"data"`
}

type EventActor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type PushEventData struct {
	Ref       string          `json:"ref"`
	Before    string          `json:"before"`
	After     string          `json:"after"`
	Commits   []CommitSummary `json:"commits"`
	Head      CommitSummary   `json:"head_commit"`
	Size      int             `json:"size"`
	Distinct  int             `json:"distinct_size"`
}

type CommitSummary struct {
	Hash      string    `json:"id"`
	Message   string    `json:"message"`
	Author    EventActor `json:"author"`
	Timestamp time.Time `json:"timestamp"`
	Added     []string  `json:"added"`
	Removed   []string  `json:"removed"`
	Modified  []string  `json:"modified"`
}

type BranchEventData struct {
	Ref    string `json:"ref"`
	Action string `json:"action"` // created, deleted
}

type TagEventData struct {
	Ref     string `json:"ref"`
	Action  string `json:"action"` // created, deleted
	TagName string `json:"tag_name"`
}

type MergeEventData struct {
	From      string        `json:"from"`
	To        string        `json:"to"`
	Commit    CommitSummary `json:"merge_commit"`
	Conflicts bool          `json:"had_conflicts"`
}

type StashEventData struct {
	Action  string `json:"action"` // created, applied, dropped
	StashID string `json:"stash_id"`
	Message string `json:"message"`
}

// Server-Sent Events types

type EventStreamResponse struct {
	Event WebhookEvent `json:"event"`
	Data  interface{}  `json:"data"`
	ID    string       `json:"id"`
}

// Hook execution types

type HookType string

const (
	HookPreCommit  HookType = "pre-commit"
	HookPostCommit HookType = "post-commit"
	HookPrePush    HookType = "pre-push"
	HookPostPush   HookType = "post-push"
	HookPreMerge   HookType = "pre-merge"
	HookPostMerge  HookType = "post-merge"
)

type HookExecutionRequest struct {
	Script      string            `json:"script" binding:"required"`
	Environment map[string]string `json:"environment,omitempty"`
	Timeout     int               `json:"timeout,omitempty"` // seconds, default 30
}

type HookExecutionResponse struct {
	Success    bool              `json:"success"`
	ExitCode   int               `json:"exit_code"`
	Output     string            `json:"output"`
	Error      string            `json:"error,omitempty"`
	Duration   int64             `json:"duration_ms"`
	Environment map[string]string `json:"environment"`
}