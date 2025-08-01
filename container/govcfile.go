package container

import (
	"bufio"
	"fmt"
	"strings"
)

// GovcfileInstruction represents a single instruction in a Govcfile
type GovcfileInstruction struct {
	Command string   // The instruction command (BASE, RUN, COPY, etc.)
	Args    []string // Arguments for the command
	Line    int      // Line number in the file for error reporting
}

// Govcfile represents a parsed Govcfile
type Govcfile struct {
	Instructions []GovcfileInstruction
	Metadata     map[string]string
}

// ParseGovcfile parses a Govcfile content and returns structured instructions
func ParseGovcfile(content string) (*Govcfile, error) {
	govcfile := &Govcfile{
		Instructions: []GovcfileInstruction{},
		Metadata:     make(map[string]string),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse metadata (lines starting with @)
		if strings.HasPrefix(line, "@") {
			parts := strings.SplitN(line[1:], "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				govcfile.Metadata[key] = value
			}
			continue
		}

		// Parse instructions
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		instruction := GovcfileInstruction{
			Command: strings.ToUpper(parts[0]),
			Args:    parts[1:],
			Line:    lineNum,
		}

		// Validate command
		if !isValidCommand(instruction.Command) {
			return nil, fmt.Errorf("line %d: unknown command '%s'", lineNum, instruction.Command)
		}

		govcfile.Instructions = append(govcfile.Instructions, instruction)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading govcfile: %w", err)
	}

	// Validate that we have at least a BASE instruction
	if len(govcfile.Instructions) == 0 || govcfile.Instructions[0].Command != "BASE" {
		return nil, fmt.Errorf("govcfile must start with a BASE instruction")
	}

	return govcfile, nil
}

// isValidCommand checks if a command is valid in Govcfile syntax
func isValidCommand(cmd string) bool {
	validCommands := []string{
		"BASE",      // Base image (replaces FROM)
		"RUN",       // Run commands
		"COPY",      // Copy files
		"ADD",       // Add files with extraction support
		"ENV",       // Set environment variables
		"WORKDIR",   // Set working directory
		"EXPOSE",    // Expose ports
		"VOLUME",    // Define volumes
		"USER",      // Set user
		"ENTRYPOINT",// Set entrypoint
		"CMD",       // Default command
		"LABEL",     // Add metadata labels
		"ARG",       // Build arguments
		"STOPSIGNAL",// Set stop signal
		"HEALTHCHECK",// Define health check
		"SHELL",     // Set default shell
		"ONBUILD",   // Trigger instruction for child images
	}

	for _, valid := range validCommands {
		if cmd == valid {
			return true
		}
	}
	return false
}

// Example Govcfile format:
/*
# Govcfile example - govc-native container definition

# Metadata
@version=1.0
@author=govc-team
@description=Example govc container

# Base image
BASE alpine:latest

# Build arguments
ARG VERSION=1.0

# Environment setup
ENV APP_VERSION=${VERSION}
ENV APP_HOME=/app

# Install dependencies
RUN apk add --no-cache curl git

# Set working directory
WORKDIR ${APP_HOME}

# Copy application files
COPY src/ .
COPY config/ ./config/

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost:8080/health || exit 1

# Run as non-root user
USER nobody

# Set entrypoint and default command
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["serve"]
*/