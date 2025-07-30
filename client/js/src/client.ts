import fetch from 'cross-fetch';
import { 
  ClientConfig, 
  ErrorResponse, 
  HealthResponse,
  Repository,
  CreateRepoOptions
} from './types';
import { RepositoryClient } from './repository';
import { AuthClient } from './auth';
import { TransactionClient } from './transaction';

export class GovcError extends Error {
  public code: string;

  constructor(message: string, code: string) {
    super(message);
    this.name = 'GovcError';
    this.code = code;
  }
}

export class GovcClient {
  private baseURL: string;
  private token?: string;
  private apiKey?: string;
  private headers: Record<string, string>;
  private timeout: number;
  
  public auth: AuthClient;

  constructor(config: ClientConfig | string) {
    if (typeof config === 'string') {
      config = { baseURL: config };
    }

    this.baseURL = config.baseURL.replace(/\/$/, ''); // Remove trailing slash
    this.token = config.token;
    this.apiKey = config.apiKey;
    this.headers = config.headers || {};
    this.timeout = config.timeout || 30000;
    
    this.auth = new AuthClient(this);
  }

  // Internal method to make HTTP requests
  async request<T>(
    method: string,
    path: string,
    body?: any,
    options?: RequestInit
  ): Promise<T> {
    const url = `${this.baseURL}${path}`;
    
    const headers: Record<string, string> = {
      ...this.headers,
      'Content-Type': 'application/json',
    };

    // Add authentication
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    } else if (this.apiKey) {
      headers['X-API-Key'] = this.apiKey;
    }

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
        signal: controller.signal,
        ...options,
      });

      clearTimeout(timeoutId);

      if (!response.ok) {
        const error: ErrorResponse = await response.json().catch(() => ({
          error: response.statusText,
          code: 'UNKNOWN_ERROR',
        }));
        throw new GovcError(error.error, error.code);
      }

      if (response.status === 204) {
        return {} as T;
      }

      return await response.json();
    } catch (error: any) {
      clearTimeout(timeoutId);
      
      if (error.name === 'AbortError') {
        throw new GovcError('Request timeout', 'TIMEOUT');
      }
      
      if (error instanceof GovcError) {
        throw error;
      }
      
      throw new GovcError(error.message || 'Network error', 'NETWORK_ERROR');
    }
  }

  // Convenience methods
  async get<T>(path: string): Promise<T> {
    return this.request<T>('GET', path);
  }

  async post<T>(path: string, body?: any): Promise<T> {
    return this.request<T>('POST', path, body);
  }

  async put<T>(path: string, body?: any): Promise<T> {
    return this.request<T>('PUT', path, body);
  }

  async delete<T>(path: string): Promise<T> {
    return this.request<T>('DELETE', path);
  }

  // Set authentication token
  setToken(token: string): void {
    this.token = token;
    this.apiKey = undefined;
  }

  // Set API key
  setAPIKey(apiKey: string): void {
    this.apiKey = apiKey;
    this.token = undefined;
  }

  // Clear authentication
  clearAuth(): void {
    this.token = undefined;
    this.apiKey = undefined;
  }

  // Health check
  async healthCheck(): Promise<HealthResponse> {
    return this.get<HealthResponse>('/api/v1/health');
  }

  // Repository operations
  async createRepo(id: string, options?: CreateRepoOptions): Promise<RepositoryClient> {
    const repo = await this.post<Repository>('/api/v1/repos', {
      id,
      memory_only: options?.memoryOnly,
    });
    return new RepositoryClient(this, repo);
  }

  async getRepo(id: string): Promise<RepositoryClient> {
    const repo = await this.get<Repository>(`/api/v1/repos/${encodeURIComponent(id)}`);
    return new RepositoryClient(this, { ...repo, id });
  }

  async listRepos(): Promise<RepositoryClient[]> {
    const response = await this.get<{
      repositories: Repository[];
      count: number;
    }>('/api/v1/repos');
    
    return response.repositories.map(repo => new RepositoryClient(this, repo));
  }

  async deleteRepo(id: string): Promise<void> {
    await this.delete(`/api/v1/repos/${encodeURIComponent(id)}`);
  }

  // Create transaction for a repository
  async transaction(repoId: string): Promise<TransactionClient> {
    const tx = await this.post<any>(
      `/api/v1/repos/${encodeURIComponent(repoId)}/transactions`
    );
    return new TransactionClient(this, repoId, tx);
  }
}