package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Caia-Tech/govc"
	"github.com/Caia-Tech/govc/pkg/pipeline"
)

func main() {
	// Create a new memory-first repository
	repo := govc.NewRepository()
	
	if repo == nil {
		log.Fatal("Failed to create repository")
	}
	
	fmt.Println("✓ Repository created successfully")
	
	// Test pipeline functionality
	fmt.Println("\nTesting memory-first pipeline:")
	
	// Create test executor
	executor := pipeline.NewMemoryTestExecutor()
	if executor == nil {
		log.Fatal("Failed to create test executor")
	}
	fmt.Println("✓ Memory test executor created")
	
	// Test with some simple Go code
	testCode := `
package main
import "testing"
func TestExample(t *testing.T) {
	if 1+1 != 2 {
		t.Error("Math is broken")
	}
}`
	
	// Execute tests in memory
	result, err := executor.ExecuteTestsInMemory(context.Background(), map[string][]byte{
		"main.go": []byte("package main\nfunc main() {}\n"),
	}, map[string][]byte{
		"example_test.go": []byte(testCode),
	})
	
	if err != nil {
		log.Printf("Test execution error: %v", err)
	} else {
		fmt.Printf("✓ Tests executed in memory: %d passed, %d failed\n", 
			result.Passed, result.Failed)
		for _, test := range result.Tests {
			fmt.Printf("  - %s: %s\n", test.Name, test.Status)
		}
	}
	
	fmt.Println("\n✅ Pipeline functionality test completed!")
}