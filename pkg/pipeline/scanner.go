package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// NativeSecurityScanner implements SecurityScanner with memory-first analysis
type NativeSecurityScanner struct {
	config        *ScanConfig
	rules         map[string]*SecurityRule
	qualityRules  map[string]*QualityRule
	language      string
}

// SecurityRule defines a security vulnerability pattern
type SecurityRule struct {
	ID          string
	Name        string
	Description string
	Severity    string
	Pattern     *regexp.Regexp
	Language    string
	CWE         string
	Fix         string
}

// QualityRule defines a code quality issue pattern
type QualityRule struct {
	ID          string
	Name        string
	Description string
	Severity    string
	Pattern     *regexp.Regexp
	Language    string
	Category    string
	Fix         string
}

// NewNativeSecurityScanner creates a new memory-first security scanner
func NewNativeSecurityScanner(language string, config *ScanConfig) *NativeSecurityScanner {
	scanner := &NativeSecurityScanner{
		config:       config,
		rules:        make(map[string]*SecurityRule),
		qualityRules: make(map[string]*QualityRule),
		language:     language,
	}
	
	scanner.loadDefaultRules()
	return scanner
}

// ScanMemory performs security and quality analysis on memory-resident files
func (nss *NativeSecurityScanner) ScanMemory(ctx context.Context, files map[string][]byte) (*ScanResult, error) {
	result := &ScanResult{
		Vulnerabilities: make([]Vulnerability, 0),
		CodeQuality:     make([]*QualityIssue, 0),
		Dependencies:    make([]*DependencyIssue, 0),
		Summary: ScanSummary{
			TotalFiles:   len(files),
			FilesScanned: 0,
		},
	}
	
	// Scan each file in memory
	for filePath, content := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// Skip ignored files
		if nss.shouldIgnoreFile(filePath) {
			continue
		}
		
		fileResult, err := nss.ScanFile(ctx, filePath, content)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file %s: %w", filePath, err)
		}
		
		// Aggregate results
		result.Vulnerabilities = append(result.Vulnerabilities, fileResult.Vulnerabilities...)
		result.CodeQuality = append(result.CodeQuality, fileResult.QualityIssues...)
		result.Summary.FilesScanned++
	}
	
	// Scan dependencies if present
	if depFile, exists := files["go.mod"]; exists {
		depIssues, err := nss.scanDependencies(ctx, depFile)
		if err == nil {
			for _, dep := range depIssues {
		result.Dependencies = append(result.Dependencies, &dep)
	}
		}
	}
	
	// Calculate summary statistics
	result.Summary.Vulnerabilities = len(result.Vulnerabilities)
	result.Summary.QualityIssues = len(result.CodeQuality)
	result.Summary.Score = nss.calculateSecurityScore(result)
	
	return result, nil
}

// ScanFile performs analysis on a single file in memory
func (nss *NativeSecurityScanner) ScanFile(ctx context.Context, path string, content []byte) (*FileScanResult, error) {
	result := &FileScanResult{
		Vulnerabilities: make([]Vulnerability, 0),
		QualityIssues:   make([]*QualityIssue, 0),
	}
	
	contentStr := string(content)
	lines := strings.Split(contentStr, "\n")
	
	// Scan for security vulnerabilities
	for lineNum, line := range lines {
		// Check security rules
		for _, rule := range nss.rules {
			if rule.Language != nss.language && rule.Language != "any" {
				continue
			}
			
			if rule.Pattern.MatchString(line) {
				vuln := Vulnerability{
					Type:        "security",
					Severity:    rule.Severity,
					File:        path,
					Line:        lineNum + 1,
					Description: rule.Description,
					Fix:         rule.Fix,
				}
				result.Vulnerabilities = append(result.Vulnerabilities, vuln)
			}
		}
		
		// Check quality rules
		for _, rule := range nss.qualityRules {
			if rule.Language != nss.language && rule.Language != "any" {
				continue
			}
			
			if rule.Pattern.MatchString(line) {
				issue := QualityIssue{
					Type:        "quality",
					Severity:    rule.Severity,
					File:        path,
					Line:        lineNum + 1,
					Description: rule.Description,
					Rule:        rule.ID,
				}
				result.QualityIssues = append(result.QualityIssues, &issue)
			}
		}
	}
	
	return result, nil
}

// loadDefaultRules loads built-in security and quality rules
func (nss *NativeSecurityScanner) loadDefaultRules() {
	// Security rules for Go
	if nss.language == "go" || nss.language == "any" {
		nss.addSecurityRule(&SecurityRule{
			ID:          "GO001",
			Name:        "SQL Injection",
			Description: "Potential SQL injection vulnerability",
			Severity:    "high",
			Pattern:     regexp.MustCompile(`(?i)fmt\.Sprintf.*SELECT|UPDATE|DELETE|INSERT`),
			Language:    "go",
			CWE:         "CWE-89",
			Fix:         "Use parameterized queries or prepared statements",
		})
		
		nss.addSecurityRule(&SecurityRule{
			ID:          "GO002",
			Name:        "Hardcoded Credentials",
			Description: "Hardcoded password or API key detected",
			Severity:    "critical",
			Pattern:     regexp.MustCompile(`(?i)(password|apikey|secret)\s*:?=\s*["'][^"']{8,}["']`),
			Language:    "go",
			CWE:         "CWE-798",
			Fix:         "Use environment variables or secure credential storage",
		})
		
		nss.addSecurityRule(&SecurityRule{
			ID:          "GO003",
			Name:        "Weak Crypto",
			Description: "Use of weak cryptographic algorithm",
			Severity:    "medium",
			Pattern:     regexp.MustCompile(`(?i)(md5|sha1)\.`),
			Language:    "go",
			CWE:         "CWE-327",
			Fix:         "Use SHA-256 or stronger cryptographic algorithms",
		})
		
		nss.addSecurityRule(&SecurityRule{
			ID:          "GO004",
			Name:        "Command Injection",
			Description: "Potential command injection vulnerability",
			Severity:    "high",
			Pattern:     regexp.MustCompile(`exec\.Command.*\+|exec\.CommandContext.*\+`),
			Language:    "go",
			CWE:         "CWE-77",
			Fix:         "Validate and sanitize input before executing commands",
		})
	}
	
	// Quality rules for Go
	if nss.language == "go" || nss.language == "any" {
		nss.addQualityRule(&QualityRule{
			ID:          "Q001",
			Name:        "Empty Catch Block",
			Description: "Empty error handling block",
			Severity:    "medium",
			Pattern:     regexp.MustCompile(`if\s+err\s*!=\s*nil\s*\{\s*\}`),
			Language:    "go",
			Category:    "error-handling",
			Fix:         "Handle or log the error appropriately",
		})
		
		nss.addQualityRule(&QualityRule{
			ID:          "Q002",
			Name:        "TODO Comment",
			Description: "TODO comment found",
			Severity:    "info",
			Pattern:     regexp.MustCompile(`(?i)//\s*TODO`),
			Language:    "any",
			Category:    "maintainability",
			Fix:         "Complete the TODO or create a proper issue",
		})
		
		nss.addQualityRule(&QualityRule{
			ID:          "Q003",
			Name:        "Long Function",
			Description: "Function appears to be very long",
			Severity:    "low",
			Pattern:     regexp.MustCompile(`func\s+\w+.*\{[\s\S]{500,}`),
			Language:    "go",
			Category:    "complexity",
			Fix:         "Consider breaking down into smaller functions",
		})
	}
	
	// JavaScript/Node.js rules
	if nss.language == "javascript" || nss.language == "nodejs" || nss.language == "any" {
		nss.addSecurityRule(&SecurityRule{
			ID:          "JS001",
			Name:        "eval() Usage",
			Description: "Use of dangerous eval() function",
			Severity:    "high",
			Pattern:     regexp.MustCompile(`\beval\s*\(`),
			Language:    "javascript",
			CWE:         "CWE-95",
			Fix:         "Avoid eval() and use safer alternatives like JSON.parse()",
		})
		
		nss.addSecurityRule(&SecurityRule{
			ID:          "JS002",
			Name:        "Hardcoded Secrets",
			Description: "Hardcoded API key or token",
			Severity:    "critical",
			Pattern:     regexp.MustCompile(`(?i)(api_key|apikey|token|secret)\s*:?=\s*["'][^"']{8,}["']`),
			Language:    "javascript",
			CWE:         "CWE-798",
			Fix:         "Use environment variables for sensitive data",
		})
	}
}

// Helper methods
func (nss *NativeSecurityScanner) addSecurityRule(rule *SecurityRule) {
	nss.rules[rule.ID] = rule
}

func (nss *NativeSecurityScanner) addQualityRule(rule *QualityRule) {
	nss.qualityRules[rule.ID] = rule
}

func (nss *NativeSecurityScanner) shouldIgnoreFile(filePath string) bool {
	if nss.config == nil || len(nss.config.IgnoreFiles) == 0 {
		return false
	}
	
	for _, pattern := range nss.config.IgnoreFiles {
		matched, _ := filepath.Match(pattern, filePath)
		if matched {
			return true
		}
	}
	return false
}

func (nss *NativeSecurityScanner) scanDependencies(ctx context.Context, goMod []byte) ([]DependencyIssue, error) {
	// Simple dependency scanning - in a real implementation, this would
	// check against vulnerability databases
	issues := make([]DependencyIssue, 0)
	
	content := string(goMod)
	lines := strings.Split(content, "\n")
	
	for _, line := range lines {
		// Look for lines with development versions (v0.0.0-timestamp-hash format)
		if strings.Contains(line, "v0.0.0-") {
			// Extract package name from the line
			parts := strings.Fields(strings.TrimSpace(line))
			if len(parts) >= 2 {
				packageName := parts[0]
				issues = append(issues, DependencyIssue{
					Package:     packageName,
					Version:     "v0.0.0",
					Severity:    "low",
					Description: "Using development version dependency",
					Fix:         "Pin to stable version",
				})
			}
		}
	}
	
	return issues, nil
}

func (nss *NativeSecurityScanner) calculateSecurityScore(result *ScanResult) float64 {
	if result.Summary.FilesScanned == 0 {
		return 1.0
	}
	
	// Simple scoring algorithm
	score := 1.0
	
	// Penalize for vulnerabilities
	for _, vuln := range result.Vulnerabilities {
		switch vuln.Severity {
		case "critical":
			score -= 0.3
		case "high":
			score -= 0.2
		case "medium":
			score -= 0.1
		case "low":
			score -= 0.05
		}
	}
	
	// Penalize for quality issues (less severe)
	for _, issue := range result.CodeQuality {
		switch issue.Severity {
		case "high":
			score -= 0.05
		case "medium":
			score -= 0.02
		case "low":
			score -= 0.01
		}
	}
	
	if score < 0 {
		score = 0
	}
	
	return score
}

func extractPackageName(requireLine string) string {
	// Extract package name from "require package v1.0.0"
	parts := strings.Fields(requireLine)
	for i, part := range parts {
		if part == "require" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return "unknown"
}

// FileScanResult contains the results of scanning a single file
type FileScanResult struct {
	Vulnerabilities []Vulnerability
	QualityIssues   []*QualityIssue
}

// ScanStats provides statistics about the scanning process
type ScanStats struct {
	FilesScanned    int
	LinesScanned    int
	RulesApplied    int
	ScanDuration    time.Duration
	MemoryUsage     int64
}

// GetScanStats returns statistics about the scanner
func (nss *NativeSecurityScanner) GetScanStats() *ScanStats {
	return &ScanStats{
		RulesApplied: len(nss.rules) + len(nss.qualityRules),
	}
}

// AddCustomRule allows adding custom security rules
func (nss *NativeSecurityScanner) AddCustomRule(rule *SecurityRule) error {
	if rule.Pattern == nil {
		return fmt.Errorf("rule pattern cannot be nil")
	}
	nss.rules[rule.ID] = rule
	return nil
}

// AddCustomQualityRule allows adding custom quality rules  
func (nss *NativeSecurityScanner) AddCustomQualityRule(rule *QualityRule) error {
	if rule.Pattern == nil {
		return fmt.Errorf("rule pattern cannot be nil")
	}
	nss.qualityRules[rule.ID] = rule
	return nil
}

// GetSupportedLanguages returns the languages supported by the scanner
func (nss *NativeSecurityScanner) GetSupportedLanguages() []string {
	return []string{"go", "javascript", "nodejs", "python", "java"}
}