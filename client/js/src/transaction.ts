import { GovcClient } from './client';
import { Transaction, TransactionStatus, Commit } from './types';

export class TransactionClient {
  private client: GovcClient;
  private repoId: string;
  public id: string;
  public createdAt: string;

  constructor(client: GovcClient, repoId: string, tx: Transaction) {
    this.client = client;
    this.repoId = repoId;
    this.id = tx.id;
    this.createdAt = tx.created_at;
  }

  async add(path: string, content: string): Promise<void> {
    await this.client.post(
      `/api/v1/repos/${encodeURIComponent(this.repoId)}/transactions/${encodeURIComponent(this.id)}/add`,
      { path, content }
    );
  }

  async remove(path: string): Promise<void> {
    await this.client.delete(
      `/api/v1/repos/${encodeURIComponent(this.repoId)}/transactions/${encodeURIComponent(this.id)}/remove/${encodeURIComponent(path)}`
    );
  }

  async validate(): Promise<boolean> {
    const response = await this.client.post<{
      valid: boolean;
      errors?: string[];
      message: string;
    }>(
      `/api/v1/repos/${encodeURIComponent(this.repoId)}/transactions/${encodeURIComponent(this.id)}/validate`
    );

    if (!response.valid && response.errors && response.errors.length > 0) {
      throw new Error(`Validation failed: ${response.errors.join(', ')}`);
    }

    return response.valid;
  }

  async commit(message: string): Promise<Commit> {
    return this.client.post<Commit>(
      `/api/v1/repos/${encodeURIComponent(this.repoId)}/transactions/${encodeURIComponent(this.id)}/commit`,
      { message }
    );
  }

  async rollback(): Promise<void> {
    await this.client.post(
      `/api/v1/repos/${encodeURIComponent(this.repoId)}/transactions/${encodeURIComponent(this.id)}/rollback`
    );
  }

  async status(): Promise<TransactionStatus> {
    return this.client.get<TransactionStatus>(
      `/api/v1/repos/${encodeURIComponent(this.repoId)}/transactions/${encodeURIComponent(this.id)}`
    );
  }
}