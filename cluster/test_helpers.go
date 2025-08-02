package cluster

import (
	"fmt"
	"time"
)

// Helper function to create a node with simple parameters for tests
func createTestNode(id, address string, port int) (*Node, error) {
	config := NodeConfig{
		ID:                id,
		Address:           address,
		Port:              port,
		DataDir:           fmt.Sprintf("/tmp/test-node-%s", id),
		ElectionTimeout:   150 * time.Millisecond,
		HeartbeatTimeout:  50 * time.Millisecond,
		MaxLogEntries:     1000,
		SnapshotThreshold: 500,
	}
	return NewNode(config)
}

// Helper function to create test nodes
func createTestNodes(count int) ([]*Node, error) {
	nodes := make([]*Node, count)
	for i := 0; i < count; i++ {
		node, err := createTestNode(fmt.Sprintf("node%d", i+1), "127.0.0.1", 8080+i)
		if err != nil {
			return nil, err
		}
		nodes[i] = node
	}
	return nodes, nil
}
