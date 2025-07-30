"""Main client for govc API."""

import os
from typing import Optional, List, Dict, Any, Union
from urllib.parse import urljoin
import requests
from requests.adapters import HTTPAdapter
from requests.packages.urllib3.util.retry import Retry

from .config import Config
from .exceptions import (
    GovcError,
    AuthenticationError,
    RepositoryNotFoundError,
    ServerError,
    ValidationError,
    RateLimitError,
    TimeoutError,
    ConnectionError as GovcConnectionError,
)
from .models import (
    Repository as RepositoryModel,
    AuthResponse,
    User,
    HealthStatus,
)
from .repository import Repository


class GovcClient:
    """Client for interacting with govc server."""
    
    def __init__(
        self,
        base_url: str,
        token: Optional[str] = None,
        api_key: Optional[str] = None,
        config: Optional[Config] = None,
    ):
        """
        Initialize govc client.
        
        Args:
            base_url: Base URL of govc server
            token: JWT token for authentication
            api_key: API key for authentication
            config: Client configuration
        """
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.api_key = api_key
        self.config = config or Config()
        
        # Setup session with retry logic
        self.session = requests.Session()
        retry_strategy = Retry(
            total=self.config.max_retries,
            backoff_factor=1,
            status_forcelist=[429, 500, 502, 503, 504],
        )
        adapter = HTTPAdapter(max_retries=retry_strategy)
        self.session.mount("http://", adapter)
        self.session.mount("https://", adapter)
        
        # Set default headers
        self.session.headers.update({
            "User-Agent": self.config.user_agent,
            "Accept": "application/json",
            "Content-Type": "application/json",
        })
        
        # Add custom headers
        if self.config.headers:
            self.session.headers.update(self.config.headers)
        
        # Set authentication
        if self.token:
            self.session.headers["Authorization"] = f"Bearer {self.token}"
        elif self.api_key:
            self.session.headers["X-API-Key"] = self.api_key
        
        # Set proxy if configured
        if self.config.proxy:
            self.session.proxies = {
                "http": self.config.proxy,
                "https": self.config.proxy,
            }
    
    @classmethod
    def from_env(cls, config: Optional[Config] = None) -> "GovcClient":
        """
        Create client from environment variables.
        
        Environment variables:
            GOVC_SERVER_URL: Base URL of govc server
            GOVC_TOKEN: JWT token
            GOVC_API_KEY: API key
        """
        base_url = os.getenv("GOVC_SERVER_URL")
        if not base_url:
            raise ValueError("GOVC_SERVER_URL environment variable not set")
        
        return cls(
            base_url=base_url,
            token=os.getenv("GOVC_TOKEN"),
            api_key=os.getenv("GOVC_API_KEY"),
            config=config or Config.from_env(),
        )
    
    @classmethod
    def login(
        cls,
        base_url: str,
        username: str,
        password: str,
        config: Optional[Config] = None,
    ) -> AuthResponse:
        """
        Login to govc server.
        
        Args:
            base_url: Base URL of govc server
            username: Username
            password: Password
            config: Client configuration
        
        Returns:
            Authentication response with token
        """
        temp_client = cls(base_url, config=config)
        response = temp_client._request(
            "POST",
            "/api/v1/auth/login",
            json={"username": username, "password": password},
        )
        return AuthResponse.from_dict(response)
    
    def _request(
        self,
        method: str,
        path: str,
        params: Optional[Dict[str, Any]] = None,
        json: Optional[Dict[str, Any]] = None,
        data: Optional[Union[str, bytes]] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> Any:
        """Make HTTP request to govc server."""
        url = urljoin(self.base_url, path)
        
        # Prepare headers
        req_headers = {}
        if headers:
            req_headers.update(headers)
        
        try:
            response = self.session.request(
                method=method,
                url=url,
                params=params,
                json=json,
                data=data,
                headers=req_headers,
                timeout=self.config.timeout,
                verify=self.config.verify_ssl,
            )
        except requests.exceptions.Timeout:
            raise TimeoutError(f"Request to {url} timed out")
        except requests.exceptions.ConnectionError as e:
            raise GovcConnectionError(f"Failed to connect to {url}: {str(e)}")
        except Exception as e:
            raise GovcError(f"Request failed: {str(e)}")
        
        # Handle rate limiting
        if response.status_code == 429:
            retry_after = response.headers.get("Retry-After")
            raise RateLimitError(retry_after=int(retry_after) if retry_after else None)
        
        # Handle authentication errors
        if response.status_code == 401:
            raise AuthenticationError()
        
        # Handle not found errors
        if response.status_code == 404:
            error_data = self._parse_error(response)
            if error_data.get("code") == "REPO_NOT_FOUND":
                raise RepositoryNotFoundError(error_data.get("details", {}).get("repository", "unknown"))
            raise GovcError(error_data.get("message", "Not found"), code="NOT_FOUND")
        
        # Handle validation errors
        if response.status_code == 400:
            error_data = self._parse_error(response)
            raise ValidationError(
                error_data.get("message", "Validation failed"),
                details=error_data.get("details"),
            )
        
        # Handle server errors
        if response.status_code >= 500:
            error_data = self._parse_error(response)
            raise ServerError(error_data.get("message", "Internal server error"))
        
        # Handle other errors
        if response.status_code >= 400:
            error_data = self._parse_error(response)
            raise GovcError(
                error_data.get("message", f"Request failed with status {response.status_code}"),
                code=error_data.get("code"),
                details=error_data.get("details"),
                status_code=response.status_code,
            )
        
        # Parse JSON response
        if response.headers.get("Content-Type", "").startswith("application/json"):
            return response.json()
        
        # Return text for non-JSON responses
        return response.text
    
    def _parse_error(self, response: requests.Response) -> Dict[str, Any]:
        """Parse error response."""
        try:
            data = response.json()
            if "error" in data:
                return data["error"]
            return data
        except:
            return {"message": response.text}
    
    # Repository operations
    
    def create_repo(
        self,
        repo_id: str,
        memory_only: bool = False,
        description: Optional[str] = None,
    ) -> Repository:
        """
        Create a new repository.
        
        Args:
            repo_id: Repository identifier
            memory_only: Whether to create memory-only repository
            description: Repository description
        
        Returns:
            Repository instance
        """
        data = {
            "id": repo_id,
            "memory_only": memory_only,
        }
        if description:
            data["description"] = description
        
        self._request("POST", "/api/v1/repos", json=data)
        return Repository(self, repo_id)
    
    def get_repo(self, repo_id: str) -> Repository:
        """
        Get existing repository.
        
        Args:
            repo_id: Repository identifier
        
        Returns:
            Repository instance
        """
        self._request("GET", f"/api/v1/repos/{repo_id}")
        return Repository(self, repo_id)
    
    def list_repos(
        self,
        memory_only: Optional[bool] = None,
        limit: int = 100,
        offset: int = 0,
    ) -> List[RepositoryModel]:
        """
        List repositories.
        
        Args:
            memory_only: Filter by storage type
            limit: Maximum results
            offset: Pagination offset
        
        Returns:
            List of repository models
        """
        params = {
            "limit": limit,
            "offset": offset,
        }
        if memory_only is not None:
            params["memory_only"] = memory_only
        
        response = self._request("GET", "/api/v1/repos", params=params)
        return [RepositoryModel.from_dict(repo) for repo in response]
    
    def delete_repo(self, repo_id: str) -> None:
        """
        Delete repository.
        
        Args:
            repo_id: Repository identifier
        """
        self._request("DELETE", f"/api/v1/repos/{repo_id}")
    
    # User operations
    
    def get_current_user(self) -> User:
        """Get current authenticated user."""
        response = self._request("GET", "/api/v1/auth/me")
        return User.from_dict(response)
    
    def list_users(self) -> List[User]:
        """List all users (admin only)."""
        response = self._request("GET", "/api/v1/users")
        return [User.from_dict(user) for user in response]
    
    def create_user(
        self,
        username: str,
        email: str,
        password: str,
        is_admin: bool = False,
        roles: Optional[List[str]] = None,
    ) -> User:
        """
        Create new user (admin only).
        
        Args:
            username: Username
            email: Email address
            password: Password
            is_admin: Whether user is admin
            roles: User roles
        
        Returns:
            Created user
        """
        data = {
            "username": username,
            "email": email,
            "password": password,
            "is_admin": is_admin,
            "roles": roles or [],
        }
        response = self._request("POST", "/api/v1/users", json=data)
        return User.from_dict(response)
    
    def delete_user(self, username: str) -> None:
        """Delete user (admin only)."""
        self._request("DELETE", f"/api/v1/users/{username}")
    
    # Health checks
    
    def health_check(self) -> HealthStatus:
        """Check server health."""
        response = self._request("GET", "/health")
        return HealthStatus.from_dict(response)
    
    def is_healthy(self) -> bool:
        """Check if server is healthy."""
        try:
            status = self.health_check()
            return status.status == "healthy"
        except:
            return False
    
    # Utility methods
    
    def close(self) -> None:
        """Close client session."""
        self.session.close()
    
    def __enter__(self):
        """Context manager entry."""
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit."""
        self.close()