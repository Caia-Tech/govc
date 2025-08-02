package govc

import (
	"context"
	"fmt"
	"time"
)

// AuthService handles authentication operations
type AuthService struct {
	client *Client
}

// Auth returns the authentication service
func (c *Client) Auth() *AuthService {
	return &AuthService{client: c}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      *User     `json:"user"`
}

// User represents a user
type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
}

// Login authenticates with the server and stores the token
func (as *AuthService) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	req := LoginRequest{
		Username: username,
		Password: password,
	}

	var resp LoginResponse
	err := as.client.post(ctx, "/api/v1/auth/login", req, &resp)
	if err != nil {
		return nil, err
	}

	// Store the token in the client
	as.client.token = resp.Token

	return &resp, nil
}

// RefreshToken refreshes the current JWT token
func (as *AuthService) RefreshToken(ctx context.Context) (*LoginResponse, error) {
	if as.client.token == "" {
		return nil, fmt.Errorf("no token to refresh")
	}

	var resp LoginResponse
	err := as.client.post(ctx, "/api/v1/auth/refresh", nil, &resp)
	if err != nil {
		return nil, err
	}

	// Update the token in the client
	as.client.token = resp.Token

	return &resp, nil
}

// WhoAmI returns information about the current authenticated user
func (as *AuthService) WhoAmI(ctx context.Context) (*User, error) {
	var user User
	err := as.client.get(ctx, "/api/v1/auth/whoami", &user)
	return &user, err
}

// APIKey represents an API key
type APIKey struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	KeyHash     string     `json:"key_hash"`
	Permissions []string   `json:"permissions"`
	CreatedAt   time.Time  `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKeyRequest represents a request to create an API key
type CreateAPIKeyRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions,omitempty"`
	ExpiresIn   string   `json:"expires_in,omitempty"`
}

// CreateAPIKeyResponse represents the response when creating an API key
type CreateAPIKeyResponse struct {
	ID     string  `json:"id"`
	Key    string  `json:"key"`
	APIKey *APIKey `json:"api_key"`
}

// CreateAPIKey creates a new API key
func (as *AuthService) CreateAPIKey(ctx context.Context, name string, permissions []string) (*CreateAPIKeyResponse, error) {
	req := CreateAPIKeyRequest{
		Name:        name,
		Permissions: permissions,
	}

	var resp CreateAPIKeyResponse
	err := as.client.post(ctx, "/api/v1/apikeys", req, &resp)
	return &resp, err
}

// ListAPIKeys lists all API keys for the current user
func (as *AuthService) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	var response struct {
		Keys  []*APIKey `json:"keys"`
		Count int       `json:"count"`
	}

	err := as.client.get(ctx, "/api/v1/apikeys", &response)
	return response.Keys, err
}

// RevokeAPIKey revokes an API key
func (as *AuthService) RevokeAPIKey(ctx context.Context, keyID string) error {
	return as.client.delete(ctx, fmt.Sprintf("/api/v1/apikeys/%s", keyID))
}

// SetToken sets the authentication token
func (c *Client) SetToken(token string) {
	c.token = token
}

// SetAPIKey sets the API key
func (c *Client) SetAPIKey(apiKey string) {
	c.apiKey = apiKey
}

// ClearAuth clears all authentication credentials
func (c *Client) ClearAuth() {
	c.token = ""
	c.apiKey = ""
}
