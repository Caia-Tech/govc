const { GovcClient } = require('@caiatech/govc-client');

async function main() {
  // Create client
  const client = new GovcClient(process.env.GOVC_URL || 'http://localhost:8080');

  try {
    // Check server health
    console.log('Checking server health...');
    const health = await client.healthCheck();
    console.log(`Server status: ${health.status}, version: ${health.version}`);

    // Create a repository
    console.log('\nCreating repository...');
    const repo = await client.createRepo('example-repo-js', { memoryOnly: true });
    console.log(`Created repository: ${repo.id}`);

    // Add some files
    console.log('\nAdding files...');
    await repo.addFile('index.js', `
console.log('Hello from govc!');

function greet(name) {
  return \`Hello, \${name}! Welcome to govc.\`;
}

module.exports = { greet };
`);

    await repo.addFile('package.json', JSON.stringify({
      name: 'example-project',
      version: '1.0.0',
      description: 'Example project using govc',
      main: 'index.js',
    }, null, 2));

    await repo.addFile('README.md', `# Example Project

This project demonstrates the govc JavaScript SDK.

## Features
- Memory-first Git operations
- Real-time event streaming
- Parallel reality testing
`);

    // Check status
    console.log('\nRepository status:');
    const status = await repo.status();
    console.log(`Branch: ${status.branch}`);
    console.log(`Staged files: ${status.staged.join(', ')}`);

    // Commit changes
    console.log('\nCommitting changes...');
    const commit = await repo.commit('Initial commit with example files', {
      name: 'Example User',
      email: 'user@example.com',
    });
    console.log(`Created commit: ${commit.hash}`);

    // Create a feature branch
    console.log('\nCreating feature branch...');
    await repo.createBranch('feature/add-tests');

    // List branches
    const branches = await repo.listBranches();
    console.log('\nBranches:');
    branches.forEach(branch => {
      const current = branch.is_current ? ' (current)' : '';
      console.log(`  - ${branch.name}${current}`);
    });

    // Watch for events (for 5 seconds)
    console.log('\nWatching for events for 5 seconds...');
    const subscription = repo.watch().subscribe(
      (event) => {
        console.log(`Event: ${event.type} - ${JSON.stringify(event.data)}`);
      },
      (error) => {
        console.error('Event error:', error);
      }
    );

    // Create some events
    setTimeout(async () => {
      await repo.checkout('feature/add-tests');
      await repo.addFile('test.js', 'console.log("Test file");');
      await repo.commit('Add test file');
    }, 1000);

    // Stop watching after 5 seconds
    setTimeout(() => {
      subscription.unsubscribe();
      console.log('\nStopped watching events');
      
      // Cleanup
      client.deleteRepo('example-repo-js')
        .then(() => console.log('Repository cleaned up'))
        .catch(err => console.error('Cleanup failed:', err));
    }, 5000);

  } catch (error) {
    console.error('Error:', error.message);
    if (error.code) {
      console.error('Error code:', error.code);
    }
  }
}

// Run the example
main().catch(console.error);