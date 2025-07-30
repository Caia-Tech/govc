# govc CLI Authentication Guide

The govc CLI provides comprehensive authentication support for working with remote govc servers. This guide covers all authentication-related features.

## Table of Contents

- [Quick Start](#quick-start)
- [Authentication Commands](#authentication-commands)
- [API Key Management](#api-key-management)
- [User Management](#user-management)
- [Configuration](#configuration)
- [Remote Repository Operations](#remote-repository-operations)
- [Security Best Practices](#security-best-practices)

## Quick Start

### Login to a govc server

```bash
# Interactive login
govc auth login --server https://govc.example.com

# Login with username provided
govc auth login --server https://govc.example.com --username alice

# Login without saving credentials
govc auth login --server https://govc.example.com --no-save
```

### Check authentication status

```bash
# Show current user information
govc auth whoami

# List all saved authentication contexts
govc auth token list
```

### Logout

```bash
govc auth logout
```

## Authentication Commands

### `govc auth login`

Authenticate to a govc server and save credentials.

**Options:**
- `--server`: Server URL (required)
- `--username`: Username (optional, will prompt if not provided)
- `--context`: Context name to save credentials (default: "default")
- `--no-save`: Don't save credentials to disk

**Example:**
```bash
govc auth login --server https://govc.company.com --username developer --context work
```

### `govc auth logout`

Remove saved authentication credentials.

```bash
govc auth logout
```

### `govc auth whoami`

Display information about the currently authenticated user.

```bash
$ govc auth whoami
Current context: default
Server: https://govc.example.com
Username: alice
Token expires: 2024-12-25 15:30:00
Status: Active
```

### `govc auth token list`

List all saved authentication contexts.

```bash
$ govc auth token list
CONTEXT              SERVER                                   USERNAME             STATUS
* default            https://govc.example.com                alice                Active
  work               https://govc.company.com                developer            Active
  staging            https://staging.govc.com                test-user            Expired
```

## API Key Management

API keys provide an alternative authentication method for automated tools and CI/CD pipelines.

### Create an API key

```bash
# Create a new API key
govc auth apikey create "CI Pipeline"

# Output:
# API key created successfully!
# ID: 550e8400-e29b-41d4-a716-446655440000
# Name: CI Pipeline
# Key: gk_live_abcdefghijklmnopqrstuvwxyz1234567890
# 
# IMPORTANT: Save this key securely. It won't be shown again.
```

### List API keys

```bash
$ govc auth apikey list
ID                                    NAME                           CREATED              LAST USED
550e8400-e29b-41d4-a716-446655440000  CI Pipeline                   2024-01-15 10:30    2024-01-15 14:22
660e8400-e29b-41d4-a716-446655440001  Monitoring Script             2024-01-10 09:15    Never
```

### Delete an API key

```bash
govc auth apikey delete 550e8400-e29b-41d4-a716-446655440000
```

### Using API keys

Set the API key in your environment or configuration:

```bash
# Via environment variable
export GOVC_API_KEY="gk_live_abcdefghijklmnopqrstuvwxyz1234567890"

# Or configure it in the CLI config
govc config set api_key "gk_live_abcdefghijklmnopqrstuvwxyz1234567890"
```

## User Management

Admin users can manage other users through the CLI.

### List users

```bash
$ govc user list
USERNAME             EMAIL                          ACTIVE   ADMIN    ROLES                          CREATED
alice                alice@example.com              Yes      Yes      admin, developer               2024-01-01
bob                  bob@example.com                Yes      No       developer, reviewer            2024-01-05
charlie              charlie@example.com            No       No       viewer                         2024-01-10
```

### Create a user

```bash
# Create a regular user
govc user create john --email john@example.com

# Create an admin user with roles
govc user create admin --email admin@example.com --admin --roles admin,developer
```

### Get user details

```bash
$ govc user get alice
Username: alice
ID: 550e8400-e29b-41d4-a716-446655440000
Email: alice@example.com
Active: true
Admin: true
Roles: admin, developer, reviewer
Created: 2024-01-01 10:30:00
Updated: 2024-01-15 14:22:00
```

### Update user information

```bash
# Update email
govc user update alice --email alice.smith@example.com

# Deactivate user
govc user update bob --active=false
```

### Change password

```bash
# Change your own password
govc user password

# Change another user's password (admin only)
govc user password alice
```

### Manage user roles

```bash
# Add a role
govc user role add alice reviewer

# Remove a role
govc user role remove alice developer
```

### Delete a user

```bash
govc user delete charlie
```

## Configuration

The govc CLI supports configuration files for default settings.

### View configuration

```bash
# List all configuration
govc config list

# Get specific value
govc config get default_server
```

### Set configuration values

```bash
# Set default server
govc config set default_server https://govc.example.com

# Set default timeout (seconds)
govc config set default_timeout 60

# Enable auto-authentication
govc config set auto_auth true

# Set default author information
govc config set default_author.name "Alice Smith"
govc config set default_author.email "alice@example.com"
```

### Configuration file location

Configuration is stored in `~/.config/govc/config.json`:

```json
{
  "default_server": "https://govc.example.com",
  "default_timeout": 30,
  "enable_metrics": false,
  "enable_logging": false,
  "log_level": "info",
  "editor": "vim",
  "color_output": true,
  "auto_auth": true,
  "default_author": {
    "name": "Alice Smith",
    "email": "alice@example.com"
  },
  "aliases": {
    "st": "status",
    "co": "checkout"
  }
}
```

## Remote Repository Operations

Work with repositories on remote govc servers.

### List remote repositories

```bash
$ govc remote list
ID                             TYPE       DESCRIPTION                                        UPDATED
my-project                     memory     Main project repository                            2024-01-15 14:30
test-repo                      disk       Testing repository                                 2024-01-14 10:15
documentation                  memory     Documentation and examples                         2024-01-13 09:00
```

### Create a remote repository

```bash
# Create a disk-based repository
govc remote create my-new-repo --description "New project repository"

# Create a memory-only repository
govc remote create temp-repo --memory --description "Temporary testing"
```

### Clone a remote repository

```bash
# Clone to directory with same name
govc remote clone my-project

# Clone to specific directory
govc remote clone my-project ./local-project
```

### Push and pull changes

```bash
# Push current branch to remote
govc remote push

# Push specific branch
govc remote push feature-branch

# Pull changes from remote
govc remote pull

# Pull specific branch
govc remote pull main
```

### Delete a remote repository

```bash
govc remote delete test-repo
```

## Security Best Practices

### 1. Credential Storage

- Credentials are stored in `~/.config/govc/auth.json` with restricted permissions (0600)
- Use `--no-save` flag for temporary sessions on shared machines
- Regularly rotate authentication tokens

### 2. API Key Management

- Use API keys for automated tools instead of user credentials
- Create separate API keys for different applications
- Regularly audit and remove unused API keys
- Never commit API keys to version control

### 3. Environment Variables

When using environment variables for authentication:

```bash
# For JWT tokens
export GOVC_AUTH_TOKEN="your-jwt-token"

# For API keys
export GOVC_API_KEY="gk_live_..."

# For server URL
export GOVC_SERVER="https://govc.example.com"
```

### 4. Multiple Contexts

Use different contexts for different environments:

```bash
# Production
govc auth login --server https://prod.govc.com --context production

# Staging
govc auth login --server https://staging.govc.com --context staging

# Switch between contexts by logging out and logging in with different context
govc auth logout
govc auth login --server https://staging.govc.com --context staging
```

### 5. Token Expiration

- Tokens expire after 24 hours by default
- The CLI will prompt for re-authentication when tokens expire
- Use `govc auth whoami` to check token status

## Troubleshooting

### Authentication failures

1. Check server connectivity:
   ```bash
   curl -I https://govc.example.com/health
   ```

2. Verify credentials are correct
3. Check token expiration with `govc auth whoami`
4. Try logging out and logging in again

### Permission errors

1. Verify your user has the required permissions
2. Check with an admin if roles need to be added
3. Use `govc user get <username>` to view current roles

### Configuration issues

1. Reset configuration to defaults:
   ```bash
   govc config reset
   ```

2. Check configuration file permissions:
   ```bash
   ls -la ~/.config/govc/
   ```

3. Manually edit configuration files if needed

## Examples

### CI/CD Pipeline Setup

```bash
# Create API key for CI
govc auth apikey create "GitHub Actions"

# In your CI configuration, set:
# GOVC_API_KEY=gk_live_...
# GOVC_SERVER=https://govc.example.com

# Then commands work without interactive login:
govc remote list
govc remote clone my-project
```

### Team Development Workflow

```bash
# Each developer logs in with their account
govc auth login --server https://govc.company.com --username alice

# Work with remote repositories
govc remote clone team-project
cd team-project
govc checkout -b feature/new-feature

# Make changes and push
govc add .
govc commit -m "Add new feature"
govc remote push feature/new-feature
```

### Admin Tasks

```bash
# Login as admin
govc auth login --server https://govc.company.com --username admin

# Create new developer account
govc user create newdev --email newdev@company.com --roles developer

# Grant additional permissions
govc user role add newdev reviewer

# Create API key for monitoring
govc auth apikey create "Monitoring System"
```