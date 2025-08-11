# SonarQube Analysis Report

Generated: Mon Aug 11 19:12:04 EDT 2025

## Coverage Summary
- **Total Coverage**: 65.4%
- **Coverage File**: coverage.out
- **HTML Report**: .analysis/coverage.html

## Static Analysis Results
- **Go Vet Issues**: 0
- **Security Issues**: 0
- **Lint Issues**: N/A

## Top Coverage by Package
```
github.com/Caia-Tech/govc/auth/apikey.go:36:			NewAPIKeyManager		100.0%
github.com/Caia-Tech/govc/auth/apikey.go:44:			GenerateAPIKey			92.9%
github.com/Caia-Tech/govc/auth/apikey.go:89:			ValidateAPIKey			81.5%
github.com/Caia-Tech/govc/auth/apikey.go:142:			GetAPIKey			71.4%
github.com/Caia-Tech/govc/auth/apikey.go:169:			ListAPIKeys			73.3%
github.com/Caia-Tech/govc/auth/apikey.go:197:			RevokeAPIKey			100.0%
github.com/Caia-Tech/govc/auth/apikey.go:212:			DeleteAPIKey			0.0%
github.com/Caia-Tech/govc/auth/apikey.go:227:			HasPermission			0.0%
github.com/Caia-Tech/govc/auth/apikey.go:244:			HasRepositoryPermission		0.0%
github.com/Caia-Tech/govc/auth/apikey.go:265:			UpdateAPIKeyPermissions		0.0%
github.com/Caia-Tech/govc/auth/apikey.go:281:			CleanupExpiredKeys		0.0%
github.com/Caia-Tech/govc/auth/apikey.go:299:			GetKeyStats			100.0%
github.com/Caia-Tech/govc/auth/apikey.go:327:			generateAPIKeyID		100.0%
github.com/Caia-Tech/govc/auth/apikey.go:347:			ToInfo				100.0%
github.com/Caia-Tech/govc/auth/jwt.go:39:			NewJWTAuth			60.0%
github.com/Caia-Tech/govc/auth/jwt.go:59:			GenerateToken			100.0%
github.com/Caia-Tech/govc/auth/jwt.go:81:			ValidateToken			82.6%
github.com/Caia-Tech/govc/auth/jwt.go:132:			cleanupExpiredTokens		0.0%
github.com/Caia-Tech/govc/auth/jwt.go:142:			RefreshToken			100.0%
github.com/Caia-Tech/govc/auth/jwt.go:153:			ExtractUserID			0.0%
```

## Files Analyzed
- Coverage Report: `.analysis/coverage.html`
- Go Vet Report: `.analysis/govet_report.txt`
- Security Report: `.analysis/gosec_report.json`
- Lint Report: `.analysis/golangci_report.json`

## SonarQube Integration
- Project Key: govc
- Server URL: http://localhost:9000
- Properties: `sonar-project.properties`

## Next Steps
1. Review coverage report: open `.analysis/coverage.html`
2. Address static analysis issues
3. Improve test coverage for low-coverage areas
4. Run SonarQube analysis: `sonar-scanner`

