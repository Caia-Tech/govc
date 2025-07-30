#!/usr/bin/env python3
"""Quick start example for govc Python client."""

from govc import GovcClient, GovcError
import os


def main():
    # Initialize client
    # You can use environment variables:
    # export GOVC_SERVER_URL=http://localhost:8080
    # export GOVC_API_KEY=your-api-key
    
    # Or initialize directly:
    client = GovcClient(
        base_url=os.getenv("GOVC_SERVER_URL", "http://localhost:8080"),
        api_key=os.getenv("GOVC_API_KEY")
    )
    
    try:
        # Create a repository
        print("Creating repository...")
        repo = client.create_repo("python-example", memory_only=True)
        print(f"✓ Created repository: python-example")
        
        # Add some files
        print("\nAdding files...")
        repo.add("README.md", """# Python Example

This is an example repository created with the govc Python client.

## Features
- Memory-first Git implementation
- Lightning fast operations
- Full Git compatibility
""")
        
        repo.add("main.py", """#!/usr/bin/env python3

def main():
    print("Hello from govc!")

if __name__ == "__main__":
    main()
""")
        
        repo.add(".gitignore", """__pycache__/
*.pyc
.env
venv/
""")
        print("✓ Added 3 files")
        
        # Commit changes
        print("\nCommitting changes...")
        commit = repo.commit(
            "Initial commit",
            author={"name": "Python Example", "email": "example@govc.io"}
        )
        print(f"✓ Created commit: {commit.hash[:7]} - {commit.message}")
        
        # Create a branch
        print("\nCreating feature branch...")
        feature_branch = repo.create_branch("feature/add-tests")
        repo.checkout("feature/add-tests")
        print(f"✓ Created and switched to branch: {feature_branch.name}")
        
        # Add test file
        repo.add("test_main.py", """import unittest
from main import main

class TestMain(unittest.TestCase):
    def test_main(self):
        # Test would go here
        pass

if __name__ == "__main__":
    unittest.main()
""")
        
        test_commit = repo.commit("Add unit tests")
        print(f"✓ Added tests: {test_commit.hash[:7]}")
        
        # List files
        print("\nRepository contents:")
        files = repo.list_files("/")
        for file in files:
            print(f"  {file.name:<20} {file.size:>10} bytes")
        
        # Show commit history
        print("\nCommit history:")
        commits = repo.log(limit=5)
        for commit in commits:
            print(f"  {commit.hash[:7]} - {commit.message}")
        
        # Search for content
        print("\nSearching for 'govc'...")
        results = repo.search_content("govc")
        for result in results:
            print(f"  {result.file}:{result.line} - {result.content.strip()}")
        
        # Clean up
        print("\nCleaning up...")
        client.delete_repo("python-example")
        print("✓ Repository deleted")
        
    except GovcError as e:
        print(f"Error: {e}")
        return 1
    
    return 0


if __name__ == "__main__":
    exit(main())