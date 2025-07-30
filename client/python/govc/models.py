"""Data models for govc Python client."""

from dataclasses import dataclass, field
from datetime import datetime
from typing import Optional, List, Dict, Any, Union
from enum import Enum


class FileType(Enum):
    """File type enumeration."""
    FILE = "file"
    DIRECTORY = "directory"
    SYMLINK = "symlink"


class MergeStrategy(Enum):
    """Merge strategy enumeration."""
    RECURSIVE = "recursive"
    OURS = "ours"
    THEIRS = "theirs"
    OCTOPUS = "octopus"


class ResetMode(Enum):
    """Reset mode enumeration."""
    SOFT = "soft"
    MIXED = "mixed"
    HARD = "hard"


@dataclass
class Author:
    """Git author information."""
    name: str
    email: str
    time: Optional[datetime] = None
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Author":
        """Create Author from dictionary."""
        return cls(
            name=data["name"],
            email=data["email"],
            time=datetime.fromisoformat(data["time"].replace("Z", "+00:00"))
            if "time" in data else None
        )
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary."""
        result = {
            "name": self.name,
            "email": self.email,
        }
        if self.time:
            result["time"] = self.time.isoformat()
        return result


@dataclass
class Commit:
    """Git commit information."""
    hash: str
    message: str
    author: Author
    committer: Optional[Author] = None
    parents: List[str] = field(default_factory=list)
    tree: Optional[str] = None
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Commit":
        """Create Commit from dictionary."""
        return cls(
            hash=data["hash"],
            message=data["message"],
            author=Author.from_dict(data["author"]),
            committer=Author.from_dict(data["committer"])
            if "committer" in data else None,
            parents=data.get("parents", []),
            tree=data.get("tree"),
        )


@dataclass
class Branch:
    """Git branch information."""
    name: str
    commit: str
    protected: bool = False
    is_default: bool = False
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Branch":
        """Create Branch from dictionary."""
        return cls(
            name=data["name"],
            commit=data["commit"],
            protected=data.get("protected", False),
            is_default=data.get("is_default", False),
        )


@dataclass
class Tag:
    """Git tag information."""
    name: str
    commit: str
    message: Optional[str] = None
    tagger: Optional[Author] = None
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Tag":
        """Create Tag from dictionary."""
        return cls(
            name=data["name"],
            commit=data["commit"],
            message=data.get("message"),
            tagger=Author.from_dict(data["tagger"])
            if "tagger" in data else None,
        )


@dataclass
class FileEntry:
    """File entry in repository."""
    name: str
    path: str
    type: FileType
    mode: str = "100644"
    size: Optional[int] = None
    hash: Optional[str] = None
    
    @property
    def is_file(self) -> bool:
        """Check if entry is a file."""
        return self.type == FileType.FILE
    
    @property
    def is_directory(self) -> bool:
        """Check if entry is a directory."""
        return self.type == FileType.DIRECTORY
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "FileEntry":
        """Create FileEntry from dictionary."""
        return cls(
            name=data["name"],
            path=data.get("path", data["name"]),
            type=FileType(data["type"]),
            mode=data.get("mode", "100644"),
            size=data.get("size"),
            hash=data.get("hash"),
        )


@dataclass
class StashEntry:
    """Stash entry information."""
    id: str
    message: str
    branch: str
    created_at: datetime
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "StashEntry":
        """Create StashEntry from dictionary."""
        return cls(
            id=data["id"],
            message=data["message"],
            branch=data["branch"],
            created_at=datetime.fromisoformat(data["created_at"].replace("Z", "+00:00")),
        )


@dataclass
class SearchResult:
    """Search result information."""
    file: str
    line: int
    content: str
    match: str
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "SearchResult":
        """Create SearchResult from dictionary."""
        return cls(
            file=data["file"],
            line=data["line"],
            content=data["content"],
            match=data["match"],
        )


@dataclass
class Repository:
    """Repository information."""
    id: str
    memory_only: bool = False
    description: Optional[str] = None
    created_at: Optional[datetime] = None
    updated_at: Optional[datetime] = None
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Repository":
        """Create Repository from dictionary."""
        return cls(
            id=data["id"],
            memory_only=data.get("memory_only", False),
            description=data.get("description"),
            created_at=datetime.fromisoformat(data["created_at"].replace("Z", "+00:00"))
            if "created_at" in data else None,
            updated_at=datetime.fromisoformat(data["updated_at"].replace("Z", "+00:00"))
            if "updated_at" in data else None,
        )


@dataclass
class Transaction:
    """Transaction information."""
    id: str
    created_at: datetime
    expires_at: datetime
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "Transaction":
        """Create Transaction from dictionary."""
        return cls(
            id=data["id"],
            created_at=datetime.fromisoformat(data["created_at"].replace("Z", "+00:00")),
            expires_at=datetime.fromisoformat(data["expires_at"].replace("Z", "+00:00")),
        )


@dataclass
class ParallelReality:
    """Parallel reality information."""
    id: str
    name: str
    base_branch: str
    created_at: datetime
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "ParallelReality":
        """Create ParallelReality from dictionary."""
        return cls(
            id=data["id"],
            name=data["name"],
            base_branch=data["base_branch"],
            created_at=datetime.fromisoformat(data["created_at"].replace("Z", "+00:00")),
        )


@dataclass
class DiffEntry:
    """Diff entry information."""
    path: str
    old_path: Optional[str] = None
    old_mode: Optional[str] = None
    new_mode: Optional[str] = None
    added: int = 0
    deleted: int = 0
    binary: bool = False
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "DiffEntry":
        """Create DiffEntry from dictionary."""
        return cls(
            path=data["path"],
            old_path=data.get("old_path"),
            old_mode=data.get("old_mode"),
            new_mode=data.get("new_mode"),
            added=data.get("added", 0),
            deleted=data.get("deleted", 0),
            binary=data.get("binary", False),
        )


@dataclass
class BlameEntry:
    """Blame entry information."""
    line: int
    content: str
    commit: str
    author: Author
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "BlameEntry":
        """Create BlameEntry from dictionary."""
        return cls(
            line=data["line"],
            content=data["content"],
            commit=data["commit"],
            author=Author.from_dict(data["author"]),
        )


@dataclass
class MergeResult:
    """Merge result information."""
    commit: str
    success: bool
    conflicts: List[str] = field(default_factory=list)
    message: Optional[str] = None
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "MergeResult":
        """Create MergeResult from dictionary."""
        return cls(
            commit=data["commit"],
            success=data["success"],
            conflicts=data.get("conflicts", []),
            message=data.get("message"),
        )


@dataclass
class HealthStatus:
    """Health check status."""
    status: str
    version: str
    uptime: int
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "HealthStatus":
        """Create HealthStatus from dictionary."""
        return cls(
            status=data["status"],
            version=data["version"],
            uptime=data["uptime"],
        )


@dataclass
class User:
    """User information."""
    id: str
    username: str
    email: str
    is_active: bool = True
    is_admin: bool = False
    roles: List[str] = field(default_factory=list)
    created_at: Optional[datetime] = None
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "User":
        """Create User from dictionary."""
        return cls(
            id=data["id"],
            username=data["username"],
            email=data["email"],
            is_active=data.get("is_active", True),
            is_admin=data.get("is_admin", False),
            roles=data.get("roles", []),
            created_at=datetime.fromisoformat(data["created_at"].replace("Z", "+00:00"))
            if "created_at" in data else None,
        )


@dataclass
class AuthResponse:
    """Authentication response."""
    token: str
    expires_at: datetime
    user: User
    
    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "AuthResponse":
        """Create AuthResponse from dictionary."""
        return cls(
            token=data["token"],
            expires_at=datetime.fromisoformat(data["expires_at"].replace("Z", "+00:00")),
            user=User.from_dict(data["user"]),
        )