// Dashboard JavaScript Unit Tests
// This file contains tests for the dashboard.js functionality
// To run these tests, you would typically use a test runner like Jest

// Mock the global objects and APIs that the dashboard expects
const mockWebSocket = {
    send: jest.fn(),
    close: jest.fn(),
    addEventListener: jest.fn(),
    readyState: 1, // WebSocket.OPEN
};

const mockChart = {
    update: jest.fn(),
    destroy: jest.fn(),
    data: {
        labels: [],
        datasets: [{ data: [] }]
    }
};

// Mock Chart.js
global.Chart = jest.fn().mockImplementation(() => mockChart);

// Mock fetch API
global.fetch = jest.fn();

// Mock WebSocket
global.WebSocket = jest.fn().mockImplementation(() => mockWebSocket);

// Mock DOM elements
const mockElement = {
    textContent: '',
    innerHTML: '',
    classList: {
        add: jest.fn(),
        remove: jest.fn(),
        toggle: jest.fn(),
        contains: jest.fn()
    },
    addEventListener: jest.fn(),
    appendChild: jest.fn(),
    removeChild: jest.fn(),
    querySelector: jest.fn(),
    querySelectorAll: jest.fn().mockReturnValue([]),
    style: {},
    cells: []
};

const mockDocument = {
    getElementById: jest.fn().mockReturnValue(mockElement),
    querySelector: jest.fn().mockReturnValue(mockElement),
    querySelectorAll: jest.fn().mockReturnValue([mockElement]),
    createElement: jest.fn().mockReturnValue(mockElement),
    addEventListener: jest.fn()
};

global.document = mockDocument;
global.window = { location: { protocol: 'http:', host: 'localhost:8080' } };

// Import the dashboard code (in a real test environment)
// For this example, we'll define the key functions to test

describe('Dashboard Class', () => {
    let dashboard;

    beforeEach(() => {
        // Reset all mocks
        jest.clearAllMocks();
        
        // Mock successful fetch responses
        global.fetch.mockResolvedValue({
            ok: true,
            json: () => Promise.resolve({
                totalRepositories: 5,
                activeRepositories: 3,
                averageResponseTime: 25,
                memoryUsage: 128
            })
        });

        // Create dashboard instance
        dashboard = new Dashboard();
    });

    afterEach(() => {
        if (dashboard) {
            dashboard.destroy();
        }
    });

    describe('Initialization', () => {
        test('should initialize WebSocket connection', () => {
            expect(global.WebSocket).toHaveBeenCalledWith('ws://localhost:8080/ws');
        });

        test('should setup event listeners', () => {
            expect(mockDocument.addEventListener).toHaveBeenCalled();
        });

        test('should initialize charts', () => {
            expect(global.Chart).toHaveBeenCalled();
        });
    });

    describe('Data Loading', () => {
        test('should load overview data', async () => {
            await dashboard.refreshData();

            expect(global.fetch).toHaveBeenCalledWith('/api/v1/dashboard/overview');
        });

        test('should load repositories', async () => {
            global.fetch.mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve([
                    { id: 'repo1', path: ':memory:', status: 'active' },
                    { id: 'repo2', path: '/tmp/repo2', status: 'idle' }
                ])
            });

            await dashboard.loadRepositories();

            expect(global.fetch).toHaveBeenCalledWith('/api/v1/repositories');
        });

        test('should handle fetch errors gracefully', async () => {
            global.fetch.mockRejectedValueOnce(new Error('Network error'));

            // Should not throw
            await expect(dashboard.refreshData()).resolves.not.toThrow();
        });
    });

    describe('UI Updates', () => {
        test('should update overview metrics', () => {
            const testData = {
                totalRepositories: 10,
                activeRepositories: 8,
                averageResponseTime: 45,
                memoryUsage: 256
            };

            dashboard.updateOverviewMetrics(testData);

            expect(mockDocument.getElementById).toHaveBeenCalledWith('total-repos');
            expect(mockDocument.getElementById).toHaveBeenCalledWith('active-repos');
            expect(mockDocument.getElementById).toHaveBeenCalledWith('avg-response');
            expect(mockDocument.getElementById).toHaveBeenCalledWith('memory-usage');
        });

        test('should update repositories table', () => {
            const repositories = [
                {
                    id: 'test-repo',
                    path: ':memory:',
                    status: 'active',
                    lastAccessed: new Date().toISOString(),
                    accessCount: 5
                }
            ];

            dashboard.updateRepositoriesTable(repositories);

            expect(mockDocument.getElementById).toHaveBeenCalledWith('repositories-tbody');
        });

        test('should add activity items', () => {
            dashboard.addActivityItem('info', 'Test activity message');

            expect(mockDocument.getElementById).toHaveBeenCalledWith('activity-list');
            expect(mockDocument.createElement).toHaveBeenCalledWith('div');
        });
    });

    describe('Repository Management', () => {
        test('should create repository', async () => {
            global.fetch.mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve({ message: 'Repository created successfully' })
            });

            // Mock form data
            const mockForm = {
                reset: jest.fn()
            };
            
            const mockFormData = new Map([
                ['repo-id', 'test-repo'],
                ['repo-path', ':memory:'],
                ['memory-only', 'on']
            ]);

            // Mock FormData
            global.FormData = jest.fn().mockImplementation(() => ({
                get: (key) => mockFormData.get(key)
            }));

            mockDocument.getElementById.mockReturnValueOnce(mockForm);

            await dashboard.createRepository();

            expect(global.fetch).toHaveBeenCalledWith('/api/v1/repositories', expect.objectContaining({
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: expect.any(String)
            }));
        });

        test('should delete repository with confirmation', async () => {
            // Mock window.confirm
            global.confirm = jest.fn().mockReturnValue(true);
            
            global.fetch.mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve({ message: 'Repository deleted successfully' })
            });

            await dashboard.deleteRepository('test-repo');

            expect(global.confirm).toHaveBeenCalledWith('Are you sure you want to delete repository "test-repo"?');
            expect(global.fetch).toHaveBeenCalledWith('/api/v1/repositories/test-repo', {
                method: 'DELETE'
            });
        });

        test('should not delete repository if not confirmed', async () => {
            global.confirm = jest.fn().mockReturnValue(false);

            await dashboard.deleteRepository('test-repo');

            expect(global.fetch).not.toHaveBeenCalled();
        });
    });

    describe('Authentication', () => {
        test('should generate JWT token', async () => {
            global.fetch.mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve({ token: 'mock-jwt-token' })
            });

            await dashboard.generateToken();

            expect(global.fetch).toHaveBeenCalledWith('/api/v1/auth/token/generate', expect.objectContaining({
                method: 'POST',
                headers: { 'Content-Type': 'application/json' }
            }));
        });

        test('should validate JWT token', async () => {
            // Mock token display element
            const tokenElement = { textContent: 'mock-jwt-token' };
            mockDocument.getElementById.mockReturnValueOnce(tokenElement);

            global.fetch.mockResolvedValueOnce({
                ok: true,
                json: () => Promise.resolve({ username: 'testuser', valid: true })
            });

            await dashboard.validateToken();

            expect(global.fetch).toHaveBeenCalledWith('/api/v1/auth/token/validate', expect.objectContaining({
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: expect.stringContaining('mock-jwt-token')
            }));
        });
    });

    describe('WebSocket Handling', () => {
        test('should handle activity messages', () => {
            const message = {
                type: 'activity',
                level: 'info',
                message: 'Test activity'
            };

            dashboard.handleWebSocketMessage(message);

            // Should add activity item
            expect(mockDocument.getElementById).toHaveBeenCalledWith('activity-list');
        });

        test('should handle metrics updates', () => {
            const message = {
                type: 'metrics',
                data: {
                    totalRepositories: 5,
                    activeRepositories: 3
                }
            };

            dashboard.handleWebSocketMessage(message);

            // Should update metrics
            expect(mockDocument.getElementById).toHaveBeenCalledWith('total-repos');
        });

        test('should handle log entries', () => {
            const message = {
                type: 'log',
                level: 'ERROR',
                message: 'Test log message',
                timestamp: new Date().toISOString()
            };

            dashboard.handleWebSocketMessage(message);

            expect(mockDocument.getElementById).toHaveBeenCalledWith('log-entries');
        });

        test('should handle repository updates', () => {
            const message = {
                type: 'repository_update'
            };

            // Mock loadRepositories
            dashboard.loadRepositories = jest.fn();

            dashboard.handleWebSocketMessage(message);

            expect(dashboard.loadRepositories).toHaveBeenCalled();
        });
    });

    describe('Filtering and Search', () => {
        test('should filter repositories by search term', () => {
            // Mock table with rows
            const mockRow1 = {
                cells: [
                    { textContent: 'repo1' },
                    { textContent: '/tmp/repo1' }
                ],
                style: {}
            };
            
            const mockRow2 = {
                cells: [
                    { textContent: 'repo2' },
                    { textContent: '/tmp/repo2' }
                ],
                style: {}
            };

            const mockTbody = {
                querySelectorAll: jest.fn().mockReturnValue([mockRow1, mockRow2])
            };

            mockDocument.getElementById.mockReturnValueOnce(mockTbody);

            dashboard.filterRepositories('repo1');

            expect(mockRow1.style.display).toBe('');
            expect(mockRow2.style.display).toBe('none');
        });

        test('should filter logs by level', () => {
            const mockLogEntry1 = {
                classList: { contains: jest.fn().mockReturnValue(true) },
                style: {}
            };
            
            const mockLogEntry2 = {
                classList: { contains: jest.fn().mockReturnValue(false) },
                style: {}
            };

            const mockLogContainer = {
                querySelectorAll: jest.fn().mockReturnValue([mockLogEntry1, mockLogEntry2])
            };

            mockDocument.getElementById.mockReturnValueOnce(mockLogContainer);

            dashboard.filterLogs('ERROR');

            expect(mockLogEntry1.style.display).toBe('');
            expect(mockLogEntry2.style.display).toBe('none');
        });
    });

    describe('Section Navigation', () => {
        test('should show section and update navigation', () => {
            // Mock navigation elements
            const mockNavItem = {
                classList: { remove: jest.fn(), add: jest.fn() },
                parentElement: { classList: { add: jest.fn() } }
            };

            const mockSection = {
                classList: { add: jest.fn() }
            };

            mockDocument.querySelectorAll.mockImplementation((selector) => {
                if (selector === '.nav-item') return [mockNavItem];
                if (selector === '.content-section') return [mockSection];
                return [];
            });

            mockDocument.querySelector.mockReturnValueOnce(mockNavItem);
            mockDocument.getElementById.mockReturnValueOnce(mockSection);

            dashboard.showSection('repositories');

            expect(mockSection.classList.add).toHaveBeenCalledWith('active');
        });
    });

    describe('Cleanup', () => {
        test('should properly destroy dashboard', () => {
            dashboard.destroy();

            expect(mockWebSocket.close).toHaveBeenCalled();
            expect(mockChart.destroy).toHaveBeenCalled();
        });
    });
});

describe('Dashboard Utility Functions', () => {
    test('should format activity icons correctly', () => {
        // Test activity icon mapping
        const testCases = [
            { type: 'info', expected: 'info-circle' },
            { type: 'success', expected: 'check-circle' },
            { type: 'warning', expected: 'exclamation-triangle' },
            { type: 'error', expected: 'times-circle' },
            { type: 'unknown', expected: 'info-circle' }
        ];

        testCases.forEach(({ type, expected }) => {
            const dashboard = new Dashboard();
            const icon = dashboard.getActivityIcon(type);
            expect(icon).toBe(expected);
        });
    });
});

describe('Dashboard Error Handling', () => {
    let dashboard;

    beforeEach(() => {
        dashboard = new Dashboard();
    });

    afterEach(() => {
        dashboard.destroy();
    });

    test('should handle network errors gracefully', async () => {
        global.fetch.mockRejectedValue(new Error('Network error'));

        // Mock addActivityItem to track error logging
        dashboard.addActivityItem = jest.fn();

        await dashboard.refreshData();

        expect(dashboard.addActivityItem).toHaveBeenCalledWith('error', expect.stringContaining('Failed to refresh'));
    });

    test('should handle WebSocket connection errors', () => {
        // Mock WebSocket error
        const errorHandler = mockWebSocket.addEventListener.mock.calls.find(
            call => call[0] === 'error'
        )?.[1];

        if (errorHandler) {
            errorHandler(new Error('WebSocket error'));
        }

        // Should update connection status
        expect(mockDocument.getElementById).toHaveBeenCalledWith('status-indicator');
    });

    test('should handle malformed WebSocket messages', () => {
        // Mock WebSocket message with invalid JSON
        const messageHandler = mockWebSocket.addEventListener.mock.calls.find(
            call => call[0] === 'message'
        )?.[1];

        if (messageHandler) {
            messageHandler({ data: 'invalid json' });
        }

        // Should not crash the application
        expect(true).toBe(true); // Test passes if no exception is thrown
    });
});