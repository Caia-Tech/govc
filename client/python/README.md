# govc Python Client

Official Python client library for govc - a memory-first Git implementation.

## Installation

```bash
pip install govc-client
```

For async support:
```bash
pip install govc-client[async]
```

## Quick Start

```python
from govc import GovcClient

# Initialize client
client = GovcClient("https://govc.example.com", token="your-jwt-token")

# Or use API key
client = GovcClient("https://govc.example.com", api_key="gk_live_...")

# Create a repository
repo = client.create_repo("my-project", memory_only=True)

# Work with files
repo.add("README.md", "# My Project\n\nWelcome to my project!")
repo.add("src/main.py", "print('Hello, World!')")

# Commit changes
commit = repo.commit("Initial commit", author={"name": "Alice", "email": "alice@example.com"})
print(f"Created commit: {commit.hash}")

# Create branches
feature_branch = repo.create_branch("feature/new-feature")
repo.checkout("feature/new-feature")

# List files
files = repo.list_files("/")
for file in files:
    print(f"{file.path} - {file.size} bytes")
```

## Authentication

### JWT Token

```python
from govc import GovcClient

# Login to get token
auth_response = GovcClient.login(
    "https://govc.example.com",
    username="alice",
    password="secret"
)

# Use token
client = GovcClient("https://govc.example.com", token=auth_response.token)
```

### API Key

```python
client = GovcClient("https://govc.example.com", api_key="gk_live_...")
```

### Environment Variables

```python
# Set environment variables
# GOVC_SERVER_URL=https://govc.example.com
# GOVC_API_KEY=gk_live_...

client = GovcClient.from_env()
```

## Repository Operations

### Create and List Repositories

```python
# Create repository
repo = client.create_repo(
    "my-project",
    memory_only=False,
    description="My awesome project"
)

# List all repositories
repos = client.list_repos()
for repo in repos:
    print(f"{repo.id} - {repo.description}")

# Get specific repository
repo = client.get_repo("my-project")

# Delete repository
client.delete_repo("old-project")
```

### File Operations

```python
# Add files
repo.add("file.txt", "Content here")
repo.add("binary.jpg", b"\\x89PNG...", mode="100755")

# Read files
content = repo.read("file.txt")
print(content)

# List directory
entries = repo.list_files("/src")
for entry in entries:
    if entry.is_file:
        print(f"File: {entry.name} ({entry.size} bytes)")
    else:
        print(f"Dir: {entry.name}/")

# Remove files
repo.remove("old-file.txt")

# Move/rename files
repo.move("old-name.txt", "new-name.txt")
```

### Commits

```python
# Create commit
commit = repo.commit(
    "Add new feature",
    author={"name": "Alice", "email": "alice@example.com"}
)

# Get commit history
commits = repo.log(limit=10)
for commit in commits:
    print(f"{commit.hash[:7]} - {commit.message}")

# Get specific commit
commit = repo.get_commit("abc123...")
print(f"Author: {commit.author.name} <{commit.author.email}>")
print(f"Date: {commit.author.time}")
print(f"Message: {commit.message}")
```

### Branches

```python
# List branches
branches = repo.list_branches()
for branch in branches:
    print(f"{branch.name} -> {branch.commit[:7]}")

# Create branch
repo.create_branch("feature/awesome", from_ref="main")

# Switch branch
repo.checkout("feature/awesome")

# Get current branch
current = repo.get_current_branch()
print(f"On branch: {current}")

# Delete branch
repo.delete_branch("old-feature")

# Merge branches
merge_result = repo.merge("feature/awesome", into="main")
print(f"Merge commit: {merge_result.commit}")
```

### Tags

```python
# Create tag
tag = repo.create_tag(
    "v1.0.0",
    commit="abc123...",
    message="Release version 1.0.0",
    tagger={"name": "Alice", "email": "alice@example.com"}
)

# List tags
tags = repo.list_tags()
for tag in tags:
    print(f"{tag.name} -> {tag.commit[:7]}")

# Delete tag
repo.delete_tag("old-version")
```

## Advanced Features

### Transactions

```python
# Start transaction for atomic operations
with repo.transaction() as tx:
    tx.add("file1.txt", "Content 1")
    tx.add("file2.txt", "Content 2")
    tx.remove("old-file.txt")
    tx.move("rename-me.txt", "renamed.txt")
    
    # All changes committed atomically
    tx.commit("Multiple changes in one commit")
```

### Parallel Realities

```python
# Create parallel realities for testing
realities = repo.create_parallel_realities(
    ["test-1", "test-2", "test-3"],
    base="main"
)

# Work in different realities
for reality in realities:
    reality.add(f"test-{reality.name}.txt", f"Testing in {reality.name}")
    reality.commit(f"Test commit in {reality.name}")
```

### Time Travel

```python
# Access repository at specific time
from datetime import datetime, timedelta

yesterday = datetime.now() - timedelta(days=1)
past_state = repo.time_travel(yesterday)

# Read file from the past
old_content = past_state.read("README.md")
```

### Stash

```python
# Save work in progress
stash_id = repo.stash("WIP: Working on feature")

# List stashes
stashes = repo.list_stashes()
for stash in stashes:
    print(f"{stash.id} - {stash.message}")

# Apply stash
repo.apply_stash(stash_id)

# Drop stash
repo.drop_stash(stash_id)
```

### Search

```python
# Search commits
commits = repo.search_commits(
    query="fix bug",
    author="alice@example.com",
    since="2024-01-01"
)

# Search file content
results = repo.search_content(
    query="TODO",
    path="src/**/*.py"
)

for result in results:
    print(f"{result.file}:{result.line} - {result.content}")

# Search filenames
files = repo.search_files("*.test.py")
```

## Async Support

```python
import asyncio
from govc import AsyncGovcClient

async def main():
    # Initialize async client
    client = AsyncGovcClient("https://govc.example.com", api_key="...")
    
    # Create repository
    repo = await client.create_repo("async-project")
    
    # Parallel operations
    await asyncio.gather(
        repo.add("file1.txt", "Content 1"),
        repo.add("file2.txt", "Content 2"),
        repo.add("file3.txt", "Content 3")
    )
    
    # Commit
    commit = await repo.commit("Add multiple files")
    print(f"Commit: {commit.hash}")
    
    # Clean up
    await client.close()

asyncio.run(main())
```

## Error Handling

```python
from govc import GovcClient, GovcError, RepositoryNotFoundError, AuthenticationError

try:
    client = GovcClient("https://govc.example.com", token="invalid")
    repo = client.get_repo("my-project")
except AuthenticationError:
    print("Invalid credentials")
except RepositoryNotFoundError:
    print("Repository not found")
except GovcError as e:
    print(f"API error: {e.code} - {e.message}")
```

## Configuration

```python
from govc import GovcClient, Config

# Configure client
config = Config(
    timeout=30,  # Request timeout in seconds
    max_retries=3,
    verify_ssl=True,
    proxy="http://proxy.example.com:8080"
)

client = GovcClient(
    "https://govc.example.com",
    api_key="...",
    config=config
)
```

## Logging

```python
import logging

# Enable debug logging
logging.basicConfig(level=logging.DEBUG)

# govc client uses the 'govc' logger
logger = logging.getLogger('govc')
logger.setLevel(logging.DEBUG)
```

## Testing

```python
import pytest
from govc import GovcClient
from govc.testing import MockGovcServer

def test_repository_operations():
    # Use mock server for testing
    with MockGovcServer() as server:
        client = GovcClient(server.url)
        
        # Test operations
        repo = client.create_repo("test-repo")
        repo.add("test.txt", "Test content")
        commit = repo.commit("Test commit")
        
        assert commit.message == "Test commit"
```

## Examples

### Complete Example: Blog System

```python
from govc import GovcClient
from datetime import datetime

# Initialize client
client = GovcClient.from_env()

# Create blog repository
blog = client.create_repo("my-blog", description="Personal blog")

# Create blog structure
blog.add("index.md", "# My Blog\n\nWelcome to my blog!")
blog.add("posts/.gitkeep", "")
blog.add("assets/.gitkeep", "")
blog.add("config.yml", """
title: My Blog
author: Alice Smith
theme: minimal
""")

# Commit initial structure
blog.commit("Initial blog setup", author={
    "name": "Alice Smith",
    "email": "alice@example.com"
})

# Add first post
post_date = datetime.now().strftime("%Y-%m-%d")
blog.add(f"posts/{post_date}-hello-world.md", """
# Hello, World!

This is my first blog post using govc!

## Features

- Memory-first Git implementation
- Lightning fast operations
- Built-in Python client

Happy blogging!
""")

blog.commit("Add first blog post")

# Create feature branch for new post
blog.create_branch("post/python-tips")
blog.checkout("post/python-tips")

# Work on new post
blog.add("posts/2024-01-16-python-tips.md", """
# Python Tips and Tricks

Here are some useful Python tips...
""")

# Save work in progress
stash_id = blog.stash("WIP: Python tips post")

# Switch back to main
blog.checkout("main")

# Later, resume work
blog.checkout("post/python-tips")
blog.apply_stash(stash_id)

# Finish and commit
blog.commit("Add Python tips post")

# Merge to main
blog.checkout("main")
blog.merge("post/python-tips")

# Tag release
blog.create_tag("v1.0.0", message="First release with two posts")

print("Blog setup complete!")
```

## API Reference

See the [full API documentation](https://docs.govc.io/python-client) for detailed information about all available methods and classes.

## Contributing

Contributions are welcome! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.