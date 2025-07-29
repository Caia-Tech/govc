# Git as Event Stream: Reactive Infrastructure with govc

This example demonstrates how govc transforms Git from a passive version control system into an active event stream that powers reactive infrastructure.

## The Paradigm Shift

Traditional Git: Store changes → Manual deployment → Hope nothing breaks

govc Event Stream: Store changes → Automatic reactions → Self-healing infrastructure

## How It Works

Every commit in govc becomes an event that can trigger:
- Terraform plans and applies
- Kubernetes validations
- Monitoring updates
- Automatic rollbacks
- Compliance logging

## Example Scenarios

### 1. Terraform Auto-Apply
```go
repo.Watch(func(event CommitEvent) {
    if event.Changes("*.tf") {
        terraform.Plan(event.Branch())
        if approved {
            terraform.Apply()
        }
    }
})
```

### 2. Kubernetes Validation
When K8s files change:
1. Create a parallel reality for testing
2. Validate syntax and security policies
3. Test resource limits
4. Only merge if all checks pass

### 3. Emergency Rollback
Commit message contains "EMERGENCY":
1. Immediately switch to previous commit
2. Because govc is memory-first, this takes microseconds
3. No waiting for file operations

### 4. Multi-Stage Reactions
```
Commit → Validate → Test in parallel reality → Deploy → Monitor
   ↓                     ↓                        ↓         ↓
 Audit               If fails,                If fails,  Alert
  Log                rollback               auto-scale   Team
```

## Running the Example

```bash
go run reactive_infra.go
```

Watch as infrastructure changes trigger immediate reactions:
- Terraform changes → Automatic planning
- K8s changes → Parallel validation
- Alert changes → Instant updates
- Emergency commits → Immediate rollback

## Real-World Applications

### GitOps on Steroids
- Every commit triggers appropriate actions
- No polling delays - instant reactions
- Parallel realities for safe testing

### Self-Healing Infrastructure
- Detect problems in commit stream
- Automatically create fix branches
- Test fixes in parallel realities
- Merge when healthy

### Compliance and Audit
- Every change logged with context
- Automatic compliance checks
- Policy enforcement via event handlers

## Key Insight

> "When version control becomes an event stream, infrastructure becomes reactive. 
> Changes don't wait for deployment pipelines - they trigger immediate, 
> appropriate responses."

## Advanced Patterns

### Event Routing
```go
infra.On("terraform/*.tf", terraformHandler)
infra.On("k8s/*.yaml", kubernetesHandler)  
infra.On("security/*", securityHandler)
```

### Conditional Reactions
```go
if event.Author == "automation" && event.Hour() < 6 {
    // Auto-approve off-hours automation changes
    approve()
} else {
    // Require manual approval
    notify(oncall)
}
```

### Reality-Based Testing
```go
// Test change in 3 different realities simultaneously
realities := repo.ParallelRealities(["staging", "qa", "perf"])
results := testAll(realities)
if allPass(results) {
    repo.Merge(change, "production")
}
```