import { GovcClient, RepositoryClient } from '../src';
import fetch from 'cross-fetch';

jest.mock('cross-fetch');
const mockFetch = fetch as jest.MockedFunction<typeof fetch>;

describe('RepositoryClient', () => {
  let client: GovcClient;
  let repo: RepositoryClient;

  beforeEach(() => {
    jest.clearAllMocks();
    client = new GovcClient('https://api.example.com');
    repo = new RepositoryClient(client, {
      id: 'test-repo',
      path: '/repos/test-repo',
      current_branch: 'main',
      created_at: new Date().toISOString(),
    });
  });

  describe('file operations', () => {
    it('should add file to staging', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'added', message: 'file added' }),
      } as Response);

      await repo.addFile('test.txt', 'Hello, World!');

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/add',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            path: 'test.txt',
            content: 'Hello, World!',
          }),
        })
      );
    });

    it('should read file', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          path: 'test.txt',
          content: 'Hello, World!',
          size: 13,
        }),
      } as Response);

      const content = await repo.readFile('test.txt');
      
      expect(content).toBe('Hello, World!');
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/read/test.txt',
        expect.any(Object)
      );
    });

    it('should remove file', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'removed' }),
      } as Response);

      await repo.removeFile('test.txt');

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/remove/test.txt',
        expect.objectContaining({
          method: 'DELETE',
        })
      );
    });
  });

  describe('git operations', () => {
    it('should create commit', async () => {
      const commitData = {
        hash: 'abc123',
        message: 'Test commit',
        author: 'Test User',
        email: 'test@example.com',
        timestamp: new Date().toISOString(),
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => commitData,
      } as Response);

      const commit = await repo.commit('Test commit', {
        name: 'Test User',
        email: 'test@example.com',
      });

      expect(commit).toEqual(commitData);
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/commit',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            message: 'Test commit',
            author: 'Test User',
            email: 'test@example.com',
          }),
        })
      );
    });

    it('should get repository status', async () => {
      const statusData = {
        branch: 'main',
        staged: ['file1.txt'],
        modified: ['file2.txt'],
        untracked: ['file3.txt'],
        clean: false,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => statusData,
      } as Response);

      const status = await repo.status();
      
      expect(status).toEqual(statusData);
    });

    it('should get commit log', async () => {
      const commits = [
        {
          hash: 'abc123',
          message: 'Commit 1',
          author: 'User 1',
          email: 'user1@example.com',
          timestamp: new Date().toISOString(),
        },
        {
          hash: 'def456',
          message: 'Commit 2',
          author: 'User 2',
          email: 'user2@example.com',
          timestamp: new Date().toISOString(),
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ commits, count: 2 }),
      } as Response);

      const log = await repo.log(10);
      
      expect(log).toEqual(commits);
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/log?limit=10',
        expect.any(Object)
      );
    });
  });

  describe('branch operations', () => {
    it('should list branches', async () => {
      const branches = [
        { name: 'main', commit: 'abc123', is_current: true },
        { name: 'feature', commit: 'def456', is_current: false },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ branches, current: 'main', count: 2 }),
      } as Response);

      const result = await repo.listBranches();
      
      expect(result).toEqual(branches);
    });

    it('should create branch', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: async () => ({ status: 'created', message: 'branch created' }),
      } as Response);

      await repo.createBranch('feature/new', 'main');

      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/branches',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            name: 'feature/new',
            from: 'main',
          }),
        })
      );
    });

    it('should checkout branch', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ status: 'success', message: 'switched to feature' }),
      } as Response);

      await repo.checkout('feature');

      expect(repo.currentBranch).toBe('feature');
      expect(mockFetch).toHaveBeenCalledWith(
        'https://api.example.com/api/v1/repos/test-repo/checkout',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ branch: 'feature' }),
        })
      );
    });
  });

  describe('advanced features', () => {
    it('should create parallel realities', async () => {
      const realities = [
        { name: 'reality-1', isolated: true, ephemeral: true },
        { name: 'reality-2', isolated: true, ephemeral: true },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ realities, count: 2 }),
      } as Response);

      const result = await repo.createParallelRealities(['reality-1', 'reality-2']);
      
      expect(result).toEqual(realities);
    });

    it('should time travel', async () => {
      const snapshot = {
        time: '2024-01-01T12:00:00Z',
        commit: {
          hash: 'abc123',
          message: 'Historical commit',
          author: 'User',
          email: 'user@example.com',
          timestamp: '2024-01-01T12:00:00Z',
        },
        files: [
          { path: 'file1.txt', content: 'content1' },
          { path: 'file2.txt', content: 'content2' },
        ],
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => snapshot,
      } as Response);

      const result = await repo.timeTravel('2024-01-01T12:00:00Z');
      
      expect(result).toEqual(snapshot);
    });
  });

  describe('event streaming', () => {
    it('should create event observable', () => {
      const observable = repo.watch();
      expect(observable).toBeDefined();
    });

    it('should create event stream', () => {
      const stream = repo.createEventStream();
      expect(stream).toBeDefined();
    });
  });
});