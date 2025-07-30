import { 
  GovcClient, 
  GovcError,
  RepositoryClient,
  Commit,
  RepositoryEvent 
} from '@caia-tech/govc-client';

async function demonstrateTypeScript(): Promise<void> {
  // Create a strongly-typed client
  const client = new GovcClient({
    baseURL: process.env.GOVC_URL || 'http://localhost:8080',
    timeout: 30000,
  });

  try {
    // Type-safe repository creation
    const repo: RepositoryClient = await client.createRepo('ts-example', {
      memoryOnly: true,
    });

    // Transaction with full type safety
    const tx = await repo.transaction();
    
    await tx.add('src/types.ts', `
export interface User {
  id: string;
  name: string;
  email: string;
}

export interface Project {
  id: string;
  name: string;
  owner: User;
  created: Date;
}
`);

    await tx.add('src/index.ts', `
import { User, Project } from './types';

export class ProjectManager {
  private projects: Map<string, Project> = new Map();

  createProject(name: string, owner: User): Project {
    const project: Project = {
      id: crypto.randomUUID(),
      name,
      owner,
      created: new Date(),
    };
    
    this.projects.set(project.id, project);
    return project;
  }

  getProject(id: string): Project | undefined {
    return this.projects.get(id);
  }
}
`);

    // Validate and commit
    const isValid = await tx.validate();
    if (isValid) {
      const commit: Commit = await tx.commit('Add TypeScript project structure');
      console.log(`Committed: ${commit.hash} - ${commit.message}`);
    }

    // Parallel realities with type checking
    const realities = await repo.createParallelRealities([
      'test-typescript-strict',
      'test-typescript-loose',
      'test-javascript',
    ]);

    // Type-safe event handling
    const handleEvent = (event: RepositoryEvent): void => {
      switch (event.type) {
        case 'commit':
          console.log(`New commit in ${event.repository}`);
          break;
        case 'branch':
          console.log(`Branch event in ${event.repository}`);
          break;
        case 'merge':
          console.log(`Merge event in ${event.repository}`);
          break;
      }
    };

    // Subscribe with proper typing
    const subscription = repo.watch().subscribe(
      handleEvent,
      (error: Error) => console.error('Stream error:', error),
      () => console.log('Stream completed')
    );

    // Clean up after 3 seconds
    setTimeout(() => {
      subscription.unsubscribe();
      client.deleteRepo('ts-example').catch(console.error);
    }, 3000);

  } catch (error) {
    // Type guard for GovcError
    if (error instanceof GovcError) {
      console.error(`Govc error [${error.code}]: ${error.message}`);
      
      // Handle specific error codes
      switch (error.code) {
        case 'REPO_NOT_FOUND':
          console.error('Repository does not exist');
          break;
        case 'UNAUTHORIZED':
          console.error('Authentication required');
          break;
        case 'TIMEOUT':
          console.error('Request timed out');
          break;
      }
    } else {
      console.error('Unexpected error:', error);
    }
  }
}

// Async function with error boundaries
async function runWithErrorBoundary(): Promise<void> {
  try {
    await demonstrateTypeScript();
  } catch (error) {
    console.error('Fatal error:', error);
    process.exit(1);
  }
}

// Run the demonstration
runWithErrorBoundary();