package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	userCmd = &cobra.Command{
		Use:   "user",
		Short: "Manage users",
	}

	userListCmd = &cobra.Command{
		Use:   "list",
		Short: "List all users",
		Run:   runUserList,
	}

	userCreateCmd = &cobra.Command{
		Use:   "create [username]",
		Short: "Create a new user",
		Args:  cobra.ExactArgs(1),
		Run:   runUserCreate,
	}

	userDeleteCmd = &cobra.Command{
		Use:   "delete [username]",
		Short: "Delete a user",
		Args:  cobra.ExactArgs(1),
		Run:   runUserDelete,
	}

	userGetCmd = &cobra.Command{
		Use:   "get [username]",
		Short: "Get user details",
		Args:  cobra.ExactArgs(1),
		Run:   runUserGet,
	}

	userUpdateCmd = &cobra.Command{
		Use:   "update [username]",
		Short: "Update user information",
		Args:  cobra.ExactArgs(1),
		Run:   runUserUpdate,
	}

	userPasswordCmd = &cobra.Command{
		Use:   "password [username]",
		Short: "Change user password",
		Args:  cobra.MaximumNArgs(1),
		Run:   runUserPassword,
	}

	userRoleCmd = &cobra.Command{
		Use:   "role",
		Short: "Manage user roles",
	}

	userRoleAddCmd = &cobra.Command{
		Use:   "add [username] [role]",
		Short: "Add role to user",
		Args:  cobra.ExactArgs(2),
		Run:   runUserRoleAdd,
	}

	userRoleRemoveCmd = &cobra.Command{
		Use:   "remove [username] [role]",
		Short: "Remove role from user",
		Args:  cobra.ExactArgs(2),
		Run:   runUserRoleRemove,
	}
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	IsActive  bool      `json:"is_active"`
	IsAdmin   bool      `json:"is_admin"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func init() {
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userGetCmd)
	userCmd.AddCommand(userUpdateCmd)
	userCmd.AddCommand(userPasswordCmd)
	userCmd.AddCommand(userRoleCmd)

	userRoleCmd.AddCommand(userRoleAddCmd)
	userRoleCmd.AddCommand(userRoleRemoveCmd)

	userCreateCmd.Flags().String("email", "", "User email address")
	userCreateCmd.Flags().Bool("admin", false, "Create user as admin")
	userCreateCmd.Flags().StringSlice("roles", []string{}, "Initial roles for the user")

	userUpdateCmd.Flags().String("email", "", "New email address")
	userUpdateCmd.Flags().Bool("active", true, "Set user active status")
}

func runUserList(cmd *cobra.Command, args []string) {
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
	url := ctx.ServerURL + "/api/v1/users"

	resp, err := makeAuthRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing users: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to list users: %s\n", string(body))
		os.Exit(1)
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(users) == 0 {
		fmt.Println("No users found")
		return
	}

	fmt.Printf("%-20s %-30s %-8s %-8s %-30s %s\n", "USERNAME", "EMAIL", "ACTIVE", "ADMIN", "ROLES", "CREATED")
	for _, user := range users {
		active := "Yes"
		if !user.IsActive {
			active = "No"
		}
		admin := "No"
		if user.IsAdmin {
			admin = "Yes"
		}
		roles := strings.Join(user.Roles, ", ")
		if len(roles) > 27 {
			roles = roles[:27] + "..."
		}
		fmt.Printf("%-20s %-30s %-8s %-8s %-30s %s\n",
			user.Username,
			user.Email,
			active,
			admin,
			roles,
			user.CreatedAt.Format("2006-01-02"),
		)
	}
}

func runUserCreate(cmd *cobra.Command, args []string) {
	username := args[0]
	email, _ := cmd.Flags().GetString("email")
	isAdmin, _ := cmd.Flags().GetBool("admin")
	roles, _ := cmd.Flags().GetStringSlice("roles")

	if email == "" {
		fmt.Print("Email: ")
		fmt.Scanln(&email)
	}

	fmt.Print("Password: ")
	passwordBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
	password := string(passwordBytes)

	fmt.Print("Confirm password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	if password != string(confirmBytes) {
		fmt.Fprintf(os.Stderr, "Passwords do not match\n")
		os.Exit(1)
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
	url := ctx.ServerURL + "/api/v1/users"

	reqBody := map[string]interface{}{
		"username": username,
		"email":    email,
		"password": password,
		"is_admin": isAdmin,
		"roles":    roles,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := makeAuthRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating user: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to create user: %s\n", string(body))
		os.Exit(1)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("User '%s' created successfully\n", user.Username)
	fmt.Printf("ID: %s\n", user.ID)
	fmt.Printf("Email: %s\n", user.Email)
	if user.IsAdmin {
		fmt.Println("Admin: Yes")
	}
	if len(user.Roles) > 0 {
		fmt.Printf("Roles: %s\n", strings.Join(user.Roles, ", "))
	}
}

func runUserDelete(cmd *cobra.Command, args []string) {
	username := args[0]

	fmt.Printf("Are you sure you want to delete user '%s'? [y/N]: ", username)
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
	url := ctx.ServerURL + "/api/v1/users/" + username

	resp, err := makeAuthRequest("DELETE", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deleting user: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to delete user: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("User '%s' deleted successfully\n", username)
}

func runUserGet(cmd *cobra.Command, args []string) {
	username := args[0]

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
	url := ctx.ServerURL + "/api/v1/users/" + username

	resp, err := makeAuthRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting user: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to get user: %s\n", string(body))
		os.Exit(1)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("ID: %s\n", user.ID)
	fmt.Printf("Email: %s\n", user.Email)
	fmt.Printf("Active: %v\n", user.IsActive)
	fmt.Printf("Admin: %v\n", user.IsAdmin)
	if len(user.Roles) > 0 {
		fmt.Printf("Roles: %s\n", strings.Join(user.Roles, ", "))
	} else {
		fmt.Println("Roles: none")
	}
	fmt.Printf("Created: %s\n", user.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", user.UpdatedAt.Format("2006-01-02 15:04:05"))
}

func runUserUpdate(cmd *cobra.Command, args []string) {
	username := args[0]
	email, _ := cmd.Flags().GetString("email")
	active, _ := cmd.Flags().GetBool("active")

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
	url := ctx.ServerURL + "/api/v1/users/" + username

	reqBody := make(map[string]interface{})
	if email != "" {
		reqBody["email"] = email
	}
	if cmd.Flags().Changed("active") {
		reqBody["is_active"] = active
	}

	if len(reqBody) == 0 {
		fmt.Println("No updates specified")
		return
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := makeAuthRequest("PATCH", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error updating user: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to update user: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("User '%s' updated successfully\n", username)
}

func runUserPassword(cmd *cobra.Command, args []string) {
	var username string
	if len(args) > 0 {
		username = args[0]
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

	// If no username specified, change current user's password
	if username == "" {
		if ctx, ok := authConfig.Contexts[authConfig.Current]; ok {
			username = ctx.Username
		}
	}

	fmt.Print("Current password: ")
	currentBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	fmt.Print("New password: ")
	newBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	fmt.Print("Confirm new password: ")
	confirmBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError reading password: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()

	if string(newBytes) != string(confirmBytes) {
		fmt.Fprintf(os.Stderr, "Passwords do not match\n")
		os.Exit(1)
	}

	ctx := authConfig.Contexts[authConfig.Current]
	url := ctx.ServerURL + "/api/v1/users/" + username + "/password"

	reqBody := map[string]string{
		"current_password": string(currentBytes),
		"new_password":     string(newBytes),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := makeAuthRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error changing password: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to change password: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Println("Password changed successfully")
}

func runUserRoleAdd(cmd *cobra.Command, args []string) {
	username := args[0]
	role := args[1]

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
	url := ctx.ServerURL + "/api/v1/users/" + username + "/roles"

	reqBody := map[string]string{
		"role": role,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
		os.Exit(1)
	}

	resp, err := makeAuthRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error adding role: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to add role: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("Role '%s' added to user '%s'\n", role, username)
}

func runUserRoleRemove(cmd *cobra.Command, args []string) {
	username := args[0]
	role := args[1]

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
	url := ctx.ServerURL + "/api/v1/users/" + username + "/roles/" + role

	resp, err := makeAuthRequest("DELETE", url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error removing role: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "Failed to remove role: %s\n", string(body))
		os.Exit(1)
	}

	fmt.Printf("Role '%s' removed from user '%s'\n", role, username)
}
