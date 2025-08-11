package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Caia-Tech/govc"
	"github.com/spf13/cobra"
)

var (
	remoteCmd = &cobra.Command{
		Use:   "remote",
		Short: "Manage remote repositories",
	}

	remoteListCmd = &cobra.Command{
		Use:   "list",
		Short: "List remote repositories",
		Run:   runRemoteList,
	}

	remoteCreateCmd = &cobra.Command{
		Use:   "create [name]",
		Short: "Create a remote repository",
		Args:  cobra.ExactArgs(1),
		Run:   runRemoteCreate,
	}

	remoteDeleteCmd = &cobra.Command{
		Use:   "delete [name]",
		Short: "Delete a remote repository",
		Args:  cobra.ExactArgs(1),
		Run:   runRemoteDelete,
	}

	remoteCloneCmd = &cobra.Command{
		Use:   "clone [repo-id] [path]",
		Short: "Clone a remote repository",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runRemoteClone,
	}

	remotePushCmd = &cobra.Command{
		Use:   "push [branch]",
		Short: "Push changes to remote repository",
		Run:   runRemotePush,
	}

	remotePullCmd = &cobra.Command{
		Use:   "pull [branch]",
		Short: "Pull changes from remote repository",
		Run:   runRemotePull,
	}
)

func init() {
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteCreateCmd)
	remoteCmd.AddCommand(remoteDeleteCmd)
	remoteCmd.AddCommand(remoteCloneCmd)
	remoteCmd.AddCommand(remotePushCmd)
	remoteCmd.AddCommand(remotePullCmd)

	remoteCreateCmd.Flags().Bool("memory", false, "Create memory-only repository")
	remoteCreateCmd.Flags().String("description", "", "Repository description")
}

func getAuthHeaders(config *AuthConfig) map[string]string {
	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"

	if config.Current != "" {
		if ctx, ok := config.Contexts[config.Current]; ok {
			if ctx.Token != "" {
				headers["Authorization"] = "Bearer " + ctx.Token
			} else if ctx.APIKey != "" {
				headers["X-API-Key"] = ctx.APIKey
			}
		}
	}

	return headers
}

func makeAuthRequest(method, url string, body io.Reader) (*http.Response, error) {
	authConfig, err := loadAuthConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load auth config: %w", err)
	}

	if authConfig.Current == "" {
		return nil, fmt.Errorf("not authenticated. Please run 'govc auth login' first")
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	headers := getAuthHeaders(authConfig)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

func runRemoteList(cmd *cobra.Command, args []string) {
	authConfig, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading auth config: %v\n", err)
		os.Exit(1)
	}

	if authConfig.Current == "" {
		fmt.Fprintf(os.Stderr, "Not authenticated. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := authConfig.Contexts[authConfig.Current]
	url := ctx.ServerURL + "/api/v1/repos"

	resp, err := makeAuthRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing repositories: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to list repositories: %s\n", string(body))
		os.Exit(1)
	}

	var repos []struct {
		ID          string    `json:"id"`
		Description string    `json:"description"`
		MemoryOnly  bool      `json:"memory_only"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found")
		return
	}

	fmt.Printf("%-30s %-10s %-50s %s\n", "ID", "TYPE", "DESCRIPTION", "UPDATED")
	for _, repo := range repos {
		repoType := "disk"
		if repo.MemoryOnly {
			repoType = "memory"
		}
		desc := repo.Description
		if len(desc) > 47 {
			desc = desc[:47] + "..."
		}
		fmt.Printf("%-30s %-10s %-50s %s\n",
			repo.ID,
			repoType,
			desc,
			repo.UpdatedAt.Format("2006-01-02 15:04"),
		)
	}
}

func runRemoteCreate(cmd *cobra.Command, args []string) {
	repoID := args[0]
	memoryOnly, _ := cmd.Flags().GetBool("memory")
	description, _ := cmd.Flags().GetString("description")

	authConfig, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading auth config: %v\n", err)
		os.Exit(1)
	}

	if authConfig.Current == "" {
		fmt.Fprintf(os.Stderr, "Not authenticated. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := authConfig.Contexts[authConfig.Current]
	url := ctx.ServerURL + "/api/v1/repos"

	reqBody := map[string]interface{}{
		"id":          repoID,
		"memory_only": memoryOnly,
		"description": description,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := makeAuthRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating repository: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to create repository: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("Repository '%s' created successfully\n", repoID)
}

func runRemoteDelete(cmd *cobra.Command, args []string) {
	repoID := args[0]

	fmt.Printf("Are you sure you want to delete repository '%s'? [y/N]: ", repoID)
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Cancelled")
		return
	}

	authConfig, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading auth config: %v\n", err)
		os.Exit(1)
	}

	if authConfig.Current == "" {
		fmt.Fprintf(os.Stderr, "Not authenticated. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := authConfig.Contexts[authConfig.Current]
	url := ctx.ServerURL + "/api/v1/repos/" + repoID

	resp, err := makeAuthRequest("DELETE", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting repository: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to delete repository: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("Repository '%s' deleted successfully\n", repoID)
}

func runRemoteClone(cmd *cobra.Command, args []string) {
	repoID := args[0]
	path := repoID
	if len(args) > 1 {
		path = args[1]
	}

	authConfig, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading auth config: %v\n", err)
		os.Exit(1)
	}

	if authConfig.Current == "" {
		fmt.Fprintf(os.Stderr, "Not authenticated. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := authConfig.Contexts[authConfig.Current]

	// Create local directory
	if err := os.MkdirAll(path, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize local repository
	_, err = govc.Init(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing repository: %v\n", err)
		os.Exit(1)
	}

	// Save remote configuration
	remoteConfig := map[string]interface{}{
		"url":    ctx.ServerURL,
		"repo":   repoID,
		"server": ctx.ServerURL,
	}

	configPath := filepath.Join(path, ".govc", "remote.json")
	configData, err := json.MarshalIndent(remoteConfig, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error saving remote config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing remote config: %v\n", err)
		os.Exit(1)
	}

	// Fetch repository data
	url := ctx.ServerURL + "/api/v1/repos/" + repoID

	resp, err := makeAuthRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching repository: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to fetch repository: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("Cloned repository '%s' to '%s'\n", repoID, path)
	fmt.Println("Note: Full clone functionality requires additional implementation")
}

func runRemotePush(cmd *cobra.Command, args []string) {
	branch := "main"
	if len(args) > 0 {
		branch = args[0]
	}

	_, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load remote configuration
	remoteConfigPath := filepath.Join(".govc", "remote.json")
	remoteData, err := os.ReadFile(remoteConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No remote configured. Use 'govc remote clone' first.\n")
		os.Exit(1)
	}

	var remoteConfig map[string]string
	if err := json.Unmarshal(remoteData, &remoteConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing remote config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Pushing branch '%s' to remote repository '%s'\n", branch, remoteConfig["repo"])
	fmt.Println("Note: Push functionality requires additional implementation")
}

func runRemotePull(cmd *cobra.Command, args []string) {
	branch := "main"
	if len(args) > 0 {
		branch = args[0]
	}

	_, err := openRepo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load remote configuration
	remoteConfigPath := filepath.Join(".govc", "remote.json")
	remoteData, err := os.ReadFile(remoteConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No remote configured. Use 'govc remote clone' first.\n")
		os.Exit(1)
	}

	var remoteConfig map[string]string
	if err := json.Unmarshal(remoteData, &remoteConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing remote config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Pulling branch '%s' from remote repository '%s'\n", branch, remoteConfig["repo"])
	fmt.Println("Note: Pull functionality requires additional implementation")
}
