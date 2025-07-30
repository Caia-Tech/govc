export interface ClientConfig {
  baseURL: string;
  token?: string;
  apiKey?: string;
  headers?: Record<string, string>;
  timeout?: number;
}

export interface ErrorResponse {
  error: string;
  code: string;
}

export interface HealthResponse {
  status: string;
  timestamp: string;
  uptime: string;
  version: string;
  checks: Record<string, string>;
}

export interface CreateRepoOptions {
  memoryOnly?: boolean;
}

export interface Repository {
  id: string;
  path: string;
  current_branch?: string;
  created_at: string;
}

export interface Author {
  name: string;
  email: string;
}

export interface Commit {
  hash: string;
  message: string;
  author: string;
  email: string;
  timestamp: string;
  parent?: string;
}

export interface Status {
  branch: string;
  staged: string[];
  modified: string[];
  untracked: string[];
  clean: boolean;
}

export interface Branch {
  name: string;
  commit: string;
  is_current: boolean;
}

export interface Tag {
  name: string;
  commit: string;
}

export interface Transaction {
  id: string;
  repo_id: string;
  created_at: string;
}

export interface TransactionStatus {
  id: string;
  repo_id: string;
  state: string;
  files: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface ParallelReality {
  name: string;
  isolated: boolean;
  ephemeral: boolean;
  metrics?: Record<string, any>;
}

export interface BenchmarkResult {
  reality: string;
  duration: string;
  metrics: Record<string, number>;
  better: boolean;
}

export interface HistoricalSnapshot {
  time: string;
  commit: Commit;
  files: Array<{
    path: string;
    content: string;
  }>;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  expires_at: string;
  user: User;
}

export interface User {
  id: string;
  username: string;
  email: string;
  roles: string[];
}

export interface APIKey {
  id: string;
  name: string;
  key_hash: string;
  permissions: string[];
  created_at: string;
  last_used_at?: string;
  expires_at?: string;
}

export interface CreateAPIKeyRequest {
  name: string;
  permissions?: string[];
  expires_in?: string;
}

export interface CreateAPIKeyResponse {
  id: string;
  key: string;
  api_key: APIKey;
}