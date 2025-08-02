package validation

import (
	"fmt"
	"net/mail"
	"path/filepath"
	"regexp"
	"strings"
)

// Common validation patterns
var (
	// Repository ID must be alphanumeric with hyphens, underscores
	repoIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	
	// Branch name validation (Git branch naming rules)
	branchNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9/_.-])*[a-zA-Z0-9]$`)
	
	// Tag name validation
	tagNamePattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9/_.-])*[a-zA-Z0-9]$`)
	
	// File path component validation (no path traversal)
	safePathPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	
	// Commit message validation (reasonable length, no control characters)
	commitMessagePattern = regexp.MustCompile(`^[\x20-\x7E\n\r\t]+$`)
	
	// Username validation
	usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{2,31}$`)
)

// ValidationError represents a validation error with field information
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on field '%s': %s", e.Field, e.Message)
}

// ValidateRepositoryID validates a repository ID
func ValidateRepositoryID(id string) error {
	if id == "" {
		return ValidationError{Field: "repo_id", Message: "repository ID cannot be empty"}
	}
	
	if len(id) > 64 {
		return ValidationError{Field: "repo_id", Message: "repository ID must not exceed 64 characters"}
	}
	
	if !repoIDPattern.MatchString(id) {
		return ValidationError{Field: "repo_id", Message: "repository ID must be alphanumeric with hyphens or underscores, starting with alphanumeric"}
	}
	
	// Prevent common reserved names
	reserved := []string{"api", "admin", "root", "system", "config", "health", "metrics", "swagger"}
	lowerID := strings.ToLower(id)
	for _, r := range reserved {
		if lowerID == r {
			return ValidationError{Field: "repo_id", Message: fmt.Sprintf("repository ID '%s' is reserved", id)}
		}
	}
	
	return nil
}

// ValidateBranchName validates a Git branch name
func ValidateBranchName(name string) error {
	if name == "" {
		return ValidationError{Field: "branch", Message: "branch name cannot be empty"}
	}
	
	if len(name) > 255 {
		return ValidationError{Field: "branch", Message: "branch name must not exceed 255 characters"}
	}
	
	// Check for invalid patterns
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return ValidationError{Field: "branch", Message: "branch name cannot start or end with hyphen"}
	}
	
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return ValidationError{Field: "branch", Message: "branch name cannot start or end with period"}
	}
	
	if strings.Contains(name, "..") {
		return ValidationError{Field: "branch", Message: "branch name cannot contain consecutive periods"}
	}
	
	if strings.Contains(name, "//") {
		return ValidationError{Field: "branch", Message: "branch name cannot contain consecutive slashes"}
	}
	
	// Check against Git's forbidden patterns
	forbidden := []string{"@{", "\\", "~", "^", ":", "?", "*", "[", " ", "\t", "\n"}
	for _, f := range forbidden {
		if strings.Contains(name, f) {
			return ValidationError{Field: "branch", Message: fmt.Sprintf("branch name cannot contain '%s'", f)}
		}
	}
	
	return nil
}

// ValidateFilePath validates a file path for security
func ValidateFilePath(path string) error {
	if path == "" {
		return ValidationError{Field: "path", Message: "file path cannot be empty"}
	}
	
	if len(path) > 4096 {
		return ValidationError{Field: "path", Message: "file path too long"}
	}
	
	// Clean the path and check for directory traversal
	cleaned := filepath.Clean(path)
	
	// Prevent path traversal attacks
	if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "../") {
		return ValidationError{Field: "path", Message: "path traversal detected"}
	}
	
	// Prevent absolute paths (both Unix and Windows style)
	if filepath.IsAbs(cleaned) || (len(path) >= 2 && path[1] == ':') {
		return ValidationError{Field: "path", Message: "absolute paths are not allowed"}
	}
	
	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return ValidationError{Field: "path", Message: "null bytes in path are not allowed"}
	}
	
	// Prevent access to hidden files at root level
	parts := strings.Split(cleaned, "/")
	if len(parts) > 0 && strings.HasPrefix(parts[0], ".") && parts[0] != "." {
		return ValidationError{Field: "path", Message: "access to hidden files at root level is not allowed"}
	}
	
	return nil
}

// ValidateCommitMessage validates a commit message
func ValidateCommitMessage(message string) error {
	if message == "" {
		return ValidationError{Field: "message", Message: "commit message cannot be empty"}
	}
	
	if len(message) > 100000 { // 100KB limit
		return ValidationError{Field: "message", Message: "commit message too long (max 100KB)"}
	}
	
	// Check for control characters except newline, carriage return, and tab
	if !commitMessagePattern.MatchString(message) {
		return ValidationError{Field: "message", Message: "commit message contains invalid control characters"}
	}
	
	return nil
}

// ValidateUsername validates a username
func ValidateUsername(username string) error {
	if username == "" {
		return ValidationError{Field: "username", Message: "username cannot be empty"}
	}
	
	if len(username) < 3 {
		return ValidationError{Field: "username", Message: "username must be at least 3 characters"}
	}
	
	if len(username) > 32 {
		return ValidationError{Field: "username", Message: "username must not exceed 32 characters"}
	}
	
	if !usernamePattern.MatchString(username) {
		return ValidationError{Field: "username", Message: "username must be alphanumeric with dots, hyphens, or underscores, starting with alphanumeric"}
	}
	
	// Prevent reserved usernames
	reserved := []string{"admin", "root", "system", "api", "anonymous", "guest"}
	lower := strings.ToLower(username)
	for _, r := range reserved {
		if lower == r {
			return ValidationError{Field: "username", Message: fmt.Sprintf("username '%s' is reserved", username)}
		}
	}
	
	return nil
}

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	if email == "" {
		return ValidationError{Field: "email", Message: "email cannot be empty"}
	}
	
	if len(email) > 254 { // RFC 5321
		return ValidationError{Field: "email", Message: "email address too long"}
	}
	
	_, err := mail.ParseAddress(email)
	if err != nil {
		return ValidationError{Field: "email", Message: "invalid email format"}
	}
	
	return nil
}

// ValidatePassword validates a password
func ValidatePassword(password string) error {
	if password == "" {
		return ValidationError{Field: "password", Message: "password cannot be empty"}
	}
	
	if len(password) < 8 {
		return ValidationError{Field: "password", Message: "password must be at least 8 characters"}
	}
	
	if len(password) > 128 {
		return ValidationError{Field: "password", Message: "password must not exceed 128 characters"}
	}
	
	// Check password complexity
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, char := range password {
		switch {
		case 'A' <= char && char <= 'Z':
			hasUpper = true
		case 'a' <= char && char <= 'z':
			hasLower = true
		case '0' <= char && char <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}
	
	complexity := 0
	if hasUpper {
		complexity++
	}
	if hasLower {
		complexity++
	}
	if hasDigit {
		complexity++
	}
	if hasSpecial {
		complexity++
	}
	
	if complexity < 3 {
		return ValidationError{Field: "password", Message: "password must contain at least 3 of: uppercase, lowercase, digit, special character"}
	}
	
	return nil
}

// ValidateFileContent validates file content for security
func ValidateFileContent(content []byte, path string) error {
	// Check file size
	maxSize := int64(50 * 1024 * 1024) // 50MB default
	
	// Allow larger sizes for specific file types
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".zip", ".tar", ".gz", ".bz2":
		maxSize = 100 * 1024 * 1024 // 100MB for archives
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		maxSize = 20 * 1024 * 1024 // 20MB for images
	}
	
	if int64(len(content)) > maxSize {
		return ValidationError{Field: "content", Message: fmt.Sprintf("file size exceeds maximum allowed (%d bytes)", maxSize)}
	}
	
	return nil
}

// ValidateTagName validates a Git tag name
func ValidateTagName(name string) error {
	if name == "" {
		return ValidationError{Field: "tag", Message: "tag name cannot be empty"}
	}
	
	if len(name) > 255 {
		return ValidationError{Field: "tag", Message: "tag name must not exceed 255 characters"}
	}
	
	// Similar rules to branch names
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return ValidationError{Field: "tag", Message: "tag name cannot start or end with hyphen"}
	}
	
	if strings.Contains(name, "..") {
		return ValidationError{Field: "tag", Message: "tag name cannot contain consecutive periods"}
	}
	
	// Check against Git's forbidden patterns
	forbidden := []string{"@{", "\\", "~", "^", ":", "?", "*", "[", " ", "\t", "\n"}
	for _, f := range forbidden {
		if strings.Contains(name, f) {
			return ValidationError{Field: "tag", Message: fmt.Sprintf("tag name cannot contain '%s'", f)}
		}
	}
	
	return nil
}

// SanitizeFilePath sanitizes a file path for safe usage
func SanitizeFilePath(path string) string {
	// Remove any potential null bytes first
	path = strings.ReplaceAll(path, "\x00", "")
	
	// Split the path and process each component
	parts := strings.Split(path, "/")
	var safeParts []string
	
	for _, part := range parts {
		// Skip empty parts, current directory references, and parent directory references
		if part == "" || part == "." || part == ".." {
			continue
		}
		safeParts = append(safeParts, part)
	}
	
	// Join the safe parts
	result := strings.Join(safeParts, "/")
	
	// Final clean
	if result == "" {
		return "."
	}
	
	return result
}