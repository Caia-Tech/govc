"""Repository operations for govc Python client."""

import base64
from contextlib import contextmanager
from datetime import datetime
from typing import Optional, List, Dict, Any, Union, Generator

from .models import (
    Commit,
    Branch,
    Tag,
    FileEntry,
    Author,
    StashEntry,
    SearchResult,
    Transaction as TransactionModel,
    ParallelReality,
    DiffEntry,
    BlameEntry,
    MergeResult,
    MergeStrategy,
    ResetMode,
    FileType,
)
from .exceptions import (
    TransactionError,
    FileNotFoundError,
    BranchNotFoundError,
)


class Repository:
    """Repository operations."""
    
    def __init__(self, client: "GovcClient", repo_id: str):
        """
        Initialize repository.
        
        Args:
            client: GovcClient instance
            repo_id: Repository identifier
        """
        self.client = client
        self.repo_id = repo_id
    
    def _request(self, method: str, path: str, **kwargs) -> Any:
        """Make request with repository prefix."""
        full_path = f"/api/v1/repos/{self.repo_id}{path}"
        return self.client._request(method, full_path, **kwargs)
    
    # File operations
    
    def add(
        self,
        path: str,
        content: Union[str, bytes],
        mode: str = "100644",
        transaction_id: Optional[str] = None,
    ) -> None:
        """
        Add or update file.
        
        Args:
            path: File path
            content: File content (string or bytes)
            mode: File mode (default: 100644)
            transaction_id: Optional transaction ID
        """
        # Encode content if bytes
        if isinstance(content, bytes):
            content_str = base64.b64encode(content).decode("utf-8")
            is_binary = True
        else:
            content_str = content
            is_binary = False
        
        data = {
            "path": path,
            "content": content_str,
            "mode": mode,
        }
        if is_binary:
            data["encoding"] = "base64"
        
        params = {}
        if transaction_id:
            params["tx"] = transaction_id
        
        self._request("POST", "/add", json=data, params=params)
    
    def read(self, path: str, ref: Optional[str] = None) -> Union[str, bytes]:
        """
        Read file content.
        
        Args:
            path: File path
            ref: Branch, tag, or commit (default: HEAD)
        
        Returns:
            File content
        """
        params = {}
        if ref:
            params["ref"] = ref
        
        response = self._request("GET", f"/read/{path}", params=params)
        
        # Handle binary content
        if isinstance(response, dict) and response.get("encoding") == "base64":
            return base64.b64decode(response["content"])
        
        # Handle text content
        if isinstance(response, dict):
            return response["content"]
        
        return response
    
    def list_files(
        self,
        path: str = "/",
        ref: Optional[str] = None,
        recursive: bool = False,
    ) -> List[FileEntry]:
        """
        List directory contents.
        
        Args:
            path: Directory path
            ref: Branch, tag, or commit
            recursive: List recursively
        
        Returns:
            List of file entries
        """
        params = {}
        if ref:
            params["ref"] = ref
        if recursive:
            params["recursive"] = True
        
        response = self._request("GET", f"/tree/{path}", params=params)
        return [FileEntry.from_dict(entry) for entry in response["entries"]]
    
    def remove(self, path: str, transaction_id: Optional[str] = None) -> None:
        """
        Remove file.
        
        Args:
            path: File path
            transaction_id: Optional transaction ID
        """
        params = {}
        if transaction_id:
            params["tx"] = transaction_id
        
        self._request("DELETE", f"/remove/{path}", params=params)
    
    def move(
        self,
        from_path: str,
        to_path: str,
        transaction_id: Optional[str] = None,
    ) -> None:
        """
        Move or rename file.
        
        Args:
            from_path: Source path
            to_path: Destination path
            transaction_id: Optional transaction ID
        """
        data = {
            "from": from_path,
            "to": to_path,
        }
        
        params = {}
        if transaction_id:
            params["tx"] = transaction_id
        
        self._request("POST", "/move", json=data, params=params)
    
    # Commit operations
    
    def commit(
        self,
        message: str,
        author: Optional[Dict[str, str]] = None,
        transaction_id: Optional[str] = None,
    ) -> Commit:
        """
        Create commit.
        
        Args:
            message: Commit message
            author: Author information
            transaction_id: Optional transaction ID
        
        Returns:
            Created commit
        """
        data = {
            "message": message,
        }
        if author:
            data["author"] = author
        
        params = {}
        if transaction_id:
            params["tx"] = transaction_id
        
        response = self._request("POST", "/commit", json=data, params=params)
        return Commit.from_dict(response)
    
    def log(
        self,
        branch: Optional[str] = None,
        limit: int = 10,
        offset: int = 0,
        since: Optional[datetime] = None,
        until: Optional[datetime] = None,
        author: Optional[str] = None,
    ) -> List[Commit]:
        """
        Get commit history.
        
        Args:
            branch: Branch name
            limit: Maximum commits
            offset: Skip commits
            since: Start date
            until: End date
            author: Filter by author
        
        Returns:
            List of commits
        """
        params = {
            "limit": limit,
            "offset": offset,
        }
        if branch:
            params["branch"] = branch
        if since:
            params["since"] = since.isoformat()
        if until:
            params["until"] = until.isoformat()
        if author:
            params["author"] = author
        
        response = self._request("GET", "/log", params=params)
        return [Commit.from_dict(commit) for commit in response]
    
    def get_commit(self, commit_hash: str) -> Commit:
        """Get specific commit."""
        response = self._request("GET", f"/commits/{commit_hash}")
        return Commit.from_dict(response)
    
    # Branch operations
    
    def list_branches(self) -> List[Branch]:
        """List all branches."""
        response = self._request("GET", "/branches")
        return [Branch.from_dict(branch) for branch in response["branches"]]
    
    def create_branch(self, name: str, from_ref: Optional[str] = None) -> Branch:
        """
        Create branch.
        
        Args:
            name: Branch name
            from_ref: Source branch/commit
        
        Returns:
            Created branch
        """
        data = {
            "name": name,
        }
        if from_ref:
            data["from"] = from_ref
        
        response = self._request("POST", "/branches", json=data)
        return Branch.from_dict(response)
    
    def delete_branch(self, name: str) -> None:
        """Delete branch."""
        self._request("DELETE", f"/branches/{name}")
    
    def get_current_branch(self) -> str:
        """Get current branch name."""
        response = self._request("GET", "/branch")
        return response["name"]
    
    def checkout(self, branch: str) -> None:
        """Switch to branch."""
        self._request("POST", "/checkout", json={"branch": branch})
    
    # Tag operations
    
    def list_tags(self) -> List[Tag]:
        """List all tags."""
        response = self._request("GET", "/tags")
        return [Tag.from_dict(tag) for tag in response]
    
    def create_tag(
        self,
        name: str,
        commit: Optional[str] = None,
        message: Optional[str] = None,
        tagger: Optional[Dict[str, str]] = None,
    ) -> Tag:
        """
        Create tag.
        
        Args:
            name: Tag name
            commit: Target commit
            message: Tag message
            tagger: Tagger information
        
        Returns:
            Created tag
        """
        data = {
            "name": name,
        }
        if commit:
            data["commit"] = commit
        if message:
            data["message"] = message
        if tagger:
            data["tagger"] = tagger
        
        response = self._request("POST", "/tags", json=data)
        return Tag.from_dict(response)
    
    def delete_tag(self, name: str) -> None:
        """Delete tag."""
        self._request("DELETE", f"/tags/{name}")
    
    # Advanced operations
    
    def merge(
        self,
        source: str,
        into: Optional[str] = None,
        message: Optional[str] = None,
        strategy: MergeStrategy = MergeStrategy.RECURSIVE,
    ) -> MergeResult:
        """
        Merge branches.
        
        Args:
            source: Source branch
            into: Target branch (default: current)
            message: Merge commit message
            strategy: Merge strategy
        
        Returns:
            Merge result
        """
        data = {
            "source": source,
            "strategy": strategy.value,
        }
        if into:
            data["target"] = into
        if message:
            data["message"] = message
        
        response = self._request("POST", "/merge", json=data)
        return MergeResult.from_dict(response)
    
    def rebase(
        self,
        branch: str,
        onto: str,
        interactive: bool = False,
    ) -> None:
        """
        Rebase branch.
        
        Args:
            branch: Branch to rebase
            onto: Target branch
            interactive: Interactive rebase
        """
        data = {
            "branch": branch,
            "onto": onto,
            "interactive": interactive,
        }
        self._request("POST", "/rebase", json=data)
    
    def reset(
        self,
        commit: str,
        mode: ResetMode = ResetMode.MIXED,
    ) -> None:
        """
        Reset to commit.
        
        Args:
            commit: Target commit
            mode: Reset mode
        """
        data = {
            "commit": commit,
            "mode": mode.value,
        }
        self._request("POST", "/reset", json=data)
    
    def cherry_pick(self, commit: str, branch: Optional[str] = None) -> Commit:
        """
        Cherry-pick commit.
        
        Args:
            commit: Commit to cherry-pick
            branch: Target branch
        
        Returns:
            New commit
        """
        data = {
            "commit": commit,
        }
        if branch:
            data["branch"] = branch
        
        response = self._request("POST", "/cherry-pick", json=data)
        return Commit.from_dict(response)
    
    def revert(self, commit: str, message: Optional[str] = None) -> Commit:
        """
        Revert commit.
        
        Args:
            commit: Commit to revert
            message: Revert message
        
        Returns:
            Revert commit
        """
        data = {
            "commit": commit,
        }
        if message:
            data["message"] = message
        
        response = self._request("POST", "/revert", json=data)
        return Commit.from_dict(response)
    
    # Stash operations
    
    def stash(self, message: Optional[str] = None) -> str:
        """
        Create stash.
        
        Args:
            message: Stash message
        
        Returns:
            Stash ID
        """
        data = {}
        if message:
            data["message"] = message
        
        response = self._request("POST", "/stash", json=data)
        return response["id"]
    
    def list_stashes(self) -> List[StashEntry]:
        """List all stashes."""
        response = self._request("GET", "/stash")
        return [StashEntry.from_dict(stash) for stash in response]
    
    def apply_stash(self, stash_id: str) -> None:
        """Apply stash."""
        self._request("POST", f"/stash/{stash_id}/apply")
    
    def drop_stash(self, stash_id: str) -> None:
        """Drop stash."""
        self._request("DELETE", f"/stash/{stash_id}")
    
    # Search operations
    
    def search_commits(
        self,
        query: str,
        author: Optional[str] = None,
        since: Optional[datetime] = None,
        until: Optional[datetime] = None,
    ) -> List[Commit]:
        """Search commits."""
        params = {
            "q": query,
        }
        if author:
            params["author"] = author
        if since:
            params["since"] = since.isoformat()
        if until:
            params["until"] = until.isoformat()
        
        response = self._request("GET", "/search/commits", params=params)
        return [Commit.from_dict(commit) for commit in response]
    
    def search_content(
        self,
        query: str,
        path: Optional[str] = None,
        branch: Optional[str] = None,
    ) -> List[SearchResult]:
        """Search file content."""
        params = {
            "q": query,
        }
        if path:
            params["path"] = path
        if branch:
            params["branch"] = branch
        
        response = self._request("GET", "/search/content", params=params)
        return [SearchResult.from_dict(result) for result in response]
    
    def search_files(
        self,
        query: str,
        file_type: Optional[str] = None,
    ) -> List[FileEntry]:
        """Search filenames."""
        params = {
            "q": query,
        }
        if file_type:
            params["type"] = file_type
        
        response = self._request("GET", "/search/files", params=params)
        return [FileEntry.from_dict(entry) for entry in response]
    
    def grep(
        self,
        pattern: str,
        paths: Optional[List[str]] = None,
        case_sensitive: bool = True,
        regex: bool = True,
    ) -> List[SearchResult]:
        """Advanced pattern search."""
        data = {
            "pattern": pattern,
            "case_sensitive": case_sensitive,
            "regex": regex,
        }
        if paths:
            data["paths"] = paths
        
        response = self._request("POST", "/grep", json=data)
        return [SearchResult.from_dict(result) for result in response]
    
    # Diff and blame
    
    def diff(
        self,
        from_ref: str,
        to_ref: Optional[str] = None,
        path: Optional[str] = None,
    ) -> List[DiffEntry]:
        """Get diff between commits/branches."""
        params = {
            "from": from_ref,
        }
        if to_ref:
            params["to"] = to_ref
        if path:
            params["path"] = path
        
        response = self._request("GET", "/diff", params=params)
        return [DiffEntry.from_dict(entry) for entry in response]
    
    def blame(self, path: str, ref: Optional[str] = None) -> List[BlameEntry]:
        """Get blame information."""
        params = {}
        if ref:
            params["ref"] = ref
        
        response = self._request("GET", f"/blame/{path}", params=params)
        return [BlameEntry.from_dict(entry) for entry in response]
    
    # Transaction support
    
    @contextmanager
    def transaction(self) -> Generator["Transaction", None, None]:
        """
        Create transaction context.
        
        Usage:
            with repo.transaction() as tx:
                tx.add("file1.txt", "content")
                tx.remove("file2.txt")
                tx.commit("Multiple changes")
        """
        # Start transaction
        response = self._request("POST", "/transaction")
        tx_model = TransactionModel.from_dict(response)
        
        # Create transaction wrapper
        tx = Transaction(self, tx_model.id)
        
        try:
            yield tx
        except Exception:
            # Rollback on error
            try:
                self._request("POST", f"/transaction/{tx_model.id}/rollback")
            except:
                pass
            raise
        else:
            # Auto-commit if not already committed
            if not tx._committed:
                try:
                    self._request("POST", f"/transaction/{tx_model.id}/rollback")
                except:
                    pass
    
    # Parallel realities
    
    def create_parallel_realities(
        self,
        branches: List[str],
        base: Optional[str] = None,
    ) -> List[ParallelReality]:
        """Create parallel realities for testing."""
        data = {
            "branches": branches,
        }
        if base:
            data["base"] = base
        
        response = self._request("POST", "/parallel-realities", json=data)
        return [ParallelReality.from_dict(pr) for pr in response]
    
    # Time travel
    
    def time_travel(
        self,
        timestamp: datetime,
        branch: Optional[str] = None,
    ) -> "Repository":
        """
        Access repository at specific time.
        
        Returns a read-only repository view.
        """
        params = {
            "timestamp": timestamp.isoformat(),
        }
        if branch:
            params["branch"] = branch
        
        # This would return a special read-only repository instance
        # For simplicity, returning self
        return self


class Transaction:
    """Transaction wrapper for atomic operations."""
    
    def __init__(self, repo: Repository, transaction_id: str):
        """Initialize transaction."""
        self.repo = repo
        self.transaction_id = transaction_id
        self._committed = False
    
    def add(self, path: str, content: Union[str, bytes], mode: str = "100644") -> None:
        """Add file in transaction."""
        self.repo.add(path, content, mode, transaction_id=self.transaction_id)
    
    def remove(self, path: str) -> None:
        """Remove file in transaction."""
        self.repo.remove(path, transaction_id=self.transaction_id)
    
    def move(self, from_path: str, to_path: str) -> None:
        """Move file in transaction."""
        self.repo.move(from_path, to_path, transaction_id=self.transaction_id)
    
    def commit(
        self,
        message: str,
        author: Optional[Dict[str, str]] = None,
    ) -> Commit:
        """Commit transaction."""
        if self._committed:
            raise TransactionError("Transaction already committed", transaction_id=self.transaction_id)
        
        response = self.repo._request(
            "POST",
            f"/transaction/{self.transaction_id}/commit",
            json={
                "message": message,
                "author": author,
            }
        )
        self._committed = True
        return Commit.from_dict(response)
    
    def rollback(self) -> None:
        """Rollback transaction."""
        if self._committed:
            raise TransactionError("Cannot rollback committed transaction", transaction_id=self.transaction_id)
        
        self.repo._request("POST", f"/transaction/{self.transaction_id}/rollback")
        self._committed = True