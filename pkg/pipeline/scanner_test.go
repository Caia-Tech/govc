package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"
)

func TestNewNativeSecurityScanner(t *testing.T) {
	config := &ScanConfig{
		IgnoreFiles: []string{"*.test"},
	}
	scanner := NewNativeSecurityScanner("go", config)

	if scanner == nil {
		t.Fatal("NewNativeSecurityScanner returned nil")
	}
	if scanner.language != "go" {
		t.Errorf("Expected language 'go', got '%s'", scanner.language)
	}
	if scanner.config != config {
		t.Error("Config not set correctly")
	}
	if len(scanner.rules) == 0 {
		t.Error("No security rules loaded")
	}
	if len(scanner.qualityRules) == 0 {
		t.Error("No quality rules loaded")
	}
}

func TestSecurityRulesGo(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	testCases := []struct {
		name        string
		code        string
		expectVuln  bool
		severity    string
		description string
	}{
		{
			name:        "SQL Injection - fmt.Sprintf with SELECT",
			code:        `query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userInput)`,
			expectVuln:  true,
			severity:    "high",
			description: "Potential SQL injection vulnerability",
		},
		{
			name:        "SQL Injection - fmt.Sprintf with UPDATE",
			code:        `query := fmt.Sprintf("UPDATE users SET name = '%s' WHERE id = %d", name, id)`,
			expectVuln:  true,
			severity:    "high",
			description: "Potential SQL injection vulnerability",
		},
		{
			name:        "Hardcoded Password",
			code:        `password := "MySecretPassword123"`,
			expectVuln:  true,
			severity:    "critical",
			description: "Hardcoded password or API key detected",
		},
		{
			name:        "Hardcoded API Key",
			code:        `apikey := "sk-1234567890abcdef"`,
			expectVuln:  true,
			severity:    "critical",
			description: "Hardcoded password or API key detected",
		},
		{
			name:        "Weak Crypto - MD5",
			code:        `hash := md5.Sum(data)`,
			expectVuln:  true,
			severity:    "medium",
			description: "Use of weak cryptographic algorithm",
		},
		{
			name:        "Weak Crypto - SHA1",
			code:        `hash := sha1.Sum(data)`,
			expectVuln:  true,
			severity:    "medium",
			description: "Use of weak cryptographic algorithm",
		},
		{
			name:        "Command Injection",
			code:        `cmd := exec.Command("/bin/sh", "-c", "ls " + userInput)`,
			expectVuln:  true,
			severity:    "high",
			description: "Potential command injection vulnerability",
		},
		{
			name:        "Safe Code - Parameterized Query",
			code:        `stmt, err := db.Prepare("SELECT * FROM users WHERE id = ?")`,
			expectVuln:  false,
			severity:    "",
			description: "",
		},
		{
			name:        "Safe Code - Environment Variable",
			code:        `apiKey := os.Getenv("API_KEY")`,
			expectVuln:  false,
			severity:    "",
			description: "",
		},
		{
			name:        "Safe Code - Strong Crypto",
			code:        `hash := sha256.Sum256(data)`,
			expectVuln:  false,
			severity:    "",
			description: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{
				"test.go": []byte(tc.code),
			}

			result, err := scanner.ScanMemory(ctx, files)
			if err != nil {
				t.Fatalf("ScanMemory failed: %v", err)
			}

			if tc.expectVuln {
				if len(result.Vulnerabilities) == 0 {
					t.Errorf("Expected vulnerability to be found, but none were detected for code: %s", tc.code)
				} else {
					vuln := result.Vulnerabilities[0]
					if vuln.Severity != tc.severity {
						t.Errorf("Expected severity '%s', got '%s'", tc.severity, vuln.Severity)
					}
					if vuln.Description != tc.description {
						t.Errorf("Expected description '%s', got '%s'", tc.description, vuln.Description)
					}
				}
			} else {
				if len(result.Vulnerabilities) > 0 {
					t.Errorf("Expected no vulnerabilities, but found %d", len(result.Vulnerabilities))
				}
			}
		})
	}
}

func TestSecurityRulesJavaScript(t *testing.T) {
	scanner := NewNativeSecurityScanner("javascript", &ScanConfig{})
	ctx := context.Background()

	testCases := []struct {
		name        string
		code        string
		expectVuln  bool
		severity    string
		description string
	}{
		{
			name:        "eval() Usage",
			code:        `const result = eval(userInput);`,
			expectVuln:  true,
			severity:    "high",
			description: "Use of dangerous eval() function",
		},
		{
			name:        "Hardcoded API Key",
			code:        `const apikey = "sk-abcdef123456789";`,
			expectVuln:  true,
			severity:    "critical",
			description: "Hardcoded API key or token",
		},
		{
			name:        "Safe Code - JSON.parse",
			code:        `const data = JSON.parse(jsonString);`,
			expectVuln:  false,
			severity:    "",
			description: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{
				"test.js": []byte(tc.code),
			}

			result, err := scanner.ScanMemory(ctx, files)
			if err != nil {
				t.Fatalf("ScanMemory failed: %v", err)
			}

			if tc.expectVuln {
				if len(result.Vulnerabilities) == 0 {
					t.Error("Expected vulnerability to be found, but none were detected")
				} else {
					vuln := result.Vulnerabilities[0]
					if vuln.Severity != tc.severity {
						t.Errorf("Expected severity '%s', got '%s'", tc.severity, vuln.Severity)
					}
					if vuln.Description != tc.description {
						t.Errorf("Expected description '%s', got '%s'", tc.description, vuln.Description)
					}
				}
			} else {
				if len(result.Vulnerabilities) > 0 {
					t.Errorf("Expected no vulnerabilities, but found %d", len(result.Vulnerabilities))
				}
			}
		})
	}
}

func TestQualityRules(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	testCases := []struct {
		name         string
		code         string
		expectIssue  bool
		severity     string
		description  string
		rule         string
	}{
		{
			name:         "Empty Error Handling",
			code:         `if err != nil { }`,
			expectIssue:  true,
			severity:     "medium",
			description:  "Empty error handling block",
			rule:         "Q001",
		},
		{
			name:         "TODO Comment",
			code:         `// TODO: Fix this later`,
			expectIssue:  true,
			severity:     "info",
			description:  "TODO comment found",
			rule:         "Q002",
		},
		{
			name:         "Proper Error Handling",
			code:         `if err != nil { return err }`,
			expectIssue:  false,
			severity:     "",
			description:  "",
			rule:         "",
		},
		{
			name:         "Regular Comment",
			code:         `// This is a normal comment`,
			expectIssue:  false,
			severity:     "",
			description:  "",
			rule:         "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string][]byte{
				"test.go": []byte(tc.code),
			}

			result, err := scanner.ScanMemory(ctx, files)
			if err != nil {
				t.Fatalf("ScanMemory failed: %v", err)
			}

			if tc.expectIssue {
				if len(result.CodeQuality) == 0 {
					t.Error("Expected quality issue to be found, but none were detected")
				} else {
					issue := result.CodeQuality[0]
					if issue.Severity != tc.severity {
						t.Errorf("Expected severity '%s', got '%s'", tc.severity, issue.Severity)
					}
					if issue.Description != tc.description {
						t.Errorf("Expected description '%s', got '%s'", tc.description, issue.Description)
					}
					if issue.Rule != tc.rule {
						t.Errorf("Expected rule '%s', got '%s'", tc.rule, issue.Rule)
					}
				}
			} else {
				if len(result.CodeQuality) > 0 {
					t.Errorf("Expected no quality issues, but found %d", len(result.CodeQuality))
				}
			}
		})
	}
}

func TestScanFileIgnore(t *testing.T) {
	config := &ScanConfig{
		IgnoreFiles: []string{"*_test.go", "vendor/*"},
	}
	scanner := NewNativeSecurityScanner("go", config)
	ctx := context.Background()

	files := map[string][]byte{
		"main.go":           []byte(`password := "secret123456"`),
		"main_test.go":      []byte(`password := "secret123456"`),
		"vendor/lib.go":     []byte(`password := "secret123456"`),
		"internal/core.go":  []byte(`password := "secret123456"`),
	}

	result, err := scanner.ScanMemory(ctx, files)
	if err != nil {
		t.Fatalf("ScanMemory failed: %v", err)
	}

	// Should only find vulnerabilities in main.go and internal/core.go
	expectedVulns := 2
	if len(result.Vulnerabilities) != expectedVulns {
		t.Errorf("Expected %d vulnerabilities, got %d", expectedVulns, len(result.Vulnerabilities))
	}

	// Verify ignored files are not in results
	for _, vuln := range result.Vulnerabilities {
		if vuln.File == "main_test.go" || vuln.File == "vendor/lib.go" {
			t.Errorf("Vulnerability found in ignored file: %s", vuln.File)
		}
	}
}

func TestDependencyScanning(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	goMod := `module test-app

go 1.19

require (
	github.com/some/package v0.0.0-20230101120000-abcdef123456
	github.com/other/lib v1.2.3
)
`

	files := map[string][]byte{
		"go.mod": []byte(goMod),
		"main.go": []byte("package main\nfunc main() {}"),
	}

	result, err := scanner.ScanMemory(ctx, files)
	if err != nil {
		t.Fatalf("ScanMemory failed: %v", err)
	}

	// Should find one dependency issue (development version)
	if len(result.Dependencies) == 0 {
		t.Error("Expected dependency issue to be found")
	} else {
		dep := result.Dependencies[0]
		if dep.Package != "github.com/some/package" {
			t.Errorf("Expected package 'github.com/some/package', got '%s'", dep.Package)
		}
		if dep.Version != "v0.0.0" {
			t.Errorf("Expected version 'v0.0.0', got '%s'", dep.Version)
		}
		if dep.Severity != "low" {
			t.Errorf("Expected severity 'low', got '%s'", dep.Severity)
		}
	}
}

func TestScanSummary(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	files := map[string][]byte{
		"main.go":     []byte(`password := "secret123"`),
		"helper.go":   []byte(`// TODO: implement this`),
		"config.go":   []byte(`apiKey := os.Getenv("API_KEY")`),
		"service.go":  []byte(`hash := md5.Sum(data)`),
	}

	result, err := scanner.ScanMemory(ctx, files)
	if err != nil {
		t.Fatalf("ScanMemory failed: %v", err)
	}

	// Verify summary statistics
	if result.Summary.TotalFiles != 4 {
		t.Errorf("Expected 4 total files, got %d", result.Summary.TotalFiles)
	}
	if result.Summary.FilesScanned != 4 {
		t.Errorf("Expected 4 files scanned, got %d", result.Summary.FilesScanned)
	}
	if result.Summary.Vulnerabilities != len(result.Vulnerabilities) {
		t.Errorf("Summary vulnerability count mismatch: %d vs %d", 
			result.Summary.Vulnerabilities, len(result.Vulnerabilities))
	}
	if result.Summary.QualityIssues != len(result.CodeQuality) {
		t.Errorf("Summary quality issue count mismatch: %d vs %d", 
			result.Summary.QualityIssues, len(result.CodeQuality))
	}
	if result.Summary.Score <= 0 || result.Summary.Score > 1 {
		t.Errorf("Security score should be between 0 and 1, got %f", result.Summary.Score)
	}
}

func TestSecurityScoreCalculation(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})

	testCases := []struct {
		name         string
		vulnerabilities []Vulnerability
		qualityIssues   []QualityIssue
		expectedScore   float64
	}{
		{
			name:            "Perfect Score - No Issues",
			vulnerabilities: []Vulnerability{},
			qualityIssues:   []QualityIssue{},
			expectedScore:   1.0,
		},
		{
			name: "Critical Vulnerability",
			vulnerabilities: []Vulnerability{
				{Severity: "critical"},
			},
			qualityIssues: []QualityIssue{},
			expectedScore: 0.7, // 1.0 - 0.3
		},
		{
			name: "Multiple Issues",
			vulnerabilities: []Vulnerability{
				{Severity: "high"},
				{Severity: "medium"},
			},
			qualityIssues: []QualityIssue{
				{Severity: "high"},
				{Severity: "low"},
			},
			expectedScore: 0.64, // 1.0 - 0.2 - 0.1 - 0.05 - 0.01
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &ScanResult{
				Vulnerabilities: tc.vulnerabilities,
				CodeQuality:     tc.qualityIssues,
				Summary: ScanSummary{
					FilesScanned: 1,
				},
			}

			score := scanner.calculateSecurityScore(result)
			if score != tc.expectedScore {
				t.Errorf("Expected score %f, got %f", tc.expectedScore, score)
			}
		})
	}
}

func TestCustomRules(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	
	// Add custom security rule
	customRule := &SecurityRule{
		ID:          "CUSTOM001",
		Name:        "Custom Rule",
		Description: "Custom security check",
		Severity:    "high",
		Pattern:     regexp.MustCompile(`dangerous_function\(`),
		Language:    "go",
		CWE:         "CWE-999",
		Fix:         "Use safer alternative",
	}
	
	err := scanner.AddCustomRule(customRule)
	if err != nil {
		t.Fatalf("Failed to add custom rule: %v", err)
	}

	// Test custom rule detection
	ctx := context.Background()
	files := map[string][]byte{
		"test.go": []byte("result := dangerous_function(input)"),
	}

	result, err := scanner.ScanMemory(ctx, files)
	if err != nil {
		t.Fatalf("ScanMemory failed: %v", err)
	}

	if len(result.Vulnerabilities) == 0 {
		t.Error("Custom rule should have detected vulnerability")
	} else {
		vuln := result.Vulnerabilities[0]
		if vuln.Description != "Custom security check" {
			t.Errorf("Expected custom rule description, got '%s'", vuln.Description)
		}
	}
}

func TestContextCancellation(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	
	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	
	// Wait for context to be cancelled
	time.Sleep(1 * time.Millisecond)

	files := map[string][]byte{
		"test.go": []byte("package main\nfunc main() {}"),
	}

	_, err := scanner.ScanMemory(ctx, files)
	if err == nil {
		t.Error("Expected context cancellation error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestScanStats(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	
	stats := scanner.GetScanStats()
	if stats.RulesApplied == 0 {
		t.Error("Expected rules to be applied")
	}
	
	expectedRules := len(scanner.rules) + len(scanner.qualityRules)
	if stats.RulesApplied != expectedRules {
		t.Errorf("Expected %d rules applied, got %d", expectedRules, stats.RulesApplied)
	}
}

func TestSupportedLanguages(t *testing.T) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	
	languages := scanner.GetSupportedLanguages()
	expectedLanguages := []string{"go", "javascript", "nodejs", "python", "java"}
	
	if len(languages) != len(expectedLanguages) {
		t.Errorf("Expected %d supported languages, got %d", len(expectedLanguages), len(languages))
	}
	
	for _, expected := range expectedLanguages {
		found := false
		for _, lang := range languages {
			if lang == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected language '%s' not found in supported languages", expected)
		}
	}
}

// Benchmark tests for memory-first scanning performance
func BenchmarkScanMemory(b *testing.B) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	// Create test files with various patterns
	files := map[string][]byte{
		"main.go": []byte(`
package main

import (
	"fmt"
	"crypto/md5"
	"os/exec"
)

func main() {
	password := "secret123"
	query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userInput)
	hash := md5.Sum(data)
	cmd := exec.Command("ls", userInput)
	
	if err != nil {
		// TODO: handle error
	}
}
`),
		"helper.go": []byte(`
package main

func helper() {
	apikey := "sk-1234567890abcdef"
	// Some regular code
	for i := 0; i < 100; i++ {
		fmt.Println(i)
	}
}
`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scanner.ScanMemory(ctx, files)
		if err != nil {
			b.Fatalf("ScanMemory failed: %v", err)
		}
	}
}

func BenchmarkScanLargeFile(b *testing.B) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	// Create large file content
	var content []byte
	for i := 0; i < 1000; i++ {
		content = append(content, []byte(fmt.Sprintf("func test%d() { fmt.Println(\"test\") }\n", i))...)
	}
	// Add some vulnerabilities
	content = append(content, []byte("password := \"secret123\"\n")...)
	content = append(content, []byte("hash := md5.Sum(data)\n")...)

	files := map[string][]byte{
		"large.go": content,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scanner.ScanMemory(ctx, files)
		if err != nil {
			b.Fatalf("ScanMemory failed: %v", err)
		}
	}
}

func BenchmarkScanMultipleFiles(b *testing.B) {
	scanner := NewNativeSecurityScanner("go", &ScanConfig{})
	ctx := context.Background()

	// Create multiple files
	files := make(map[string][]byte)
	for i := 0; i < 50; i++ {
		filename := fmt.Sprintf("file%d.go", i)
		content := fmt.Sprintf(`
package main

func test%d() {
	// TODO: implement
	if i %% 2 == 0 {
		password := "secret%d"
	}
}
`, i, i)
		files[filename] = []byte(content)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := scanner.ScanMemory(ctx, files)
		if err != nil {
			b.Fatalf("ScanMemory failed: %v", err)
		}
	}
}