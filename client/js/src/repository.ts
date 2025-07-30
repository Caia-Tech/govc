import { GovcClient } from './client';
import {
  Repository,
  Commit,
  Status,
  Branch,
  Tag,
  Author,
  ParallelReality,
  BenchmarkResult,
  HistoricalSnapshot
} from './types';
import { TransactionClient } from './transaction';
import { EventStream, EventObservable, RepositoryEvent } from './events';

export class RepositoryClient {
  private client: GovcClient;
  public id: string;
  public path: string;
  public currentBranch?: string;
  public createdAt: string;

  constructor(client: GovcClient, repo: Repository) {
    this.client = client;
    this.id = repo.id;
    this.path = repo.path;
    this.currentBranch = repo.current_branch;
    this.createdAt = repo.created_at;
  }

  // File operations
  async addFile(path: string, content: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/add`, {
      path,
      content,
    });
  }

  async writeFile(path: string, content: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/write`, {
      path,
      content,
    });
  }

  async readFile(path: string): Promise<string> {
    const response = await this.client.get<{
      path: string;
      content: string;
      size: number;
      encoding?: string;
    }>(`/api/v1/repos/${encodeURIComponent(this.id)}/read/${encodeURIComponent(path)}`);
    
    return response.content;
  }

  async removeFile(path: string): Promise<void> {
    await this.client.delete(
      `/api/v1/repos/${encodeURIComponent(this.id)}/remove/${encodeURIComponent(path)}`
    );
  }

  // Git operations
  async commit(message: string, author?: Author): Promise<Commit> {
    return this.client.post<Commit>(
      `/api/v1/repos/${encodeURIComponent(this.id)}/commit`,
      {
        message,
        author: author?.name,
        email: author?.email,
      }
    );
  }

  async status(): Promise<Status> {
    return this.client.get<Status>(
      `/api/v1/repos/${encodeURIComponent(this.id)}/status`
    );
  }

  async log(limit?: number): Promise<Commit[]> {
    const path = `/api/v1/repos/${encodeURIComponent(this.id)}/log` +
      (limit ? `?limit=${limit}` : '');
    
    const response = await this.client.get<{
      commits: Commit[];
      count: number;
    }>(path);
    
    return response.commits;
  }

  // Branch operations
  async listBranches(): Promise<Branch[]> {
    const response = await this.client.get<{
      branches: Branch[];
      current: string;
      count: number;
    }>(`/api/v1/repos/${encodeURIComponent(this.id)}/branches`);
    
    return response.branches;
  }

  async createBranch(name: string, from?: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/branches`, {
      name,
      from,
    });
  }

  async deleteBranch(name: string): Promise<void> {
    await this.client.delete(
      `/api/v1/repos/${encodeURIComponent(this.id)}/branches/${encodeURIComponent(name)}`
    );
  }

  async checkout(branch: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/checkout`, {
      branch,
    });
    this.currentBranch = branch;
  }

  async merge(from: string, to: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/merge`, {
      from,
      to,
    });
  }

  // Tag operations
  async listTags(): Promise<Tag[]> {
    return this.client.get<Tag[]>(
      `/api/v1/repos/${encodeURIComponent(this.id)}/tags`
    );
  }

  async createTag(name: string, message: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/tags`, {
      name,
      message,
    });
  }

  // Transaction
  async transaction(): Promise<TransactionClient> {
    return this.client.transaction(this.id);
  }

  // Advanced features
  async createParallelRealities(names: string[]): Promise<ParallelReality[]> {
    const response = await this.client.post<{
      realities: ParallelReality[];
      count: number;
    }>(`/api/v1/repos/${encodeURIComponent(this.id)}/parallel-realities`, {
      branches: names,
    });
    
    return response.realities;
  }

  async benchmarkReality(realityName: string): Promise<BenchmarkResult> {
    return this.client.post<BenchmarkResult>(
      `/api/v1/repos/${encodeURIComponent(this.id)}/parallel-realities/${encodeURIComponent(realityName)}/benchmark`
    );
  }

  async timeTravel(timestamp: string): Promise<HistoricalSnapshot> {
    return this.client.get<HistoricalSnapshot>(
      `/api/v1/repos/${encodeURIComponent(this.id)}/time-travel?time=${encodeURIComponent(timestamp)}`
    );
  }

  // Stash operations
  async createStash(message?: string): Promise<void> {
    await this.client.post(`/api/v1/repos/${encodeURIComponent(this.id)}/stash`, {
      message,
    });
  }

  async listStashes(): Promise<any[]> {
    const response = await this.client.get<{
      stashes: any[];
      count: number;
    }>(`/api/v1/repos/${encodeURIComponent(this.id)}/stash`);
    
    return response.stashes;
  }

  async applyStash(stashId: string): Promise<void> {
    await this.client.post(
      `/api/v1/repos/${encodeURIComponent(this.id)}/stash/${encodeURIComponent(stashId)}/apply`
    );
  }

  async dropStash(stashId: string): Promise<void> {
    await this.client.delete(
      `/api/v1/repos/${encodeURIComponent(this.id)}/stash/${encodeURIComponent(stashId)}`
    );
  }

  // Event streaming
  watch(): EventObservable {
    const baseURL = (this.client as any).baseURL; // Access private property
    const url = `${baseURL}/api/v1/repos/${encodeURIComponent(this.id)}/events`;
    return new EventObservable(url);
  }

  // Direct event stream access
  createEventStream(): EventStream {
    const baseURL = (this.client as any).baseURL; // Access private property
    const url = `${baseURL}/api/v1/repos/${encodeURIComponent(this.id)}/events`;
    return new EventStream(url);
  }
}