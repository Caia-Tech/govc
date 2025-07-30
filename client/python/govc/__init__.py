"""
govc Python Client Library

A Python client for govc - memory-first Git implementation.
"""

__version__ = "1.0.0"
__author__ = "Caia Tech"
__email__ = "owner@caiatech.com"

from .client import GovcClient
from .repository import Repository
from .models import (
    Commit,
    Branch,
    Tag,
    FileEntry,
    Author,
    StashEntry,
    SearchResult,
    Transaction,
    ParallelReality,
)
from .exceptions import (
    GovcError,
    AuthenticationError,
    RepositoryNotFoundError,
    BranchNotFoundError,
    FileNotFoundError,
    ConflictError,
    ValidationError,
    TimeoutError,
)
from .config import Config

# Async support (optional)
try:
    from .async_client import AsyncGovcClient
    from .async_repository import AsyncRepository
    __all__ = [
        "GovcClient",
        "AsyncGovcClient",
        "Repository",
        "AsyncRepository",
        "Config",
        "Commit",
        "Branch",
        "Tag",
        "FileEntry",
        "Author",
        "StashEntry",
        "SearchResult",
        "Transaction",
        "ParallelReality",
        "GovcError",
        "AuthenticationError",
        "RepositoryNotFoundError",
        "BranchNotFoundError",
        "FileNotFoundError",
        "ConflictError",
        "ValidationError",
        "TimeoutError",
    ]
except ImportError:
    # Async dependencies not installed
    __all__ = [
        "GovcClient",
        "Repository",
        "Config",
        "Commit",
        "Branch",
        "Tag",
        "FileEntry",
        "Author",
        "StashEntry",
        "SearchResult",
        "Transaction",
        "ParallelReality",
        "GovcError",
        "AuthenticationError",
        "RepositoryNotFoundError",
        "BranchNotFoundError",
        "FileNotFoundError",
        "ConflictError",
        "ValidationError",
        "TimeoutError",
    ]