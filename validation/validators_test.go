package validation

import (
	"strings"
	"testing"
)

func TestValidateRepositoryID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid simple", "myrepo", false, ""},
		{"valid with hyphen", "my-repo", false, ""},
		{"valid with underscore", "my_repo", false, ""},
		{"valid alphanumeric", "repo123", false, ""},
		{"valid complex", "my-awesome_repo123", false, ""},
		{"valid single char", "a", false, ""},
		{"valid max length", strings.Repeat("a", 64), false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 65), true, "must not exceed 64 characters"},
		{"starts with hyphen", "-repo", true, "starting with alphanumeric"},
		{"starts with underscore", "_repo", true, "starting with alphanumeric"},
		{"contains spaces", "my repo", true, "must be alphanumeric"},
		{"contains special chars", "my@repo", true, "must be alphanumeric"},
		{"reserved name api", "api", true, "reserved"},
		{"reserved name admin", "admin", true, "reserved"},
		{"reserved name ADMIN", "ADMIN", true, "reserved"}, // case insensitive
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepositoryID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRepositoryID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateRepositoryID() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid simple", "main", false, ""},
		{"valid with slash", "feature/login", false, ""},
		{"valid with hyphen", "feature-123", false, ""},
		{"valid complex", "release/v1.2.3-beta", false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 256), true, "must not exceed 255 characters"},
		{"starts with hyphen", "-branch", true, "cannot start or end with hyphen"},
		{"ends with hyphen", "branch-", true, "cannot start or end with hyphen"},
		{"starts with period", ".branch", true, "cannot start or end with period"},
		{"ends with period", "branch.", true, "cannot start or end with period"},
		{"double period", "branch..name", true, "consecutive periods"},
		{"double slash", "feature//login", true, "consecutive slashes"},
		{"contains space", "my branch", true, "cannot contain ' '"},
		{"contains @{", "branch@{", true, "cannot contain '@{'"},
		{"contains backslash", "branch\\name", true, "cannot contain"},
		{"contains tilde", "branch~name", true, "cannot contain '~'"},
		{"contains caret", "branch^name", true, "cannot contain '^'"},
		{"contains colon", "branch:name", true, "cannot contain ':'"},
		{"contains question", "branch?name", true, "cannot contain '?'"},
		{"contains asterisk", "branch*name", true, "cannot contain '*'"},
		{"contains bracket", "branch[name", true, "cannot contain '['"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBranchName() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateBranchName() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid simple", "file.txt", false, ""},
		{"valid with directory", "dir/file.txt", false, ""},
		{"valid nested", "dir/subdir/file.txt", false, ""},
		{"valid with dot", "./file.txt", false, ""},
		{"valid current dir", ".", false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 4097), true, "too long"},
		{"path traversal parent", "../file.txt", true, "path traversal"},
		{"path traversal nested", "dir/../../../etc/passwd", true, "path traversal"},
		{"absolute path unix", "/etc/passwd", true, "absolute paths are not allowed"},
		{"absolute path windows", "C:\\Windows\\System32", true, "absolute paths are not allowed"},
		{"null byte", "file\x00.txt", true, "null bytes"},
		{"hidden file at root", ".git/config", true, "hidden files at root level"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateFilePath() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateCommitMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid simple", "Initial commit", false, ""},
		{"valid with newline", "feat: Add login\n\nThis adds user login functionality", false, ""},
		{"valid with special chars", "fix: Handle edge-case #123 & improve performance!", false, ""},
		{"valid unicode", "feat: Add 日本語 support", false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 100001), true, "too long"},
		{"control char", "commit\x01message", true, "invalid control characters"},
		{"null byte", "commit\x00message", true, "invalid control characters"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommitMessage(tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommitMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateCommitMessage() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name    string
		username string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid simple", "john", false, ""},
		{"valid with numbers", "john123", false, ""},
		{"valid with underscore", "john_doe", false, ""},
		{"valid with hyphen", "john-doe", false, ""},
		{"valid with dot", "john.doe", false, ""},
		{"valid min length", "abc", false, ""},
		{"valid max length", strings.Repeat("a", 32), false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too short", "ab", true, "at least 3 characters"},
		{"too long", strings.Repeat("a", 33), true, "must not exceed 32 characters"},
		{"starts with dot", ".john", true, "starting with alphanumeric"},
		{"starts with hyphen", "-john", true, "starting with alphanumeric"},
		{"contains space", "john doe", true, "must be alphanumeric"},
		{"contains special", "john@doe", true, "must be alphanumeric"},
		{"reserved admin", "admin", true, "reserved"},
		{"reserved root", "root", true, "reserved"},
		{"reserved ROOT", "ROOT", true, "reserved"}, // case insensitive
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUsername(tt.username)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUsername() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateUsername() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"valid simple", "user@example.com", false, ""},
		{"valid with plus", "user+tag@example.com", false, ""},
		{"valid with dots", "first.last@example.com", false, ""},
		{"valid subdomain", "user@mail.example.com", false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too long", strings.Repeat("a", 250) + "@example.com", true, "too long"},
		{"missing @", "userexample.com", true, "invalid email format"},
		{"missing domain", "user@", true, "invalid email format"},
		{"missing local", "@example.com", true, "invalid email format"},
		{"double @", "user@@example.com", true, "invalid email format"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateEmail() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		// Valid cases
		{"valid strong", "MyP@ssw0rd!", false, ""},
		{"valid min complexity", "mypassword123!", false, ""},
		{"valid long", "ThisIsAVeryLongPasswordWith123!", false, ""},
		
		// Invalid cases
		{"empty", "", true, "cannot be empty"},
		{"too short", "Pass1!", true, "at least 8 characters"},
		{"too long", strings.Repeat("a", 129), true, "must not exceed 128 characters"},
		{"low complexity all lower", "password", true, "at least 3 of"},
		{"low complexity lower+digit", "password123", true, "at least 3 of"},
		{"low complexity no special", "Password123", false, ""}, // This should pass (3 types)
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidatePassword() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestSanitizeFilePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple path", "file.txt", "file.txt"},
		{"nested path", "dir/file.txt", "dir/file.txt"},
		{"leading slash", "/dir/file.txt", "dir/file.txt"},
		{"path traversal", "../../../etc/passwd", "etc/passwd"},
		{"mixed traversal", "dir/../../../etc/passwd", "dir/etc/passwd"},
		{"null bytes", "file\x00.txt", "file.txt"},
		{"multiple slashes", "dir//file.txt", "dir/file.txt"},
		{"trailing slash", "dir/", "dir"},
		{"current dir", "./file.txt", "file.txt"},
		{"complex", "/../dir/.././../file.txt", "dir/file.txt"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilePath(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilePath() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateFileContent(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		path    string
		wantErr bool
		errMsg  string
	}{
		// Valid cases
		{"small text file", []byte("hello world"), "test.txt", false, ""},
		{"medium text file", make([]byte, 1024*1024), "large.txt", false, ""}, // 1MB
		{"large archive", make([]byte, 50*1024*1024), "archive.zip", false, ""}, // 50MB zip
		{"image file", make([]byte, 10*1024*1024), "photo.jpg", false, ""}, // 10MB image
		
		// Invalid cases
		{"text too large", make([]byte, 51*1024*1024), "huge.txt", true, "exceeds maximum"},
		{"archive too large", make([]byte, 101*1024*1024), "huge.zip", true, "exceeds maximum"},
		{"image too large", make([]byte, 21*1024*1024), "huge.jpg", true, "exceeds maximum"},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileContent(tt.content, tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileContent() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("ValidateFileContent() error message = %v, want containing %v", err.Error(), tt.errMsg)
			}
		})
	}
}