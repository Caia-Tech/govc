package main

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Caia-Tech/govc"
)

// This example shows how govc enables true parallel testing.
// Each test runs in its own isolated reality - they can modify
// the same files without conflicts because each exists in its
// own universe.

// MockDatabase simulates a database configuration
type MockDatabase struct {
	Host     string
	Port     int
	Replicas int
}

// TestParallelUniverses demonstrates running tests in isolated branches
func TestParallelUniverses(t *testing.T) {
	// Create a memory-first repository
	repo := govc.NewRepository()

	// Set up initial state
	tx := repo.Transaction()
	tx.Add("database.conf", []byte("host=localhost\nport=5432\nreplicas=1"))
	tx.Add("app.conf", []byte("debug=false\nworkers=4"))
	tx.Validate()
	tx.Commit("Initial test state")

	// Define test cases that would normally conflict
	tests := []struct {
		name     string
		modifier func(*govc.ParallelReality)
		verify   func(*govc.ParallelReality) error
	}{
		{
			name: "test_scaling_up",
			modifier: func(pr *govc.ParallelReality) {
				// This test scales up the database
				pr.Apply(map[string][]byte{
					"database.conf": []byte("host=cluster.local\nport=5432\nreplicas=5"),
					"app.conf":      []byte("debug=false\nworkers=16"),
				})
			},
			verify: func(pr *govc.ParallelReality) error {
				// Verify scaling worked
				return nil
			},
		},
		{
			name: "test_scaling_down",
			modifier: func(pr *govc.ParallelReality) {
				// This test scales down - normally would conflict with scaling up
				pr.Apply(map[string][]byte{
					"database.conf": []byte("host=localhost\nport=5432\nreplicas=0"),
					"app.conf":      []byte("debug=true\nworkers=1"),
				})
			},
			verify: func(pr *govc.ParallelReality) error {
				// Verify minimal config works
				return nil
			},
		},
		{
			name: "test_failover",
			modifier: func(pr *govc.ParallelReality) {
				// Test failover scenario
				pr.Apply(map[string][]byte{
					"database.conf": []byte("host=backup.local\nport=5433\nreplicas=3"),
					"app.conf":      []byte("debug=false\nworkers=8\nfailover=true"),
				})
			},
			verify: func(pr *govc.ParallelReality) error {
				// Verify failover configuration
				return nil
			},
		},
	}

	// Run all tests in parallel - each in its own reality
	var wg sync.WaitGroup
	results := make(chan TestResult, len(tests))

	for _, tc := range tests {
		wg.Add(1)
		go func(test struct {
			name     string
			modifier func(*govc.ParallelReality)
			verify   func(*govc.ParallelReality) error
		}) {
			defer wg.Done()

			start := time.Now()

			// Create an isolated reality for this test
			reality := repo.IsolatedBranch(test.name)

			// Apply test modifications
			test.modifier(reality)

			// Run verification
			err := test.verify(reality)

			results <- TestResult{
				Name:     test.name,
				Duration: time.Since(start),
				Error:    err,
				Reality:  reality,
			}
		}(tc)
	}

	wg.Wait()
	close(results)

	// Collect and display results
	fmt.Println("\nTest Results:")
	fmt.Println("=============")
	for result := range results {
		status := "PASS"
		if result.Error != nil {
			status = "FAIL"
		}
		fmt.Printf("%s: %s (%.2fms)\n", result.Name, status,
			float64(result.Duration.Microseconds())/1000)
	}
}

type TestResult struct {
	Name     string
	Duration time.Duration
	Error    error
	Reality  *govc.ParallelReality
}

// BenchmarkParallelRealities shows the performance of parallel testing
func BenchmarkParallelRealities(b *testing.B) {
	repo := govc.NewRepository()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create 100 parallel realities
		realities := make([]*govc.ParallelReality, 100)
		for j := 0; j < 100; j++ {
			realities[j] = repo.ParallelReality(fmt.Sprintf("bench-%d-%d", i, j))
		}

		// Each reality can be modified independently
		var wg sync.WaitGroup
		for _, reality := range realities {
			wg.Add(1)
			go func(r *govc.ParallelReality) {
				defer wg.Done()
				r.Apply(map[string][]byte{
					"test.txt": []byte("parallel universe testing"),
				})
			}(reality)
		}
		wg.Wait()
	}
}

// Example of a test suite that benefits from parallel realities
type IntegrationTestSuite struct {
	repo *govc.Repository
}

func (s *IntegrationTestSuite) RunParallel() {
	tests := []string{
		"TestUserAuthentication",
		"TestDataMigration",
		"TestAPIEndpoints",
		"TestCaching",
		"TestRateLimiting",
	}

	// Traditional approach: tests run sequentially to avoid conflicts
	// With govc: all tests run simultaneously in different realities

	var wg sync.WaitGroup
	for _, testName := range tests {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			// Each test gets its own reality
			reality := s.repo.IsolatedBranch(name)

			// Tests can do anything without affecting others:
			// - Modify configuration files
			// - Change database schemas
			// - Alter system state
			// All changes are isolated to this reality

			fmt.Printf("%s running in reality %s\n", name, reality.Name())
		}(testName)
	}

	wg.Wait()
}

func main() {
	// Run the parallel universe tests
	testing.Main(func(pat, str string) (bool, error) {
		return true, nil
	}, []testing.InternalTest{
		{
			Name: "TestParallelUniverses",
			F:    TestParallelUniverses,
		},
	}, []testing.InternalBenchmark{
		{
			Name: "BenchmarkParallelRealities",
			F:    BenchmarkParallelRealities,
		},
	}, nil)
}
