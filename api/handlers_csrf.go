package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getCSRFToken returns a new CSRF token
// @Summary Get CSRF token
// @Description Generates and returns a new CSRF token for use in state-changing operations
// @Tags Authentication
// @Produce json
// @Success 200 {object} CSRFTokenResponse "CSRF token generated"
// @Failure 500 {object} ErrorResponse "Failed to generate token"
// @Router /auth/csrf-token [get]
func (s *Server) getCSRFToken(c *gin.Context) {
	token := GetCSRFToken(c, s.csrfStore)
	if token == "" {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to generate CSRF token",
			Code:  "CSRF_TOKEN_GENERATION_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, CSRFTokenResponse{
		Token:     token,
		ExpiresIn: 3600, // 1 hour
	})
}

// CSRFTokenResponse represents a CSRF token response
type CSRFTokenResponse struct {
	Token     string `json:"csrf_token" example:"eyJhbGciOiJIUzI1NiIs..."`
	ExpiresIn int    `json:"expires_in" example:"3600"`
}