import { GovcClient } from './client';
import {
  LoginRequest,
  LoginResponse,
  User,
  APIKey,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse
} from './types';

export class AuthClient {
  private client: GovcClient;

  constructor(client: GovcClient) {
    this.client = client;
  }

  async login(username: string, password: string): Promise<LoginResponse> {
    const request: LoginRequest = { username, password };
    const response = await this.client.post<LoginResponse>('/api/v1/auth/login', request);
    
    // Automatically set the token
    this.client.setToken(response.token);
    
    return response;
  }

  async refreshToken(): Promise<LoginResponse> {
    const response = await this.client.post<LoginResponse>('/api/v1/auth/refresh');
    
    // Update the token
    this.client.setToken(response.token);
    
    return response;
  }

  async whoami(): Promise<User> {
    return this.client.get<User>('/api/v1/auth/whoami');
  }

  async createAPIKey(name: string, permissions?: string[]): Promise<CreateAPIKeyResponse> {
    const request: CreateAPIKeyRequest = { name, permissions };
    return this.client.post<CreateAPIKeyResponse>('/api/v1/apikeys', request);
  }

  async listAPIKeys(): Promise<APIKey[]> {
    const response = await this.client.get<{
      keys: APIKey[];
      count: number;
    }>('/api/v1/apikeys');
    
    return response.keys;
  }

  async revokeAPIKey(keyId: string): Promise<void> {
    await this.client.delete(`/api/v1/apikeys/${encodeURIComponent(keyId)}`);
  }
}