"""Exception classes for govc Python client."""

from typing import Optional, Dict, Any


class GovcError(Exception):
    """Base exception for all govc errors."""
    
    def __init__(
        self,
        message: str,
        code: Optional[str] = None,
        details: Optional[Dict[str, Any]] = None,
        status_code: Optional[int] = None
    ):
        super().__init__(message)
        self.message = message
        self.code = code
        self.details = details or {}
        self.status_code = status_code
    
    def __str__(self) -> str:
        if self.code:
            return f"[{self.code}] {self.message}"
        return self.message


class AuthenticationError(GovcError):
    """Raised when authentication fails."""
    
    def __init__(self, message: str = "Authentication failed", **kwargs):
        super().__init__(message, code="UNAUTHORIZED", status_code=401, **kwargs)


class PermissionError(GovcError):
    """Raised when user lacks required permissions."""
    
    def __init__(self, message: str = "Permission denied", **kwargs):
        super().__init__(message, code="FORBIDDEN", status_code=403, **kwargs)


class NotFoundError(GovcError):
    """Base class for not found errors."""
    
    def __init__(self, message: str, code: str = "NOT_FOUND", **kwargs):
        super().__init__(message, code=code, status_code=404, **kwargs)


class RepositoryNotFoundError(NotFoundError):
    """Raised when repository is not found."""
    
    def __init__(self, repo_id: str, **kwargs):
        super().__init__(
            f"Repository '{repo_id}' not found",
            code="REPO_NOT_FOUND",
            details={"repository": repo_id},
            **kwargs
        )


class BranchNotFoundError(NotFoundError):
    """Raised when branch is not found."""
    
    def __init__(self, branch: str, repo_id: Optional[str] = None, **kwargs):
        details = {"branch": branch}
        if repo_id:
            details["repository"] = repo_id
        super().__init__(
            f"Branch '{branch}' not found",
            code="BRANCH_NOT_FOUND",
            details=details,
            **kwargs
        )


class FileNotFoundError(NotFoundError):
    """Raised when file is not found."""
    
    def __init__(self, path: str, repo_id: Optional[str] = None, **kwargs):
        details = {"path": path}
        if repo_id:
            details["repository"] = repo_id
        super().__init__(
            f"File '{path}' not found",
            code="FILE_NOT_FOUND",
            details=details,
            **kwargs
        )


class CommitNotFoundError(NotFoundError):
    """Raised when commit is not found."""
    
    def __init__(self, commit: str, repo_id: Optional[str] = None, **kwargs):
        details = {"commit": commit}
        if repo_id:
            details["repository"] = repo_id
        super().__init__(
            f"Commit '{commit}' not found",
            code="COMMIT_NOT_FOUND",
            details=details,
            **kwargs
        )


class ConflictError(GovcError):
    """Raised when operation conflicts with current state."""
    
    def __init__(self, message: str, **kwargs):
        super().__init__(message, code="CONFLICT", status_code=409, **kwargs)


class ValidationError(GovcError):
    """Raised when input validation fails."""
    
    def __init__(self, message: str, field: Optional[str] = None, **kwargs):
        details = kwargs.get("details", {})
        if field:
            details["field"] = field
        super().__init__(
            message,
            code="VALIDATION_ERROR",
            status_code=400,
            details=details,
            **kwargs
        )


class TimeoutError(GovcError):
    """Raised when operation times out."""
    
    def __init__(self, message: str = "Operation timed out", **kwargs):
        super().__init__(message, code="TIMEOUT", status_code=408, **kwargs)


class RateLimitError(GovcError):
    """Raised when rate limit is exceeded."""
    
    def __init__(
        self,
        message: str = "Rate limit exceeded",
        retry_after: Optional[int] = None,
        **kwargs
    ):
        details = kwargs.get("details", {})
        if retry_after:
            details["retry_after"] = retry_after
        super().__init__(
            message,
            code="RATE_LIMIT",
            status_code=429,
            details=details,
            **kwargs
        )


class ServerError(GovcError):
    """Raised for server-side errors."""
    
    def __init__(self, message: str = "Internal server error", **kwargs):
        super().__init__(message, code="INTERNAL_ERROR", status_code=500, **kwargs)


class ConnectionError(GovcError):
    """Raised when connection to server fails."""
    
    def __init__(self, message: str = "Connection failed", **kwargs):
        super().__init__(message, code="CONNECTION_ERROR", **kwargs)


class TransactionError(GovcError):
    """Raised for transaction-related errors."""
    
    def __init__(self, message: str, transaction_id: Optional[str] = None, **kwargs):
        details = kwargs.get("details", {})
        if transaction_id:
            details["transaction_id"] = transaction_id
        super().__init__(
            message,
            code="TRANSACTION_ERROR",
            details=details,
            **kwargs
        )