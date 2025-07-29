package main

import (
	"fmt"
	"log"
	"sync"

	"github.com/caia-tech/govc"
)

// This example demonstrates how govc's memory-first approach enables
// safe infrastructure configuration testing. Traditional approaches require
// separate environments or risk affecting production. With govc, we can
// test multiple configurations in parallel isolated realities.

type LoadBalancerConfig struct {
	Algorithm   string
	MaxConn     int
	HealthCheck string
}

func main() {
	// Create a memory-first repository for our infrastructure state
	repo := govc.NewRepository()
	
	// Initialize with current production config
	tx := repo.BeginTransaction()
	tx.Add("lb/config.json", []byte(`{
		"algorithm": "round-robin",
		"max_connections": 1000,
		"health_check": "/health"
	}`))
	if err := tx.Validate(); err != nil {
		log.Fatal(err)
	}
	tx.Commit("Initial production config")

	// Test multiple configurations in parallel realities
	configs := []LoadBalancerConfig{
		{Algorithm: "least-conn", MaxConn: 2000, HealthCheck: "/health"},
		{Algorithm: "ip-hash", MaxConn: 1500, HealthCheck: "/status"},
		{Algorithm: "weighted", MaxConn: 3000, HealthCheck: "/ping"},
	}

	// Each configuration runs in its own reality
	results := make(chan TestResult, len(configs))
	var wg sync.WaitGroup

	for i, config := range configs {
		wg.Add(1)
		go func(cfg LoadBalancerConfig, id int) {
			defer wg.Done()
			
			// Create an isolated reality for this test
			reality := repo.ParallelReality(fmt.Sprintf("test-config-%d", id))
			
			// Apply the configuration in this reality
			configJSON := fmt.Sprintf(`{
				"algorithm": "%s",
				"max_connections": %d,
				"health_check": "%s"
			}`, cfg.Algorithm, cfg.MaxConn, cfg.HealthCheck)
			
			reality.Apply(map[string][]byte{
				"lb/config.json": []byte(configJSON),
			})
			
			// Simulate performance testing in this reality
			result := TestResult{
				Config:     cfg,
				Throughput: simulateThroughput(cfg),
				Latency:    simulateLatency(cfg),
				ErrorRate:  simulateErrorRate(cfg),
			}
			
			results <- result
		}(config, i)
	}

	wg.Wait()
	close(results)

	// Find the best performing configuration
	var best TestResult
	for result := range results {
		fmt.Printf("Config %s: Throughput=%d req/s, Latency=%dms, Errors=%.2f%%\n",
			result.Config.Algorithm,
			result.Throughput,
			result.Latency,
			result.ErrorRate,
		)
		
		if result.Score() > best.Score() {
			best = result
		}
	}

	// The winning configuration can now be applied to production
	fmt.Printf("\nBest configuration: %s with score %.2f\n", 
		best.Config.Algorithm, best.Score())
	
	// In a real system, you would:
	// 1. Create a branch for the winning config
	// 2. Run additional validation
	// 3. Merge to main branch
	// 4. Deploy the change
}

type TestResult struct {
	Config     LoadBalancerConfig
	Throughput int     // requests per second
	Latency    int     // milliseconds
	ErrorRate  float64 // percentage
}

func (tr TestResult) Score() float64 {
	// Simple scoring: higher throughput, lower latency and errors
	return float64(tr.Throughput) / float64(tr.Latency) * (1 - tr.ErrorRate)
}

// Simulated metrics based on algorithm
func simulateThroughput(cfg LoadBalancerConfig) int {
	base := cfg.MaxConn * 10
	switch cfg.Algorithm {
	case "least-conn":
		return base + 200
	case "ip-hash":
		return base + 100
	case "weighted":
		return base + 300
	default:
		return base
	}
}

func simulateLatency(cfg LoadBalancerConfig) int {
	switch cfg.Algorithm {
	case "least-conn":
		return 45
	case "ip-hash":
		return 50
	case "weighted":
		return 40
	default:
		return 55
	}
}

func simulateErrorRate(cfg LoadBalancerConfig) float64 {
	switch cfg.Algorithm {
	case "least-conn":
		return 0.01
	case "ip-hash":
		return 0.02
	case "weighted":
		return 0.005
	default:
		return 0.03
	}
}