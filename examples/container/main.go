package main

import (
	"fmt"
	"log"
	"time"

	"github.com/caiatech/govc"
	"github.com/caiatech/govc/container"
)

func main() {
	// Create a new govc repository
	repo := govc.New()
	repo.SetConfig("user.name", "Container Example")
	repo.SetConfig("user.email", "example@govc.dev")

	// Create container manager
	mgr := container.NewManager()

	// Track container events
	mgr.OnEvent(func(event container.ContainerEvent) {
		fmt.Printf("Container event: %s at %s\n", event.Type, event.Timestamp.Format(time.RFC3339))
	})

	// Register repository with container manager
	if err := mgr.RegisterRepository("example-app", repo); err != nil {
		log.Fatal(err)
	}

	// Add a Dockerfile
	dockerfile := `FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
CMD ["./main"]`

	repo.WriteFile("Dockerfile", []byte(dockerfile))

	// Add docker-compose.yml
	dockerCompose := `version: '3.8'
services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - APP_ENV=production
    restart: unless-stopped`

	repo.WriteFile("docker-compose.yml", []byte(dockerCompose))

	// Add Kubernetes deployment
	k8sDeployment := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: example-app
  labels:
    app: example-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: example-app
  template:
    metadata:
      labels:
        app: example-app
    spec:
      containers:
      - name: app
        image: example-app:latest
        ports:
        - containerPort: 8080
        env:
        - name: APP_ENV
          value: production`

	repo.WriteFile("k8s/deployment.yaml", []byte(k8sDeployment))

	// Stage and commit all files
	repo.Add("Dockerfile", "docker-compose.yml", "k8s/deployment.yaml")
	commit, err := repo.Commit("Add container definitions")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created commit: %s\n", commit.Hash())

	// List container definitions
	definitions, err := mgr.GetContainerDefinitions("example-app")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("\nFound %d container definitions:\n", len(definitions))
	for _, def := range definitions {
		fmt.Printf("- %s (%s)\n", def.Path, def.Type)
	}

	// Start a build
	fmt.Println("\nStarting container build...")
	build, err := mgr.StartBuild(container.BuildRequest{
		RepositoryID: "example-app",
		Dockerfile:   "Dockerfile",
		Context:      ".",
		Tags:         []string{"example-app:latest", "example-app:v1.0"},
		Args: map[string]string{
			"VERSION": "1.0.0",
		},
		Labels: map[string]string{
			"maintainer": "govc team",
			"version":    "1.0.0",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Build started: %s\n", build.ID)

	// Wait for build to complete
	time.Sleep(3 * time.Second)

	// Check build status
	completedBuild, err := mgr.GetBuild(build.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Build status: %s\n", completedBuild.Status)
	if completedBuild.Status == container.BuildStatusCompleted {
		fmt.Printf("Image ID: %s\n", completedBuild.ImageID)
		fmt.Printf("Digest: %s\n", completedBuild.Digest)
	}

	// Use parallel realities for testing different configurations
	fmt.Println("\nTesting with parallel realities...")
	
	// Create parallel realities for different environments
	envs := []string{"staging", "production", "canary"}
	realities := repo.ParallelRealities(envs)

	// Apply different configurations to each reality
	for i, reality := range realities {
		// Update deployment for each environment
		replicas := (i + 1) * 2 // staging=2, production=4, canary=6
		k8sUpdate := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: example-app-%s
spec:
  replicas: %d`, envs[i], replicas)

		reality.Apply(map[string][]byte{
			"k8s/deployment.yaml": []byte(k8sUpdate),
		})

		fmt.Printf("Updated %s environment with %d replicas\n", envs[i], replicas)
	}

	// Demonstrate transactional updates
	fmt.Println("\nDemonstrating transactional container updates...")
	tx := repo.Transaction()
	
	// Update multiple container definitions atomically
	tx.Add("Dockerfile", []byte("FROM golang:1.22-alpine AS builder\n# Updated base image"))
	tx.Add("docker-compose.yml", []byte("version: '3.9'\n# Updated compose version"))
	
	// Validate before committing
	if err := tx.Validate(); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
		tx.Rollback()
	} else {
		commit := tx.Commit("Update container configurations")
		fmt.Printf("Transaction committed: %s\n", commit.Hash())
	}

	fmt.Println("\nContainer system example completed!")
}