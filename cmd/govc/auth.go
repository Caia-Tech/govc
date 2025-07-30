package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type AuthConfig struct {
	ServerURL string             `json:"server_url"`
	Token     string             `json:"token,omitempty"`
	APIKey    string             `json:"api_key,omitempty"`
	Username  string             `json:"username,omitempty"`
	ExpiresAt *time.Time         `json:"expires_at,omitempty"`
	Contexts  map[string]Context `json:"contexts,omitempty"`
	Current   string             `json:"current_context,omitempty"`
}

type Context struct {
	Name      string     `json:"name"`
	ServerURL string     `json:"server_url"`
	Token     string     `json:"token,omitempty"`
	APIKey    string     `json:"api_key,omitempty"`
	Username  string     `json:"username,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

var (
	authCmd = &cobra.Command{
		Use:   "auth",
		Short: "Authentication and authorization commands",
	}

	loginCmd = &cobra.Command{
		Use:   "login",
		Short: "Login to govc server",
		Run:   runLogin,
	}

	logoutCmd = &cobra.Command{
		Use:   "logout",
		Short: "Logout from govc server",
		Run:   runLogout,
	}

	whoamiCmd = &cobra.Command{
		Use:   "whoami",
		Short: "Display current user information",
		Run:   runWhoami,
	}

	tokenCmd = &cobra.Command{
		Use:   "token",
		Short: "Manage authentication tokens",
	}

	tokenListCmd = &cobra.Command{
		Use:   "list",
		Short: "List saved tokens",
		Run:   runTokenList,
	}

	apiKeyCmd = &cobra.Command{
		Use:   "apikey",
		Short: "Manage API keys",
	}

	apiKeyCreateCmd = &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new API key",
		Args:  cobra.ExactArgs(1),
		Run:   runAPIKeyCreate,
	}

	apiKeyListCmd = &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		Run:   runAPIKeyList,
	}

	apiKeyDeleteCmd = &cobra.Command{
		Use:   "delete [key-id]",
		Short: "Delete an API key",
		Args:  cobra.ExactArgs(1),
		Run:   runAPIKeyDelete,
	}
)

func init() {
	authCmd.AddCommand(loginCmd)
	authCmd.AddCommand(logoutCmd)
	authCmd.AddCommand(whoamiCmd)
	authCmd.AddCommand(tokenCmd)
	authCmd.AddCommand(apiKeyCmd)

	tokenCmd.AddCommand(tokenListCmd)

	apiKeyCmd.AddCommand(apiKeyCreateCmd)
	apiKeyCmd.AddCommand(apiKeyListCmd)
	apiKeyCmd.AddCommand(apiKeyDeleteCmd)

	loginCmd.Flags().String("server", "", "govc server URL")
	loginCmd.Flags().String("username", "", "Username")
	loginCmd.Flags().String("context", "default", "Context name to save credentials")
	loginCmd.Flags().Bool("no-save", false, "Don't save credentials to disk")
}

func getConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "govc", "auth.json")
}

func loadAuthConfig() (*AuthConfig, error) {
	configPath := getConfigPath()
	if configPath == "" {
		return &AuthConfig{Contexts: make(map[string]Context)}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &AuthConfig{Contexts: make(map[string]Context)}, nil
		}
		return nil, err
	}

	var config AuthConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.Contexts == nil {
		config.Contexts = make(map[string]Context)
	}

	return &config, nil
}

func saveAuthConfig(config *AuthConfig) error {
	configPath := getConfigPath()
	if configPath == "" {
		return fmt.Errorf("unable to determine config path")
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

func runLogin(cmd *cobra.Command, args []string) {
	server, _ := cmd.Flags().GetString("server")
	username, _ := cmd.Flags().GetString("username")
	contextName, _ := cmd.Flags().GetString("context")
	noSave, _ := cmd.Flags().GetBool("no-save")

	if server == "" {
		fmt.Print("Server URL: ")
		fmt.Scanln(&server)
	}

	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "https://" + server
	}

	if username == "" {
		fmt.Print("Username: ")
		fmt.Scanln(&username)
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	password := string(passwordBytes)

	req := LoginRequest{
		Username: username,
		Password: password,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := http.Post(server+"/api/v1/auth/login", "application/json", strings.NewReader(string(reqBody)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to server: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Login failed: %s\n", string(body))
		os.Exit(1)
	}

	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully logged in as %s\n", loginResp.User.Username)

	if !noSave {
		config, err := loadAuthConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not load config: %v\n", err)
			return
		}

		ctx := Context{
			Name:      contextName,
			ServerURL: server,
			Token:     loginResp.Token,
			Username:  loginResp.User.Username,
			ExpiresAt: &loginResp.ExpiresAt,
		}

		config.Contexts[contextName] = ctx
		config.Current = contextName

		// Also save to top-level for backward compatibility
		config.ServerURL = server
		config.Token = loginResp.Token
		config.Username = loginResp.User.Username
		config.ExpiresAt = &loginResp.ExpiresAt

		if err := saveAuthConfig(config); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not save credentials: %v\n", err)
		} else {
			fmt.Printf("Credentials saved to context '%s'\n", contextName)
		}
	}
}

func runLogout(cmd *cobra.Command, args []string) {
	config, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if config.Current == "" {
		fmt.Println("Not logged in")
		return
	}

	// Clear current context
	delete(config.Contexts, config.Current)
	config.Current = ""
	config.Token = ""
	config.APIKey = ""
	config.Username = ""
	config.ExpiresAt = nil

	if err := saveAuthConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully logged out")
}

func runWhoami(cmd *cobra.Command, args []string) {
	config, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if config.Current == "" || config.Token == "" {
		fmt.Println("Not logged in")
		os.Exit(1)
	}

	ctx, ok := config.Contexts[config.Current]
	if !ok {
		fmt.Println("Current context not found")
		os.Exit(1)
	}

	fmt.Printf("Current context: %s\n", config.Current)
	fmt.Printf("Server: %s\n", ctx.ServerURL)
	fmt.Printf("Username: %s\n", ctx.Username)

	if ctx.ExpiresAt != nil {
		if time.Now().After(*ctx.ExpiresAt) {
			fmt.Printf("Token: EXPIRED (expired %s)\n", ctx.ExpiresAt.Format(time.RFC3339))
		} else {
			fmt.Printf("Token expires: %s\n", ctx.ExpiresAt.Format(time.RFC3339))
		}
	}

	// Verify token with server
	req, err := http.NewRequest("GET", ctx.ServerURL+"/api/v1/auth/me", nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+ctx.Token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Status: Unable to verify (server unreachable)\n")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Status: Active\n")
	} else {
		fmt.Printf("Status: Invalid token\n")
	}
}

func runTokenList(cmd *cobra.Command, args []string) {
	config, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(config.Contexts) == 0 {
		fmt.Println("No saved tokens")
		return
	}

	fmt.Printf("%-20s %-40s %-20s %s\n", "CONTEXT", "SERVER", "USERNAME", "STATUS")
	for name, ctx := range config.Contexts {
		status := "Active"
		if ctx.ExpiresAt != nil && time.Now().After(*ctx.ExpiresAt) {
			status = "Expired"
		}
		current := ""
		if name == config.Current {
			current = "* "
		}
		fmt.Printf("%s%-18s %-40s %-20s %s\n", current, name, ctx.ServerURL, ctx.Username, status)
	}
}

func runAPIKeyCreate(cmd *cobra.Command, args []string) {
	name := args[0]

	config, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if config.Current == "" || config.Token == "" {
		fmt.Fprintf(os.Stderr, "Not logged in. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := config.Contexts[config.Current]

	reqBody := fmt.Sprintf(`{"name": "%s"}`, name)
	req, err := http.NewRequest("POST", ctx.ServerURL+"/api/v1/auth/keys", strings.NewReader(reqBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Authorization", "Bearer "+ctx.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating API key: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to create API key: %s\n", string(body))
		os.Exit(1)
	}

	var result struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Key       string    `json:"key"`
		CreatedAt time.Time `json:"created_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("API key created successfully!\n")
	fmt.Printf("ID: %s\n", result.ID)
	fmt.Printf("Name: %s\n", result.Name)
	fmt.Printf("Key: %s\n", result.Key)
	fmt.Printf("\nIMPORTANT: Save this key securely. It won't be shown again.\n")
}

func runAPIKeyList(cmd *cobra.Command, args []string) {
	config, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if config.Current == "" || config.Token == "" {
		fmt.Fprintf(os.Stderr, "Not logged in. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := config.Contexts[config.Current]

	req, err := http.NewRequest("GET", ctx.ServerURL+"/api/v1/auth/keys", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Authorization", "Bearer "+ctx.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing API keys: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to list API keys: %s\n", string(body))
		os.Exit(1)
	}

	var keys []struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
		LastUsed  time.Time `json:"last_used,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(keys) == 0 {
		fmt.Println("No API keys found")
		return
	}

	fmt.Printf("%-36s %-30s %-20s %s\n", "ID", "NAME", "CREATED", "LAST USED")
	for _, key := range keys {
		lastUsed := "Never"
		if !key.LastUsed.IsZero() {
			lastUsed = key.LastUsed.Format("2006-01-02 15:04")
		}
		fmt.Printf("%-36s %-30s %-20s %s\n",
			key.ID,
			key.Name,
			key.CreatedAt.Format("2006-01-02 15:04"),
			lastUsed,
		)
	}
}

func runAPIKeyDelete(cmd *cobra.Command, args []string) {
	keyID := args[0]

	config, err := loadAuthConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if config.Current == "" || config.Token == "" {
		fmt.Fprintf(os.Stderr, "Not logged in. Please run 'govc auth login' first.\n")
		os.Exit(1)
	}

	ctx := config.Contexts[config.Current]

	req, err := http.NewRequest("DELETE", ctx.ServerURL+"/api/v1/auth/keys/"+keyID, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Authorization", "Bearer "+ctx.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting API key: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to delete API key: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("API key %s deleted successfully\n", keyID)
}