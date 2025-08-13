package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Caia-Tech/govc"
	"github.com/spf13/cobra"
)

var (
	version = "0.1.0"
	rootCmd = &cobra.Command{
		Use:   "govc",
		Short: "Go Version Control - A Git implementation in pure Go",
		Long: `govc is a Git implementation written in pure Go.
It provides memory-first operations with no CGO dependencies.`,
		Version: version,
	}
)

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(commitCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(logCmd)
	rootCmd.AddCommand(branchCmd)
	rootCmd.AddCommand(checkoutCmd)
	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(remoteCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
}

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a new govc repository",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Initialize repository at the specified path
		_, err = govc.LoadRepository(absPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing repository: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Initialized empty govc repository in %s/.govc\n", absPath)
	},
}

var addCmd = &cobra.Command{
	Use:   "add [files...]",
	Short: "Add files to the staging area",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		for _, path := range args {
			// Read file content
			content, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading file %s: %v\n", path, err)
				continue
			}

			// Add file to staging area
			stagingArea := repo.GetStagingArea()
			if err := stagingArea.Add(path, content); err != nil {
				fmt.Fprintf(os.Stderr, "Error adding file %s: %v\n", path, err)
				continue
			}

			fmt.Printf("Added %s\n", path)
		}
	},
}

var commitCmd = &cobra.Command{
	Use:   "commit -m <message>",
	Short: "Record changes to the repository",
	Run: func(cmd *cobra.Command, args []string) {
		message, _ := cmd.Flags().GetString("message")
		if message == "" {
			fmt.Fprintf(os.Stderr, "Error: commit message required\n")
			os.Exit(1)
		}

		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		commit, err := repo.Commit(message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating commit: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("[%s] %s\n", commit.Hash()[:7], message)
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the working tree status",
	Run: func(cmd *cobra.Command, args []string) {
		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		status, err := repo.Status()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting status: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("On branch %s\n\n", status.Branch)

		if len(status.Staged) > 0 {
			fmt.Println("Changes to be committed:")
			for _, file := range status.Staged {
				fmt.Printf("  new file:   %s\n", file)
			}
			fmt.Println()
		}

		if len(status.Modified) > 0 {
			fmt.Println("Changes not staged for commit:")
			for _, file := range status.Modified {
				fmt.Printf("  modified:   %s\n", file)
			}
			fmt.Println()
		}

		if len(status.Untracked) > 0 {
			fmt.Println("Untracked files:")
			for _, file := range status.Untracked {
				fmt.Printf("  %s\n", file)
			}
			fmt.Println()
		}

		if len(status.Staged) == 0 && len(status.Modified) == 0 && len(status.Untracked) == 0 {
			fmt.Println("nothing to commit, working tree clean")
		}
	},
}

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show commit logs",
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		oneline, _ := cmd.Flags().GetBool("oneline")

		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		commits, err := repo.Log(limit)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting log: %v\n", err)
			os.Exit(1)
		}

		for _, commit := range commits {
			if oneline {
				fmt.Printf("%s %s\n", commit.Hash()[:7], commit.Message)
			} else {
				fmt.Printf("commit %s\n", commit.Hash())
				fmt.Printf("Author: %s <%s>\n", commit.Author.Name, commit.Author.Email)
				fmt.Printf("Date:   %s\n\n", commit.Author.Time.Format("Mon Jan 2 15:04:05 2006 -0700"))
				fmt.Printf("    %s\n\n", commit.Message)
			}
		}
	},
}

var branchCmd = &cobra.Command{
	Use:   "branch [name]",
	Short: "List, create, or delete branches",
	Run: func(cmd *cobra.Command, args []string) {
		deleteFlag, _ := cmd.Flags().GetString("delete")
		// List branches if no args provided
		_, _ = cmd.Flags().GetBool("list") // Not used, branches are listed when no args provided

		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if deleteFlag != "" {
			builder := repo.Branch(deleteFlag)
			if err := builder.Delete(); err != nil {
				fmt.Fprintf(os.Stderr, "Error deleting branch: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Deleted branch %s\n", deleteFlag)
			return
		}

		if len(args) > 0 {
			branchName := args[0]
			builder := repo.Branch(branchName)
			if err := builder.Create(); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating branch: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Created branch %s\n", branchName)
			return
		}

		branches, err := repo.ListBranches()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing branches: %v\n", err)
			os.Exit(1)
		}

		currentBranch, _ := repo.CurrentBranch()

		for _, branchName := range branches {
			name := filepath.Base(branchName)
			if name == currentBranch {
				fmt.Printf("* %s\n", name)
			} else {
				fmt.Printf("  %s\n", name)
			}
		}
	},
}

var checkoutCmd = &cobra.Command{
	Use:   "checkout <branch>",
	Short: "Switch branches or restore working tree files",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		createFlag, _ := cmd.Flags().GetBool("create")

		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		branchName := args[0]

		if createFlag {
			builder := repo.Branch(branchName)
			if err := builder.Create(); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating branch: %v\n", err)
				os.Exit(1)
			}
		}

		if err := repo.Checkout(branchName); err != nil {
			fmt.Fprintf(os.Stderr, "Error checking out: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Switched to branch '%s'\n", branchName)
	},
}

var mergeCmd = &cobra.Command{
	Use:   "merge <branch>",
	Short: "Join two or more development histories together",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		branchName := args[0]
		currentBranch, _ := repo.CurrentBranch()
		if err := repo.Merge(branchName, currentBranch); err != nil {
			fmt.Fprintf(os.Stderr, "Error merging: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Merged branch '%s'\n", branchName)
	},
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP Git server",
	Run: func(cmd *cobra.Command, args []string) {
		addr, _ := cmd.Flags().GetString("addr")

		repo, err := openRepo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Starting govc server on %s\n", addr)
		if err := repo.Serve(addr); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	commitCmd.Flags().StringP("message", "m", "", "Commit message")
	commitCmd.MarkFlagRequired("message")

	logCmd.Flags().IntP("limit", "n", 10, "Limit the number of commits")
	logCmd.Flags().Bool("oneline", false, "Show commits in one line")

	branchCmd.Flags().StringP("delete", "d", "", "Delete a branch")
	branchCmd.Flags().BoolP("list", "l", false, "List branches")

	checkoutCmd.Flags().BoolP("create", "b", false, "Create a new branch")

	serveCmd.Flags().String("addr", ":8080", "Server address")
}

func openRepo() (*govc.Repository, error) {
	// Find the repository root (current directory for now)
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}
	
	// Load or create repository at current path
	repo, err := govc.LoadRepository(cwd)
	if err != nil {
		return nil, fmt.Errorf("failed to load repository: %w", err)
	}
	
	return repo, nil
}

func findRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		govcPath := filepath.Join(cwd, ".govc")
		if _, err := os.Stat(govcPath); err == nil {
			return cwd, nil
		}

		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return "", fmt.Errorf("not a govc repository")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
