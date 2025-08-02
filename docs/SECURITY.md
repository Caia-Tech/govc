# Security Architecture

## Overview

govc implements a comprehensive security architecture designed to protect against common vulnerabilities and provide enterprise-grade security features. This document outlines the security measures, best practices, and implementation details.

## Table of Contents

- [Authentication & Authorization](#authentication--authorization)
- [Input Validation](#input-validation)
- [Security Middleware](#security-middleware)
- [Secure Configuration](#secure-configuration)
- [Security Best Practices](#security-best-practices)
- [Threat Model](#threat-model)

## Authentication & Authorization

### JWT Authentication

govc uses JSON Web Tokens (JWT) for stateless authentication:

```go
// JWT token structure
type Claims struct {
    UserID   string   `json:"user_id"`
    Username string   `json:"username"`
    Roles    []string `json:"roles"`
    jwt.StandardClaims
}
```

**Security Features:**
- Configurable token expiration (default: 24 hours)
- Secure secret key storage (minimum 32 characters recommended)
- Token refresh mechanism to limit exposure
- Automatic token validation on protected endpoints

### API Key Management

API keys provide an alternative authentication method for programmatic access:

```go
// API key generation
key := auth.GenerateAPIKey() // Creates SHA256 hash
apiKeyMgr.CreateKey(userID, keyName, permissions)
```

**Security Features:**
- SHA256 hashing of API keys
- Per-key permission scoping
- Key expiration support
- Secure key storage (only hash stored)
- Key revocation capability

### Role-Based Access Control (RBAC)

Fine-grained permission system with predefined roles:

**Default Roles:**
- `admin`: Full system access
- `developer`: Read/write repository access
- `reader`: Read-only access
- `guest`: Limited public access

**Permissions:**
```go
const (
    PermissionSystemAdmin    = "system:admin"
    PermissionRepoRead      = "repo:read"
    PermissionRepoWrite     = "repo:write"
    PermissionRepoDelete    = "repo:delete"
    PermissionUserRead      = "user:read"
    PermissionUserWrite     = "user:write"
    PermissionWebhookWrite  = "webhook:write"
)
```

## Input Validation

### Repository ID Validation

```go
func ValidateRepositoryID(id string) error {
    // Length validation
    if len(id) > 64 {
        return ValidationError{Field: "repo_id", Message: "must not exceed 64 characters"}
    }
    
    // Pattern validation (alphanumeric with hyphens/underscores)
    if !repoIDPattern.MatchString(id) {
        return ValidationError{Field: "repo_id", Message: "must be alphanumeric"}
    }
    
    // Reserved names check
    for _, reserved := range reservedRepoIDs {
        if id == reserved {
            return ValidationError{Field: "repo_id", Message: "repository ID is reserved"}
        }
    }
    
    return nil
}
```

### File Path Validation

Comprehensive protection against path traversal attacks:

```go
func ValidateFilePath(path string) error {
    // Null byte check
    if strings.Contains(path, "\x00") {
        return ValidationError{Field: "path", Message: "path cannot contain null bytes"}
    }
    
    // Path traversal check
    if strings.Contains(path, "..") {
        return ValidationError{Field: "path", Message: "path traversal detected"}
    }
    
    // Absolute path check
    if filepath.IsAbs(path) {
        return ValidationError{Field: "path", Message: "absolute paths are not allowed"}
    }
    
    // Hidden file restrictions
    if strings.HasPrefix(path, ".") && !strings.Contains(path, "/") {
        return ValidationError{Field: "path", Message: "hidden files at root level are not allowed"}
    }
    
    return nil
}
```

### Branch Name Validation

Git-compliant branch name validation:

```go
func ValidateBranchName(name string) error {
    // Invalid characters check
    invalidChars := []string{" ", "~", "^", ":", "?", "*", "[", "@{", "\\"}
    for _, char := range invalidChars {
        if strings.Contains(name, char) {
            return ValidationError{Field: "branch", Message: fmt.Sprintf("cannot contain '%s'", char)}
        }
    }
    
    // Start/end validation
    if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
        return ValidationError{Field: "branch", Message: "cannot start or end with hyphen"}
    }
    
    // Consecutive dots check
    if strings.Contains(name, "..") {
        return ValidationError{Field: "branch", Message: "cannot contain consecutive periods"}
    }
    
    return nil
}
```

## Security Middleware

### Request Size Limits

Protection against resource exhaustion:

```go
// Path length validation
if len(c.Request.URL.Path) > config.MaxPathLength {
    c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
        Error: "Request path too long",
        Code:  "PATH_TOO_LONG",
    })
    c.Abort()
    return
}

// Query string length validation
if len(c.Request.URL.RawQuery) > config.MaxQueryLength {
    c.JSON(http.StatusRequestEntityTooLarge, ErrorResponse{
        Error: "Query string too long",
        Code:  "QUERY_TOO_LONG",
    })
    c.Abort()
    return
}

// Header size validation
headerSize := calculateHeaderSize(c.Request.Header)
if headerSize > config.MaxHeaderSize {
    c.JSON(http.StatusRequestHeaderFieldsTooLarge, ErrorResponse{
        Error: "Request headers too large",
        Code:  "HEADERS_TOO_LARGE",
    })
    c.Abort()
    return
}
```

### Brute Force Protection

Automatic lockout after failed login attempts:

```go
type loginAttempt struct {
    count       int
    lastAttempt time.Time
    lockedUntil time.Time
}

// Track failed attempts
if attempt.count >= config.MaxLoginAttempts {
    attempt.lockedUntil = time.Now().Add(config.LoginLockoutDuration)
    logger.Warn("Login lockout triggered", 
        "client_ip", clientIP,
        "attempt_count", attempt.count)
}
```

### Content-Type Validation

Strict content type enforcement:

```go
allowedContentTypes := []string{
    "application/json",
    "text/plain",
    "application/octet-stream",
}

if !isAllowedContentType(contentType, allowedContentTypes) {
    c.JSON(http.StatusUnsupportedMediaType, ErrorResponse{
        Error: fmt.Sprintf("Content type '%s' not allowed", contentType),
        Code:  "UNSUPPORTED_CONTENT_TYPE",
    })
    c.Abort()
    return
}
```

### Security Headers

Automatic security headers on all responses:

```go
func SecurityHeadersMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Header("X-Content-Type-Options", "nosniff")
        c.Header("X-Frame-Options", "DENY")
        c.Header("X-XSS-Protection", "1; mode=block")
        c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        c.Header("Content-Security-Policy", "default-src 'self'")
        c.Next()
    }
}
```

## Secure Configuration

### Default Security Settings

```yaml
security:
  # Rate limiting
  rate_limit_per_minute: 60
  
  # Brute force protection
  max_login_attempts: 5
  login_lockout_duration: "15m"
  
  # Request validation
  enable_path_sanitization: true
  max_path_length: 4096
  max_header_size: 8192
  max_query_length: 2048
  max_file_upload_size: 52428800  # 50MB
  
  # Content security
  allowed_content_types:
    - "application/json"
    - "text/plain"
    - "application/octet-stream"
```

### Environment Variables

Sensitive configuration via environment variables:

```bash
# JWT secret (use strong, random value)
GOVC_JWT_SECRET=your-very-secure-secret-key-minimum-32-chars

# Database encryption key
GOVC_ENCRYPTION_KEY=another-very-secure-encryption-key

# TLS configuration
GOVC_TLS_CERT=/path/to/cert.pem
GOVC_TLS_KEY=/path/to/key.pem
```

## Security Best Practices

### 1. Authentication

- **Use strong JWT secrets**: Minimum 32 characters, randomly generated
- **Enable HTTPS in production**: Use TLS for all communications
- **Rotate API keys regularly**: Implement key rotation policies
- **Use short token expiration**: Balance security with usability

### 2. Input Handling

- **Validate all inputs**: Never trust user input
- **Sanitize file paths**: Prevent directory traversal
- **Limit request sizes**: Prevent resource exhaustion
- **Use parameterized queries**: If using database backends

### 3. Error Handling

- **Don't expose internal errors**: Use generic error messages
- **Log security events**: Track failed authentication attempts
- **Rate limit error responses**: Prevent information leakage

### 4. Deployment

- **Run as non-root user**: Use least privilege principle
- **Use network segmentation**: Isolate services
- **Enable firewall rules**: Restrict network access
- **Keep dependencies updated**: Regular security updates

## Threat Model

### Common Attack Vectors

1. **Path Traversal**
   - **Mitigation**: Strict path validation and sanitization
   - **Implementation**: ValidateFilePath() function

2. **Brute Force Attacks**
   - **Mitigation**: Account lockout and rate limiting
   - **Implementation**: LoginAttempt tracking

3. **Injection Attacks**
   - **Mitigation**: Input validation and sanitization
   - **Implementation**: Comprehensive validators

4. **Session Hijacking**
   - **Mitigation**: Secure token generation and HTTPS
   - **Implementation**: JWT with expiration

5. **Denial of Service**
   - **Mitigation**: Rate limiting and resource limits
   - **Implementation**: Request size limits and timeouts

### Security Monitoring

Monitor these events for security concerns:

```go
// Security events to log
- Failed authentication attempts
- Account lockouts triggered
- Invalid input validation failures
- Unauthorized access attempts
- Suspicious path access patterns
- Rate limit violations
```

## Security Checklist

Before deploying to production:

- [ ] Change default JWT secret
- [ ] Enable HTTPS/TLS
- [ ] Configure firewall rules
- [ ] Set up log monitoring
- [ ] Review user permissions
- [ ] Enable rate limiting
- [ ] Configure backup strategy
- [ ] Test security middleware
- [ ] Review error messages
- [ ] Update dependencies

## Reporting Security Issues

If you discover a security vulnerability, please:

1. **Do not** open a public issue
2. Email security@caia.tech with details
3. Include steps to reproduce if possible
4. Allow time for patch before disclosure

We take security seriously and will respond promptly to valid reports.