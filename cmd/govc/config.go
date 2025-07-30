package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

type CLIConfig struct {
	DefaultServer   string            `json:"default_server,omitempty"`
	DefaultTimeout  int               `json:"default_timeout,omitempty"`
	EnableMetrics   bool              `json:"enable_metrics,omitempty"`
	EnableLogging   bool              `json:"enable_logging,omitempty"`
	LogLevel        string            `json:"log_level,omitempty"`
	Editor          string            `json:"editor,omitempty"`
	ColorOutput     bool              `json:"color_output,omitempty"`
	Aliases         map[string]string `json:"aliases,omitempty"`
	DefaultAuthor   Author            `json:"default_author,omitempty"`
	AutoAuth        bool              `json:"auto_auth,omitempty"`
	MemoryRepoPath  string            `json:"memory_repo_path,omitempty"`
}

type Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

var (
	configCmd = &cobra.Command{
		Use:   "config",
		Short: "Manage govc configuration",
	}

	configGetCmd = &cobra.Command{
		Use:   "get [key]",
		Short: "Get configuration value",
		Args:  cobra.ExactArgs(1),
		Run:   runConfigGet,
	}

	configSetCmd = &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set configuration value",
		Args:  cobra.ExactArgs(2),
		Run:   runConfigSet,
	}

	configListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all configuration values",
		Run:   runConfigList,
	}

	configResetCmd = &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration to defaults",
		Run:   runConfigReset,
	}
)

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configResetCmd)
}

func getConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "govc", "config.json")
}

func loadCLIConfig() (*CLIConfig, error) {
	configPath := getConfigFilePath()
	if configPath == "" {
		return getDefaultConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return getDefaultConfig(), nil
		}
		return nil, err
	}

	var config CLIConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Apply defaults for missing values
	defaults := getDefaultConfig()
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = defaults.DefaultTimeout
	}
	if config.LogLevel == "" {
		config.LogLevel = defaults.LogLevel
	}
	if config.Aliases == nil {
		config.Aliases = make(map[string]string)
	}

	return &config, nil
}

func saveCLIConfig(config *CLIConfig) error {
	configPath := getConfigFilePath()
	if configPath == "" {
		return fmt.Errorf("unable to determine config path")
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func getDefaultConfig() *CLIConfig {
	return &CLIConfig{
		DefaultTimeout: 30,
		EnableMetrics:  false,
		EnableLogging:  false,
		LogLevel:       "info",
		Editor:         os.Getenv("EDITOR"),
		ColorOutput:    true,
		Aliases:        make(map[string]string),
		AutoAuth:       true,
	}
}

func runConfigGet(cmd *cobra.Command, args []string) {
	key := args[0]
	config, err := loadCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	switch key {
	case "default_server":
		fmt.Println(config.DefaultServer)
	case "default_timeout":
		fmt.Println(config.DefaultTimeout)
	case "enable_metrics":
		fmt.Println(config.EnableMetrics)
	case "enable_logging":
		fmt.Println(config.EnableLogging)
	case "log_level":
		fmt.Println(config.LogLevel)
	case "editor":
		fmt.Println(config.Editor)
	case "color_output":
		fmt.Println(config.ColorOutput)
	case "auto_auth":
		fmt.Println(config.AutoAuth)
	case "memory_repo_path":
		fmt.Println(config.MemoryRepoPath)
	case "default_author.name":
		fmt.Println(config.DefaultAuthor.Name)
	case "default_author.email":
		fmt.Println(config.DefaultAuthor.Email)
	default:
		if alias, ok := config.Aliases[key]; ok {
			fmt.Printf("alias.%s=%s\n", key, alias)
		} else {
			fmt.Fprintf(os.Stderr, "Unknown configuration key: %s\n", key)
			os.Exit(1)
		}
	}
}

func runConfigSet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := args[1]

	config, err := loadCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	switch key {
	case "default_server":
		config.DefaultServer = value
	case "default_timeout":
		var timeout int
		fmt.Sscanf(value, "%d", &timeout)
		config.DefaultTimeout = timeout
	case "enable_metrics":
		config.EnableMetrics = value == "true"
	case "enable_logging":
		config.EnableLogging = value == "true"
	case "log_level":
		config.LogLevel = value
	case "editor":
		config.Editor = value
	case "color_output":
		config.ColorOutput = value == "true"
	case "auto_auth":
		config.AutoAuth = value == "true"
	case "memory_repo_path":
		config.MemoryRepoPath = value
	case "default_author.name":
		config.DefaultAuthor.Name = value
	case "default_author.email":
		config.DefaultAuthor.Email = value
	default:
		if len(key) > 6 && key[:6] == "alias." {
			aliasName := key[6:]
			config.Aliases[aliasName] = value
		} else {
			fmt.Fprintf(os.Stderr, "Unknown configuration key: %s\n", key)
			os.Exit(1)
		}
	}

	if err := saveCLIConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Configuration updated: %s = %s\n", key, value)
}

func runConfigList(cmd *cobra.Command, args []string) {
	config, err := loadCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("# govc configuration")
	fmt.Printf("default_server = %s\n", config.DefaultServer)
	fmt.Printf("default_timeout = %d\n", config.DefaultTimeout)
	fmt.Printf("enable_metrics = %v\n", config.EnableMetrics)
	fmt.Printf("enable_logging = %v\n", config.EnableLogging)
	fmt.Printf("log_level = %s\n", config.LogLevel)
	fmt.Printf("editor = %s\n", config.Editor)
	fmt.Printf("color_output = %v\n", config.ColorOutput)
	fmt.Printf("auto_auth = %v\n", config.AutoAuth)
	fmt.Printf("memory_repo_path = %s\n", config.MemoryRepoPath)
	
	if config.DefaultAuthor.Name != "" || config.DefaultAuthor.Email != "" {
		fmt.Printf("\n# Default author\n")
		fmt.Printf("default_author.name = %s\n", config.DefaultAuthor.Name)
		fmt.Printf("default_author.email = %s\n", config.DefaultAuthor.Email)
	}

	if len(config.Aliases) > 0 {
		fmt.Printf("\n# Aliases\n")
		for name, cmd := range config.Aliases {
			fmt.Printf("alias.%s = %s\n", name, cmd)
		}
	}
}

func runConfigReset(cmd *cobra.Command, args []string) {
	fmt.Print("Are you sure you want to reset all configuration to defaults? [y/N]: ")
	var response string
	fmt.Scanln(&response)
	
	if response != "y" && response != "Y" {
		fmt.Println("Cancelled")
		return
	}

	config := getDefaultConfig()
	if err := saveCLIConfig(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Configuration reset to defaults")
}