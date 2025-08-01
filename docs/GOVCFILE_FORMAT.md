# Govcfile Format Specification

## Overview

Govcfile is govc's native container definition format, designed to replace Docker-specific terminology while maintaining compatibility with container building workflows. This format is part of govc's memory-first, repository-native container system.

## File Naming

- Primary: `Govcfile`
- With suffix: `Govcfile.prod`, `Govcfile.dev`, etc.
- Multi-container: `govc-compose.yml` or `govc-compose.yaml`

## Syntax

### Basic Structure

```govcfile
# Govcfile example

# Metadata (optional)
@version=1.0
@author=developer@example.com
@description=Application container

# Base image (required)
BASE alpine:latest

# Build arguments
ARG VERSION=1.0

# Environment variables
ENV APP_VERSION=${VERSION}
ENV APP_HOME=/app

# Install dependencies
RUN apk add --no-cache curl git

# Set working directory
WORKDIR ${APP_HOME}

# Copy files
COPY src/ .
COPY config/ ./config/

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s \
  CMD curl -f http://localhost:8080/health || exit 1

# Run as non-root user
USER nobody

# Set entrypoint and command
ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["serve"]
```

## Instructions

### BASE
Specifies the base image (replaces Docker's FROM)
```
BASE image:tag
BASE image:tag AS stage-name
```

### RUN
Executes commands during build
```
RUN command
RUN ["executable", "param1", "param2"]
```

### COPY / ADD
Copy files into the image
```
COPY source destination
COPY ["source", "destination"]
ADD source destination
```

### ENV
Set environment variables
```
ENV key value
ENV key=value
```

### WORKDIR
Set working directory
```
WORKDIR /path/to/dir
```

### EXPOSE
Declare exposed ports
```
EXPOSE port
EXPOSE port/protocol
```

### VOLUME
Create mount points
```
VOLUME /path
VOLUME ["/path1", "/path2"]
```

### USER
Set user/group
```
USER username
USER username:group
USER UID:GID
```

### ENTRYPOINT
Set container entrypoint
```
ENTRYPOINT ["executable", "param1"]
ENTRYPOINT command param1 param2
```

### CMD
Set default command
```
CMD ["executable", "param1"]
CMD command param1 param2
```

### LABEL
Add metadata labels
```
LABEL key=value
LABEL key1=value1 key2=value2
```

### ARG
Define build arguments
```
ARG name
ARG name=default_value
```

### HEALTHCHECK
Define health check
```
HEALTHCHECK [OPTIONS] CMD command
HEALTHCHECK NONE
```

### SHELL
Set default shell
```
SHELL ["executable", "parameters"]
```

### STOPSIGNAL
Set stop signal
```
STOPSIGNAL signal
```

### ONBUILD
Add trigger for child images
```
ONBUILD instruction
```

## Metadata

Govcfile supports metadata annotations using the `@` prefix:

```
@version=1.0
@author=team@example.com
@description=Production web server
@license=MIT
```

## Multi-Stage Builds

```govcfile
# Build stage
BASE golang:1.21 AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o app

# Runtime stage
BASE alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /build/app /app
ENTRYPOINT ["/app"]
```

## govc-compose Format

For multi-container applications, use `govc-compose.yml`:

```yaml
version: '1'
services:
  web:
    build: .
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://db:5432/myapp
    depends_on:
      - db
  
  db:
    image: postgres:15
    environment:
      - POSTGRES_DB=myapp
      - POSTGRES_PASSWORD=secret
    volumes:
      - db-data:/var/lib/postgresql/data

volumes:
  db-data:
```

## Migration from Dockerfile

| Dockerfile | Govcfile |
|------------|----------|
| FROM | BASE |
| All other instructions | Same |

The govc container system automatically detects and converts legacy Dockerfiles to the Govcfile format during processing.

## Integration with govc

Govcfiles are stored in your govc repository and trigger builds on commit:

```bash
# Add Govcfile to repository
echo 'BASE alpine:latest' > Govcfile
govc add Govcfile
govc commit -m "Add container definition"

# Container build is triggered automatically
```

## Build Process

1. **Parse**: Govcfile is parsed into instructions
2. **Execute**: Each instruction creates a memory layer
3. **Cache**: Layers are cached for reuse
4. **Store**: Final image stored in govc's container registry

## Security Policies

Govcfile builds respect repository security policies:

```go
// Example security policy
policy := &SecurityPolicy{
    AllowedBaseImages: []string{"alpine:*", "ubuntu:22.04"},
    MaxImageSize: 1024 * 1024 * 1024, // 1GB
    ScanRequired: true,
}
```

## Best Practices

1. **Use specific tags** for base images
2. **Minimize layers** by combining RUN commands
3. **Order instructions** from least to most frequently changing
4. **Use build arguments** for version flexibility
5. **Add health checks** for production containers
6. **Run as non-root** user when possible
7. **Use metadata** for documentation