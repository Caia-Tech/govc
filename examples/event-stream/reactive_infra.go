package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/caiatech/govc"
)

// This example shows how govc transforms Git from a passive version control
// system into an active event stream. Every commit becomes an event that
// can trigger infrastructure changes, validations, or rollbacks.

func main() {
	// Create a memory-first repository for our infrastructure
	repo := govc.NewRepository()

	// Initialize with base configuration
	tx := repo.Transaction()
	tx.Add("terraform/main.tf", []byte(`
resource "aws_instance" "web" {
  count = 2
  instance_type = "t2.micro"
}`))
	tx.Add("k8s/deployment.yaml", []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
spec:
  replicas: 3`))
	tx.Add("monitoring/alerts.yaml", []byte(`
alerts:
  - name: high_cpu
    threshold: 80
  - name: low_memory
    threshold: 20`))
	tx.Validate()
	tx.Commit("Initial infrastructure state")

	// Set up reactive event handlers
	setupEventHandlers(repo)

	// Simulate infrastructure changes that trigger events
	simulateInfrastructureChanges(repo)

	// Keep the event stream running
	time.Sleep(5 * time.Second)

	fmt.Println("\nEvent stream demonstration complete!")
}

func setupEventHandlers(repo *govc.Repository) {
	// Handler 1: Terraform changes trigger plan/apply
	repo.Watch(func(event govc.CommitEvent) {
		if containsTerraformChanges(event) {
			fmt.Printf("\n🔷 Terraform change detected in commit %s\n", event.Hash[:7])
			fmt.Printf("   Running terraform plan...\n")

			// In real implementation, would run actual terraform
			time.Sleep(100 * time.Millisecond)

			if isAutoApproved(event) {
				fmt.Printf("   Auto-applying changes (author: %s)\n", event.Author)
			} else {
				fmt.Printf("   Manual approval required for %s\n", event.Author)
			}
		}
	})

	// Handler 2: Kubernetes changes trigger validation
	repo.Watch(func(event govc.CommitEvent) {
		if containsK8sChanges(event) {
			fmt.Printf("\n☸️  Kubernetes change detected in commit %s\n", event.Hash[:7])

			// Create a test reality to validate changes
			testReality := repo.ParallelReality("k8s-validation")

			fmt.Printf("   Validating in parallel reality: %s\n", testReality.Name())
			fmt.Printf("   ✓ Syntax valid\n")
			fmt.Printf("   ✓ Resource limits OK\n")
			fmt.Printf("   ✓ Security policies passed\n")
		}
	})

	// Handler 3: Monitoring changes trigger immediate updates
	repo.Watch(func(event govc.CommitEvent) {
		if containsMonitoringChanges(event) {
			fmt.Printf("\n📊 Monitoring change detected in commit %s\n", event.Hash[:7])
			fmt.Printf("   Updating alert rules immediately...\n")
			fmt.Printf("   New alerts active in all regions\n")
		}
	})

	// Handler 4: Rollback handler for failed deployments
	repo.Watch(func(event govc.CommitEvent) {
		if strings.Contains(event.Message, "EMERGENCY") {
			fmt.Printf("\n🚨 EMERGENCY ROLLBACK triggered by %s!\n", event.Author)

			// Get the previous good state
			commits, _ := repo.Log(2)
			if len(commits) >= 2 {
				previousCommit := commits[1]
				fmt.Printf("   Rolling back to %s: %s\n",
					previousCommit.Hash()[:7],
					previousCommit.Message)

				// In govc, rollback is just a branch switch - instant!
				repo.Checkout(previousCommit.Hash())
				fmt.Printf("   ✓ Rollback complete in microseconds\n")
			}
		}
	})

	// Handler 5: Compliance and audit logging
	repo.Watch(func(event govc.CommitEvent) {
		// Log all infrastructure changes for compliance
		logEntry := fmt.Sprintf("[%s] %s by %s: %s",
			event.Timestamp.Format("2006-01-02 15:04:05"),
			event.Hash[:7],
			event.Author,
			event.Message,
		)

		// In production, this would go to audit log
		fmt.Printf("\n📝 Audit: %s\n", logEntry)
	})
}

func simulateInfrastructureChanges(repo *govc.Repository) {
	time.Sleep(1 * time.Second)

	// Change 1: Scale up Terraform instances
	fmt.Println("\n--- Simulating infrastructure changes ---")

	scaleTx := repo.Transaction()
	scaleTx.Add("terraform/main.tf", []byte(`
resource "aws_instance" "web" {
  count = 5  # Scaled from 2 to 5
  instance_type = "t2.small"  # Upgraded from t2.micro
}`))
	scaleTx.Validate()
	scaleTx.Commit("Scale web instances for Black Friday")

	time.Sleep(1 * time.Second)

	// Change 2: Update Kubernetes deployment
	k8sTx := repo.Transaction()
	k8sTx.Add("k8s/deployment.yaml", []byte(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-app
spec:
  replicas: 10  # Scaled from 3 to 10
  template:
    spec:
      containers:
      - name: app
        resources:
          limits:
            memory: "512Mi"
            cpu: "500m"`))
	k8sTx.Validate()
	k8sTx.Commit("Increase K8s replicas and set resource limits")

	time.Sleep(1 * time.Second)

	// Change 3: Update monitoring alerts
	alertTx := repo.Transaction()
	alertTx.Add("monitoring/alerts.yaml", []byte(`
alerts:
  - name: high_cpu
    threshold: 90  # Increased from 80
  - name: low_memory  
    threshold: 10  # Decreased from 20
  - name: high_latency  # New alert
    threshold: 500`))
	alertTx.Validate()
	alertTx.Commit("Adjust alert thresholds for scaled infrastructure")

	time.Sleep(1 * time.Second)

	// Change 4: Emergency rollback
	emergencyTx := repo.Transaction()
	emergencyTx.Add("terraform/main.tf", []byte(`
# This change has issues
resource "aws_instance" "web" {
  count = 0  # Accidentally set to 0!
}`))
	emergencyTx.Validate()
	emergencyTx.Commit("EMERGENCY: Fixing production issue")
}

// Helper functions to detect change types
func containsTerraformChanges(event govc.CommitEvent) bool {
	// In real implementation, would check event.Changes
	return strings.Contains(event.Message, "Scale") ||
		strings.Contains(event.Message, "Terraform")
}

func containsK8sChanges(event govc.CommitEvent) bool {
	return strings.Contains(event.Message, "K8s") ||
		strings.Contains(event.Message, "replicas")
}

func containsMonitoringChanges(event govc.CommitEvent) bool {
	return strings.Contains(event.Message, "alert") ||
		strings.Contains(event.Message, "monitoring")
}

func isAutoApproved(event govc.CommitEvent) bool {
	// Example: auto-approve for certain authors or small changes
	return event.Author == "automation" ||
		strings.Contains(event.Message, "Auto-approved")
}

// ReactiveInfrastructure demonstrates a more complex reactive pattern
type ReactiveInfrastructure struct {
	repo     *govc.Repository
	handlers map[string][]EventHandler
}

type EventHandler func(event govc.CommitEvent) error

func NewReactiveInfrastructure(repo *govc.Repository) *ReactiveInfrastructure {
	return &ReactiveInfrastructure{
		repo:     repo,
		handlers: make(map[string][]EventHandler),
	}
}

// On registers a handler for specific file patterns
func (ri *ReactiveInfrastructure) On(pattern string, handler EventHandler) {
	ri.handlers[pattern] = append(ri.handlers[pattern], handler)
}

// Example of a more sophisticated event system
func (ri *ReactiveInfrastructure) Start() {
	ri.repo.Watch(func(event govc.CommitEvent) {
		for pattern, handlers := range ri.handlers {
			if ri.matchesPattern(event, pattern) {
				for _, handler := range handlers {
					if err := handler(event); err != nil {
						// On error, create a branch to fix the issue
						fixBranch := ri.repo.ParallelReality("auto-fix-" + event.Hash[:7])
						log.Printf("Created fix branch: %s", fixBranch.Name())
					}
				}
			}
		}
	})
}

func (ri *ReactiveInfrastructure) matchesPattern(event govc.CommitEvent, pattern string) bool {
	// Simplified pattern matching
	return true
}
