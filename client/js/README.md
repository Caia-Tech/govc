# Govc JavaScript/TypeScript SDK

The official JavaScript/TypeScript client library for interacting with govc servers.

## Installation

```bash
npm install @caia-tech/govc-client
# or
yarn add @caia-tech/govc-client
# or
pnpm add @caia-tech/govc-client
```

## Quick Start

```typescript
import { GovcClient } from '@caia-tech/govc-client';

// Create a client
const client = new GovcClient('https://govc.example.com');

// Or with configuration
const client = new GovcClient({
  baseURL: 'https://govc.example.com',
  token: 'your-jwt-token',
  timeout: 30000,
});

// Create a repository
const repo = await client.createRepo('my-project', { memoryOnly: true });

// Add files
await repo.addFile('README.md', '# My Project\n\nWelcome!');
await repo.addFile('index.js', 'console.log("Hello, govc!");');

// Commit changes
const commit = await repo.commit('Initial commit', {
  name: 'John Doe',
  email: 'john@example.com',
});

console.log(`Created commit: ${commit.hash}`);
```

## Authentication

### JWT Authentication

```typescript
// Login with username/password
const auth = client.auth;
const response = await auth.login('username', 'password');
console.log(`Token expires at: ${response.expires_at}`);

// The token is automatically stored in the client
// You can also set it manually:
client.setToken('your-jwt-token');

// Refresh token before expiry
await auth.refreshToken();
```

### API Key Authentication

```typescript
// Create an API key (requires JWT auth first)
const keyResponse = await auth.createAPIKey('ci-key', ['repo:read', 'repo:write']);
console.log(`New API key: ${keyResponse.key}`);

// Use the API key
client.setAPIKey(keyResponse.key);

// Or create a new client with API key
const client = new GovcClient({
  baseURL: 'https://govc.example.com',
  apiKey: 'your-api-key',
});
```

## Repository Operations

### Basic Operations

```typescript
// Get a repository
const repo = await client.getRepo('my-project');

// List all repositories
const repos = await client.listRepos();
repos.forEach(r => console.log(r.id));

// Delete a repository
await client.deleteRepo('old-project');
```

### Working with Files

```typescript
// Add files to staging
await repo.addFile('src/app.js', 'const app = express();');

// Write files directly
await repo.writeFile('config.json', JSON.stringify({ port: 3000 }));

// Read files
const content = await repo.readFile('README.md');
console.log(content);

// Remove files
await repo.removeFile('old-file.txt');

// Get repository status
const status = await repo.status();
console.log(`Branch: ${status.branch}, Staged: ${status.staged.length} files`);
```

### Branch Management

```typescript
// List branches
const branches = await repo.listBranches();

// Create a new branch
await repo.createBranch('feature/new-feature', 'main');

// Checkout a branch
await repo.checkout('feature/new-feature');

// Delete a branch
await repo.deleteBranch('old-branch');

// Merge branches
await repo.merge('feature/new-feature', 'main');
```

### Tags

```typescript
// List tags
const tags = await repo.listTags();

// Create a tag
await repo.createTag('v1.0.0', 'Release version 1.0.0');
```

## Transactions

Transactions provide atomic operations:

```typescript
// Start a transaction
const tx = await repo.transaction();

try {
  // Add multiple files
  await tx.add('file1.txt', 'Content 1');
  await tx.add('file2.txt', 'Content 2');
  await tx.add('file3.txt', 'Content 3');

  // Validate the transaction
  await tx.validate();

  // Commit if valid
  const commit = await tx.commit('Add multiple files atomically');
  console.log(`Transaction committed: ${commit.hash}`);
} catch (error) {
  // Rollback on error
  await tx.rollback();
  console.error('Transaction failed:', error);
}
```

## Real-time Events

The SDK supports real-time event streaming using Server-Sent Events (SSE):

### Using Observables (RxJS-style)

```typescript
// Watch for repository events
const subscription = repo.watch().subscribe(
  (event) => {
    console.log(`Event: ${event.type} at ${event.timestamp}`);
    console.log('Data:', event.data);
  },
  (error) => {
    console.error('Stream error:', error);
  },
  () => {
    console.log('Stream completed');
  }
);

// Unsubscribe when done
subscription.unsubscribe();
```

### Using Event Streams (Direct API)

```typescript
// Create an event stream
const stream = repo.createEventStream();

// Add event handler
const removeHandler = stream.onEvent((event) => {
  console.log(`${event.type} event:`, event.data);
});

// Add error handler
const removeErrorHandler = stream.onError((error) => {
  console.error('Stream error:', error);
});

// Connect to the stream
stream.connect();

// Later: disconnect and cleanup
removeHandler();
removeErrorHandler();
stream.disconnect();
```

## Advanced Features

### Parallel Realities

Test multiple configurations simultaneously:

```typescript
// Create parallel realities
const realities = await repo.createParallelRealities([
  'config-a',
  'config-b',
  'config-c',
]);

// Benchmark a reality
const result = await repo.benchmarkReality('config-a');
console.log(`Reality ${result.reality} performance:`, result.metrics);
```

### Time Travel

Access historical snapshots:

```typescript
// Get repository state at a specific time
const snapshot = await repo.timeTravel('2024-01-01T12:00:00Z');

// Access files from that point in time
snapshot.files.forEach(file => {
  console.log(`${file.path}: ${file.content.substring(0, 50)}...`);
});

// See the commit at that time
console.log(`Commit: ${snapshot.commit.message} by ${snapshot.commit.author}`);
```

## Error Handling

The SDK provides typed errors for better error handling:

```typescript
import { GovcError } from '@caia-tech/govc-client';

try {
  const repo = await client.getRepo('non-existent');
} catch (error) {
  if (error instanceof GovcError) {
    console.error(`Error code: ${error.code}`);
    console.error(`Message: ${error.message}`);
    
    switch (error.code) {
      case 'REPO_NOT_FOUND':
        // Handle repository not found
        break;
      case 'UNAUTHORIZED':
        // Handle auth error
        break;
      case 'TIMEOUT':
        // Handle timeout
        break;
      default:
        // Handle other errors
    }
  }
}
```

## TypeScript Support

The SDK is written in TypeScript and provides full type definitions:

```typescript
import { 
  GovcClient, 
  Repository, 
  Commit, 
  Branch,
  CreateRepoOptions,
  RepositoryEvent
} from '@caia-tech/govc-client';

// All types are fully typed
const handleEvent = (event: RepositoryEvent): void => {
  switch (event.type) {
    case 'commit':
      console.log('New commit:', event.data);
      break;
    case 'branch':
      console.log('Branch event:', event.data);
      break;
  }
};
```

## Examples

### CI/CD Integration

```typescript
// CI script example
async function runCI() {
  const client = new GovcClient({
    baseURL: process.env.GOVC_URL!,
    apiKey: process.env.GOVC_API_KEY!,
  });

  const repo = await client.getRepo('my-app');
  
  // Create a test branch
  await repo.createBranch('ci/test-run', 'main');
  await repo.checkout('ci/test-run');
  
  // Run tests in parallel realities
  const realities = await repo.createParallelRealities([
    'test-node-16',
    'test-node-18',
    'test-node-20',
  ]);
  
  // Benchmark each configuration
  const results = await Promise.all(
    realities.map(r => repo.benchmarkReality(r.name))
  );
  
  // Find best performer
  const best = results.reduce((a, b) => a.better ? a : b);
  console.log(`Best configuration: ${best.reality}`);
}
```

### React Hook Example

```typescript
// useRepository.ts
import { useEffect, useState } from 'react';
import { GovcClient, RepositoryClient, RepositoryEvent } from '@caia-tech/govc-client';

export function useRepository(repoId: string) {
  const [repo, setRepo] = useState<RepositoryClient | null>(null);
  const [events, setEvents] = useState<RepositoryEvent[]>([]);
  
  useEffect(() => {
    const client = new GovcClient(process.env.REACT_APP_GOVC_URL!);
    
    client.getRepo(repoId).then(r => {
      setRepo(r);
      
      // Subscribe to events
      const subscription = r.watch().subscribe(
        (event) => setEvents(prev => [...prev, event]),
        (error) => console.error('Event stream error:', error)
      );
      
      return () => subscription.unsubscribe();
    });
  }, [repoId]);
  
  return { repo, events };
}
```

## Browser Support

The SDK works in both Node.js and browser environments. For browsers, ensure your bundler includes the necessary polyfills for:
- `fetch` (if targeting older browsers)
- `EventSource` (for event streaming)

## API Reference

For detailed API documentation, see the [TypeScript definitions](https://github.com/caia-tech/govc/tree/main/client/js/src/types.ts).

## Contributing

Contributions are welcome! Please read our [Contributing Guide](../../CONTRIBUTING.md) for details.

## License

This SDK is part of the govc project and is licensed under the same terms.