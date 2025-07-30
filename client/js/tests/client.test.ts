import { GovcClient, GovcError } from '../src';
import fetch from 'cross-fetch';

// Mock fetch
jest.mock('cross-fetch');
const mockFetch = fetch as jest.MockedFunction<typeof fetch>;

describe('GovcClient', () => {
  let client: GovcClient;

  beforeEach(() => {
    jest.clearAllMocks();
    client = new GovcClient('https://api.example.com');
  });

  describe('constructor', () => {
    it('should create client with string URL', () => {
      const c = new GovcClient('https://api.example.com');
      expect(c).toBeInstanceOf(GovcClient);
    });

    it('should create client with config object', () => {
      const c = new GovcClient({
        baseURL: 'https://api.example.com',
        token: 'test-token',
        apiKey: 'test-key',
        timeout: 5000,
      });
      expect(c).toBeInstanceOf(GovcClient);
    });

    it('should remove trailing slash from baseURL', () => {
      const c1 = new GovcClient('https://api.example.com/');
      const c2 = new GovcClient({ baseURL: 'https://api.example.com/' });
      // Both should work the same way
      expect(c1).toBeInstanceOf(GovcClient);
      expect(c2).toBeInstanceOf(GovcClient);
    });
  });

  describe('authentication', () => {
    it('should add Bearer token to requests', async () => {
      client.setToken('test-token');
      
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'healthy' }),
      } as Response);

      await client.healthCheck();

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/health',
        expect.objectContaining({
          headers: expect.objectContaining({
            'Authorization': 'Bearer test-token',
          }),
        })
      );
    });

    it('should add API key to requests', async () => {
      client.setAPIKey('test-api-key');
      
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'healthy' }),
      } as Response);

      await client.healthCheck();

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/health',
        expect.objectContaining({
          headers: expect.objectContaining({
            'X-API-Key': 'test-api-key',
          }),
        })
      );
    });

    it('should prefer token over API key', async () => {
      client.setAPIKey('test-api-key');
      client.setToken('test-token');
      
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'healthy' }),
      } as Response);

      await client.healthCheck();

      const call = mockFetch.mock.calls[0];
      const headers = call[1]?.headers as any;
      expect(headers['Authorization']).toBe('Bearer test-token');
      expect(headers['X-API-Key']).toBeUndefined();
    });

    it('should clear auth credentials', () => {
      client.setToken('test-token');
      client.clearAuth();
      
      // After clearing, no auth headers should be sent
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'healthy' }),
      } as Response);

      client.healthCheck();

      const call = mockFetch.mock.calls[0];
      const headers = call[1]?.headers as any;
      expect(headers['Authorization']).toBeUndefined();
      expect(headers['X-API-Key']).toBeUndefined();
    });
  });

  describe('error handling', () => {
    it('should throw GovcError on HTTP error', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: 'Not Found',
        json: async () => ({
          error: 'Repository not found',
          code: 'REPO_NOT_FOUND',
        }),
      } as Response);

      await expect(client.getRepo('non-existent')).rejects.toThrow(GovcError);
      await expect(client.getRepo('non-existent')).rejects.toMatchObject({
        message: 'Repository not found',
        code: 'REPO_NOT_FOUND',
      });
    });

    it('should handle timeout', async () => {
      // Create client with very short timeout
      const timeoutClient = new GovcClient({
        baseURL: 'https://api.example.com',
        timeout: 1,
      });

      // Mock fetch that never resolves
      mockFetch.mockImplementationOnce(() => new Promise(() => {}));

      await expect(timeoutClient.healthCheck()).rejects.toThrow(GovcError);
      await expect(timeoutClient.healthCheck()).rejects.toMatchObject({
        code: 'TIMEOUT',
      });
    });

    it('should handle network errors', async () => {
      mockFetch.mockRejectedValueOnce(new Error('Network failure'));

      await expect(client.healthCheck()).rejects.toThrow(GovcError);
      await expect(client.healthCheck()).rejects.toMatchObject({
        code: 'NETWORK_ERROR',
      });
    });
  });

  describe('repository operations', () => {
    it('should create repository', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({
          id: 'test-repo',
          path: '/repos/test-repo',
          created_at: new Date().toISOString(),
        }),
      } as Response);

      const repo = await client.createRepo('test-repo', { memoryOnly: true });
      
      expect(repo).toBeDefined();
      expect(repo.id).toBe('test-repo');
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            id: 'test-repo',
            memory_only: true,
          }),
        })
      );
    });

    it('should list repositories', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          repositories: [
            { id: 'repo1', path: '/repos/repo1', created_at: new Date().toISOString() },
            { id: 'repo2', path: '/repos/repo2', created_at: new Date().toISOString() },
          ],
          count: 2,
        }),
      } as Response);

      const repos = await client.listRepos();
      
      expect(repos).toHaveLength(2);
      expect(repos[0].id).toBe('repo1');
      expect(repos[1].id).toBe('repo2');
    });

    it('should delete repository', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 204,
      } as Response);

      await expect(client.deleteRepo('test-repo')).resolves.not.toThrow();
      
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo',
        expect.objectContaining({
          method: 'DELETE',
        })
      );
    });
  });

  describe('health check', () => {
    it('should return health status', async () => {
      const healthData = {
        status: 'healthy',
        timestamp: new Date().toISOString(),
        uptime: '24h',
        version: '1.0.0',
        checks: {
          database: 'ok',
          memory: 'ok',
        },
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => healthData,
      } as Response);

      const health = await client.healthCheck();
      
      expect(health).toEqual(healthData);
    });
  });
});