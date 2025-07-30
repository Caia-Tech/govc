#!/bin/bash

# govc REST API Example Client
# This script demonstrates the main features of the govc REST API

BASE_URL="http://localhost:8080/api/v1"
REPO_ID="demo-$(date +%s)"

echo "🚀 govc REST API Demo"
echo "===================="
echo ""

# 1. Create a repository
echo "1️⃣  Creating repository: $REPO_ID"
curl -s -X POST $BASE_URL/repos \
  -H "Content-Type: application/json" \
  -d "{\"id\": \"$REPO_ID\", \"memory_only\": true}" | jq .

echo ""

# 2. Check repository status
echo "2️⃣  Checking repository status"
curl -s $BASE_URL/repos/$REPO_ID/status | jq .

echo ""

# 3. Begin a transaction
echo "3️⃣  Starting a transaction"
TX_RESPONSE=$(curl -s -X POST $BASE_URL/repos/$REPO_ID/transaction)
TX_ID=$(echo $TX_RESPONSE | jq -r '.id')
echo "Transaction ID: $TX_ID"

echo ""

# 4. Add files to transaction
echo "4️⃣  Adding files to transaction"
curl -s -X POST $BASE_URL/repos/$REPO_ID/transaction/$TX_ID/add \
  -H "Content-Type: application/json" \
  -d '{"path": "README.md", "content": "# Demo Repository\n\nThis is a memory-first Git repository!"}' | jq .

curl -s -X POST $BASE_URL/repos/$REPO_ID/transaction/$TX_ID/add \
  -H "Content-Type: application/json" \
  -d '{"path": "config.yaml", "content": "version: 1.0\nname: demo"}' | jq .

echo ""

# 5. Validate transaction
echo "5️⃣  Validating transaction"
curl -s -X POST $BASE_URL/repos/$REPO_ID/transaction/$TX_ID/validate | jq .

echo ""

# 6. Commit transaction
echo "6️⃣  Committing transaction"
COMMIT_RESPONSE=$(curl -s -X POST $BASE_URL/repos/$REPO_ID/transaction/$TX_ID/commit \
  -H "Content-Type: application/json" \
  -d '{"message": "Initial commit with README and config"}')
echo $COMMIT_RESPONSE | jq .
COMMIT_HASH=$(echo $COMMIT_RESPONSE | jq -r '.hash')

echo ""

# 7. Create parallel realities
echo "7️⃣  Creating parallel realities for A/B testing"
curl -s -X POST $BASE_URL/repos/$REPO_ID/parallel-realities \
  -H "Content-Type: application/json" \
  -d '{"branches": ["config-a", "config-b", "config-c"]}' | jq .

echo ""

# 8. Apply different configurations to each reality
echo "8️⃣  Applying different configurations to each reality"
curl -s -X POST $BASE_URL/repos/$REPO_ID/parallel-realities/config-a/apply \
  -H "Content-Type: application/json" \
  -d '{"changes": {"config.yaml": "version: 2.0\nname: demo\nmode: aggressive"}}' | jq .

curl -s -X POST $BASE_URL/repos/$REPO_ID/parallel-realities/config-b/apply \
  -H "Content-Type: application/json" \
  -d '{"changes": {"config.yaml": "version: 2.0\nname: demo\nmode: balanced"}}' | jq .

curl -s -X POST $BASE_URL/repos/$REPO_ID/parallel-realities/config-c/apply \
  -H "Content-Type: application/json" \
  -d '{"changes": {"config.yaml": "version: 2.0\nname: demo\nmode: conservative"}}' | jq .

echo ""

# 9. List branches
echo "9️⃣  Listing all branches"
curl -s $BASE_URL/repos/$REPO_ID/branches | jq .

echo ""

# 10. Get commit log
echo "🔟 Getting commit log"
curl -s $BASE_URL/repos/$REPO_ID/log | jq .

echo ""

# 11. Time travel
echo "1️⃣1️⃣ Time traveling to the first commit"
TIMESTAMP=$(date +%s)
sleep 1
curl -s $BASE_URL/repos/$REPO_ID/time-travel/$TIMESTAMP | jq .

echo ""

# 12. Clean up
echo "1️⃣2️⃣ Cleaning up - deleting repository"
curl -s -X DELETE $BASE_URL/repos/$REPO_ID | jq .

echo ""
echo "✅ Demo complete!"