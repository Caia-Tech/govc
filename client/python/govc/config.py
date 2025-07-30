"""Configuration for govc Python client."""

from typing import Optional, Dict, Any
import os


class Config:
    """Configuration for GovcClient."""
    
    def __init__(
        self,
        timeout: int = 30,
        max_retries: int = 3,
        verify_ssl: bool = True,
        proxy: Optional[str] = None,
        headers: Optional[Dict[str, str]] = None,
        user_agent: Optional[str] = None,
    ):
        """
        Initialize configuration.
        
        Args:
            timeout: Request timeout in seconds
            max_retries: Maximum number of retry attempts
            verify_ssl: Whether to verify SSL certificates
            proxy: HTTP proxy URL
            headers: Additional headers to include in requests
            user_agent: Custom user agent string
        """
        self.timeout = timeout
        self.max_retries = max_retries
        self.verify_ssl = verify_ssl
        self.proxy = proxy
        self.headers = headers or {}
        self.user_agent = user_agent or f"govc-python/{self._get_version()}"
    
    @classmethod
    def from_env(cls) -> "Config":
        """Create configuration from environment variables."""
        return cls(
            timeout=int(os.getenv("GOVC_TIMEOUT", "30")),
            max_retries=int(os.getenv("GOVC_MAX_RETRIES", "3")),
            verify_ssl=os.getenv("GOVC_VERIFY_SSL", "true").lower() == "true",
            proxy=os.getenv("GOVC_PROXY"),
        )
    
    def _get_version(self) -> str:
        """Get the client version."""
        try:
            from . import __version__
            return __version__
        except ImportError:
            return "unknown"
    
    def to_dict(self) -> Dict[str, Any]:
        """Convert config to dictionary."""
        return {
            "timeout": self.timeout,
            "max_retries": self.max_retries,
            "verify_ssl": self.verify_ssl,
            "proxy": self.proxy,
            "headers": self.headers,
            "user_agent": self.user_agent,
        }