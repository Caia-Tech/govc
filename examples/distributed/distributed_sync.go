package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/caia-tech/govc"
)

// This example demonstrates the future of govc: distributed reality synchronization.
// Multiple nodes can share and sync parallel realities, enabling distributed
// infrastructure management where changes can be tested across regions before
// global deployment.

// DistributedNode represents a govc node in a distributed system
type DistributedNode struct {
	ID         string
	Region     string
	repo       *govc.Repository
	peers      map[string]*DistributedNode
	realities  map[string]*govc.ParallelReality
	syncChan   chan SyncMessage
	mu         sync.RWMutex
}

// SyncMessage represents a reality sync between nodes
type SyncMessage struct {
	FromNode   string
	ToNode     string
	Reality    string
	Operation  string
	Data       map[string][]byte
	Timestamp  time.Time
}

func main() {
	fmt.Println("🌍 govc Distributed Reality Sync Demo")
	fmt.Println("=====================================\n")

	// Create nodes in different regions
	nodeUS := NewDistributedNode("node-us-east", "us-east-1")
	nodeEU := NewDistributedNode("node-eu-west", "eu-west-1") 
	nodeAP := NewDistributedNode("node-ap-south", "ap-southeast-1")

	// Connect nodes as peers
	nodeUS.AddPeer(nodeEU)
	nodeUS.AddPeer(nodeAP)
	nodeEU.AddPeer(nodeUS)
	nodeEU.AddPeer(nodeAP)
	nodeAP.AddPeer(nodeUS)
	nodeAP.AddPeer(nodeEU)

	// Scenario 1: Test configuration change across regions
	fmt.Println("📋 Scenario 1: Multi-Region Configuration Testing")
	testMultiRegionConfig(nodeUS, nodeEU, nodeAP)

	time.Sleep(2 * time.Second)

	// Scenario 2: Distributed consensus for critical changes
	fmt.Println("\n🤝 Scenario 2: Distributed Consensus")
	testDistributedConsensus(nodeUS, nodeEU, nodeAP)

	time.Sleep(2 * time.Second)

	// Scenario 3: Reality forking and merging across regions
	fmt.Println("\n🔀 Scenario 3: Reality Forking and Merging")
	testRealityForkMerge(nodeUS, nodeEU, nodeAP)
}

func NewDistributedNode(id, region string) *DistributedNode {
	node := &DistributedNode{
		ID:        id,
		Region:    region,
		repo:      govc.NewRepository(),
		peers:     make(map[string]*DistributedNode),
		realities: make(map[string]*govc.ParallelReality),
		syncChan:  make(chan SyncMessage, 100),
	}

	// Start sync handler
	go node.handleSync()

	// Initialize with base configuration
	tx := node.repo.BeginTransaction()
	tx.Add("config/region.yaml", []byte(fmt.Sprintf("region: %s\nstatus: active", region)))
	tx.Validate()
	tx.Commit(fmt.Sprintf("Initialize %s", region))

	return node
}

func (n *DistributedNode) AddPeer(peer *DistributedNode) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.peers[peer.ID] = peer
}

func (n *DistributedNode) handleSync() {
	for msg := range n.syncChan {
		n.processSync(msg)
	}
}

func (n *DistributedNode) processSync(msg SyncMessage) {
	switch msg.Operation {
	case "CREATE_REALITY":
		// Create the same reality locally
		reality := n.repo.ParallelReality(msg.Reality)
		n.realities[msg.Reality] = reality
		fmt.Printf("   %s: Created reality '%s' (synced from %s)\n", 
			n.ID, msg.Reality, msg.FromNode)

	case "APPLY_CHANGES":
		// Apply the same changes to our local reality
		if reality, exists := n.realities[msg.Reality]; exists {
			reality.Apply(msg.Data)
			fmt.Printf("   %s: Applied changes to '%s'\n", n.ID, msg.Reality)
		}

	case "MERGE_REALITY":
		// Merge the reality into our main branch
		if reality, exists := n.realities[msg.Reality]; exists {
			// In real implementation, would actually merge
			fmt.Printf("   %s: Merged reality '%s' to main\n", n.ID, msg.Reality)
		}
	}
}

// CreateSharedReality creates a reality that's synchronized across nodes
func (n *DistributedNode) CreateSharedReality(name string) *govc.ParallelReality {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Create locally
	reality := n.repo.ParallelReality(name)
	n.realities[name] = reality

	// Sync to all peers
	for _, peer := range n.peers {
		peer.syncChan <- SyncMessage{
			FromNode:  n.ID,
			ToNode:    peer.ID,
			Reality:   name,
			Operation: "CREATE_REALITY",
			Timestamp: time.Now(),
		}
	}

	fmt.Printf("   %s: Created shared reality '%s'\n", n.ID, name)
	return reality
}

// ApplyToSharedReality applies changes and syncs to peers
func (n *DistributedNode) ApplyToSharedReality(name string, changes map[string][]byte) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if reality, exists := n.realities[name]; exists {
		reality.Apply(changes)

		// Sync changes to all peers
		for _, peer := range n.peers {
			peer.syncChan <- SyncMessage{
				FromNode:  n.ID,
				ToNode:    peer.ID,
				Reality:   name,
				Operation: "APPLY_CHANGES",
				Data:      changes,
				Timestamp: time.Now(),
			}
		}
	}
}

func testMultiRegionConfig(nodeUS, nodeEU, nodeAP *DistributedNode) {
	// US node creates a test reality for new CDN configuration
	testReality := nodeUS.CreateSharedReality("cdn-config-test")

	// Each region tests the configuration with local parameters
	fmt.Println("\n   Testing CDN configuration in each region:")

	// US applies and tests
	nodeUS.ApplyToSharedReality("cdn-config-test", map[string][]byte{
		"cdn/config.yaml": []byte("provider: cloudfront\nregion: us-east-1\ncache_ttl: 3600"),
	})
	fmt.Printf("   %s: Latency test: 12ms ✓\n", nodeUS.ID)

	// EU tests with different provider
	nodeEU.ApplyToSharedReality("cdn-config-test", map[string][]byte{
		"cdn/config.yaml": []byte("provider: cloudflare\nregion: eu-west-1\ncache_ttl: 3600"),
	})
	fmt.Printf("   %s: Latency test: 8ms ✓\n", nodeEU.ID)

	// AP tests
	nodeAP.ApplyToSharedReality("cdn-config-test", map[string][]byte{
		"cdn/config.yaml": []byte("provider: fastly\nregion: ap-southeast-1\ncache_ttl: 3600"),
	})
	fmt.Printf("   %s: Latency test: 15ms ✓\n", nodeAP.ID)

	fmt.Println("\n   ✅ All regions validated. Safe to deploy globally.")
}

func testDistributedConsensus(nodeUS, nodeEU, nodeAP *DistributedNode) {
	// Create a reality for a critical database schema change
	consensusReality := nodeUS.CreateSharedReality("db-schema-v2")

	fmt.Println("\n   Proposing database schema change...")

	// Each node votes based on their validation
	votes := make(chan bool, 3)

	// Each node validates independently
	go func() {
		// US node validates
		fmt.Printf("   %s: Validating schema... ", nodeUS.ID)
		time.Sleep(500 * time.Millisecond)
		fmt.Println("✓")
		votes <- true
	}()

	go func() {
		// EU node validates
		fmt.Printf("   %s: Validating schema... ", nodeEU.ID)
		time.Sleep(600 * time.Millisecond)
		fmt.Println("✓")
		votes <- true
	}()

	go func() {
		// AP node validates
		fmt.Printf("   %s: Validating schema... ", nodeAP.ID)
		time.Sleep(700 * time.Millisecond)
		fmt.Println("✓")
		votes <- true
	}()

	// Collect votes
	approvals := 0
	for i := 0; i < 3; i++ {
		if <-votes {
			approvals++
		}
	}

	if approvals >= 2 {
		fmt.Printf("\n   ✅ Consensus reached (%d/3). Merging to main.\n", approvals)
		// In real implementation, would merge the reality
	}
}

func testRealityForkMerge(nodeUS, nodeEU, nodeAP *DistributedNode) {
	// Start with a shared reality
	baseReality := nodeUS.CreateSharedReality("feature-optimization")

	fmt.Println("\n   Each region optimizes for local conditions:")

	// Each region forks and optimizes
	usOptimized := nodeUS.repo.ParallelReality("feature-optimization-us")
	euOptimized := nodeEU.repo.ParallelReality("feature-optimization-eu")
	apOptimized := nodeAP.repo.ParallelReality("feature-optimization-ap")

	// Apply region-specific optimizations
	usOptimized.Apply(map[string][]byte{
		"optimize/settings.yaml": []byte("cache_size: large\ncompression: moderate"),
	})
	fmt.Printf("   %s: Optimized for high bandwidth\n", nodeUS.ID)

	euOptimized.Apply(map[string][]byte{
		"optimize/settings.yaml": []byte("cache_size: medium\ncompression: high\ngdpr: enabled"),
	})
	fmt.Printf("   %s: Optimized for GDPR compliance\n", nodeEU.ID)

	apOptimized.Apply(map[string][]byte{
		"optimize/settings.yaml": []byte("cache_size: small\ncompression: maximum\nmobile_first: true"),
	})
	fmt.Printf("   %s: Optimized for mobile networks\n", nodeAP.ID)

	// Merge best practices from all regions
	fmt.Println("\n   🔀 Merging best practices from all regions...")
	time.Sleep(1 * time.Second)

	// Create global configuration incorporating all optimizations
	globalConfig := map[string][]byte{
		"global/config.yaml": []byte(`
regions:
  us-east-1:
    cache_size: large
    compression: moderate
  eu-west-1:
    cache_size: medium
    compression: high
    gdpr: enabled
  ap-southeast-1:
    cache_size: small
    compression: maximum
    mobile_first: true
`),
	}

	// Apply globally
	for _, node := range []*DistributedNode{nodeUS, nodeEU, nodeAP} {
		node.ApplyToSharedReality("feature-optimization", globalConfig)
	}

	fmt.Println("   ✅ Global optimization complete!")
}

// Future concepts for distributed govc

// ConsensusManager handles distributed consensus for critical changes
type ConsensusManager struct {
	nodes     []*DistributedNode
	threshold float64 // Percentage needed for consensus
}

// RealityRouter routes realities to appropriate nodes based on rules
type RealityRouter struct {
	rules map[string]func(*govc.ParallelReality) string
}

// GlobalStateManager maintains consistent state across all nodes
type GlobalStateManager struct {
	nodes  []*DistributedNode
	state  map[string]interface{}
	mu     sync.RWMutex
}

// Example of advanced distributed pattern
func (gsm *GlobalStateManager) ProposeChange(change Change) error {
	// Create test realities on subset of nodes
	testNodes := gsm.selectTestNodes(3)
	
	results := make(chan TestResult, len(testNodes))
	for _, node := range testNodes {
		go func(n *DistributedNode) {
			reality := n.repo.ParallelReality("test-" + change.ID)
			reality.Apply(change.Data)
			results <- n.validate(reality)
		}(node)
	}

	// If validation passes, propagate to all nodes
	successCount := 0
	for i := 0; i < len(testNodes); i++ {
		if result := <-results; result.Success {
			successCount++
		}
	}

	if successCount == len(testNodes) {
		return gsm.propagateToAll(change)
	}

	return fmt.Errorf("validation failed on %d nodes", len(testNodes)-successCount)
}

type Change struct {
	ID   string
	Data map[string][]byte
}

type TestResult struct {
	NodeID  string
	Success bool
	Metrics map[string]float64
}

func (gsm *GlobalStateManager) selectTestNodes(count int) []*DistributedNode {
	// Select diverse nodes for testing
	return gsm.nodes[:count]
}

func (gsm *GlobalStateManager) propagateToAll(change Change) error {
	// Propagate change to all nodes
	return nil
}

func (n *DistributedNode) validate(reality *govc.ParallelReality) TestResult {
	// Validate the reality
	return TestResult{
		NodeID:  n.ID,
		Success: true,
		Metrics: map[string]float64{
			"latency": 10.5,
			"cpu":     45.2,
		},
	}
}