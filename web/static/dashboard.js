// Dashboard JavaScript
class Dashboard {
    constructor() {
        this.ws = null;
        this.charts = {};
        this.currentSection = 'overview';
        this.refreshInterval = null;
        this.csrfToken = null;
        this.init();
    }

    async init() {
        await this.fetchCSRFToken();
        this.setupEventListeners();
        this.setupWebSocket();
        this.setupCharts();
        this.startAutoRefresh();
        this.loadInitialData();
    }

    async fetchCSRFToken() {
        try {
            const response = await fetch('/api/v1/auth/csrf-token');
            if (response.ok) {
                const data = await response.json();
                this.csrfToken = data.csrf_token;
            }
        } catch (error) {
            console.error('Failed to fetch CSRF token:', error);
        }
    }

    setupEventListeners() {
        // Navigation
        document.querySelectorAll('.nav-link').forEach(link => {
            link.addEventListener('click', (e) => {
                e.preventDefault();
                const section = link.dataset.section;
                this.showSection(section);
            });
        });

        // User menu
        const userMenuBtn = document.getElementById('user-menu-btn');
        const userDropdown = document.getElementById('user-dropdown');
        if (userMenuBtn && userDropdown) {
            userMenuBtn.addEventListener('click', () => {
                userDropdown.classList.toggle('show');
            });
        }

        // Close user menu when clicking outside
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.user-menu')) {
                const dropdown = document.getElementById('user-dropdown');
                if (dropdown) {
                    dropdown.classList.remove('show');
                }
            }
        });

        // Refresh button
        const refreshBtn = document.getElementById('refresh-btn');
        if (refreshBtn) {
            refreshBtn.addEventListener('click', () => {
                this.refreshData();
            });
        }

        // Create repository modal
        const createRepoBtn = document.getElementById('create-repo-btn');
        const modalOverlay = document.getElementById('modal-overlay');
        const modalClose = document.getElementById('modal-close');
        const cancelCreate = document.getElementById('cancel-create');

        if (createRepoBtn && modalOverlay) {
            createRepoBtn.addEventListener('click', () => {
                modalOverlay.classList.add('show');
            });
        }

        if (modalClose) {
            modalClose.addEventListener('click', () => {
                modalOverlay.classList.remove('show');
            });
        }

        if (cancelCreate) {
            cancelCreate.addEventListener('click', () => {
                modalOverlay.classList.remove('show');
            });
        }

        // Close modal when clicking overlay
        if (modalOverlay) {
            modalOverlay.addEventListener('click', (e) => {
                if (e.target === modalOverlay) {
                    modalOverlay.classList.remove('show');
                }
            });
        }

        // Create repository form
        const createRepoForm = document.getElementById('create-repo-form');
        if (createRepoForm) {
            createRepoForm.addEventListener('submit', (e) => {
                e.preventDefault();
                this.createRepository();
            });
        }

        // Memory-only checkbox
        const memoryOnlyCheckbox = document.getElementById('memory-only');
        const repoPathInput = document.getElementById('repo-path');
        if (memoryOnlyCheckbox && repoPathInput) {
            memoryOnlyCheckbox.addEventListener('change', (e) => {
                if (e.target.checked) {
                    repoPathInput.value = ':memory:';
                    repoPathInput.disabled = true;
                } else {
                    repoPathInput.value = '';
                    repoPathInput.disabled = false;
                }
            });
        }

        // Repository search
        const repoSearch = document.getElementById('repo-search');
        if (repoSearch) {
            repoSearch.addEventListener('input', (e) => {
                this.filterRepositories(e.target.value);
            });
        }

        // Repository status filter
        const repoStatusFilter = document.getElementById('repo-status-filter');
        if (repoStatusFilter) {
            repoStatusFilter.addEventListener('change', (e) => {
                this.filterRepositoriesByStatus(e.target.value);
            });
        }

        // Git operations
        document.getElementById('commit-btn')?.addEventListener('click', () => this.showGitOperation('commit'));
        document.getElementById('branch-btn')?.addEventListener('click', () => this.showGitOperation('branch'));
        document.getElementById('merge-btn')?.addEventListener('click', () => this.showGitOperation('merge'));
        document.getElementById('revert-btn')?.addEventListener('click', () => this.showGitOperation('revert'));

        // Auth operations
        document.getElementById('generate-token-btn')?.addEventListener('click', () => this.generateToken());
        document.getElementById('validate-token-btn')?.addEventListener('click', () => this.validateToken());

        // Log controls
        const logLevelFilter = document.getElementById('log-level-filter');
        if (logLevelFilter) {
            logLevelFilter.addEventListener('change', (e) => {
                this.filterLogs(e.target.value);
            });
        }

        const clearLogsBtn = document.getElementById('clear-logs-btn');
        if (clearLogsBtn) {
            clearLogsBtn.addEventListener('click', () => {
                this.clearLogs();
            });
        }

        // Logout
        document.getElementById('logout-btn')?.addEventListener('click', (e) => {
            e.preventDefault();
            this.logout();
        });
    }

    setupWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;
        
        this.ws = new WebSocket(wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected');
            this.updateConnectionStatus(true);
        };
        
        this.ws.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.handleWebSocketMessage(data);
            } catch (error) {
                console.error('Error parsing WebSocket message:', error);
            }
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.updateConnectionStatus(false);
            // Attempt to reconnect after 3 seconds
            setTimeout(() => this.setupWebSocket(), 3000);
        };
        
        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.updateConnectionStatus(false);
        };
    }

    setupCharts() {
        // Performance chart
        const performanceCtx = document.getElementById('performance-chart');
        if (performanceCtx) {
            this.charts.performance = new Chart(performanceCtx, {
                type: 'line',
                data: {
                    labels: [],
                    datasets: [{
                        label: 'Response Time (ms)',
                        data: [],
                        borderColor: '#007bff',
                        backgroundColor: 'rgba(0, 123, 255, 0.1)',
                        tension: 0.4
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    scales: {
                        y: {
                            beginAtZero: true
                        }
                    }
                }
            });
        }

        // Activity chart
        const activityCtx = document.getElementById('activity-chart');
        if (activityCtx) {
            this.charts.activity = new Chart(activityCtx, {
                type: 'doughnut',
                data: {
                    labels: ['Active', 'Idle'],
                    datasets: [{
                        data: [0, 0],
                        backgroundColor: ['#28a745', '#6c757d']
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false
                }
            });
        }
    }

    startAutoRefresh() {
        this.refreshInterval = setInterval(() => {
            this.refreshData();
        }, 5000); // Refresh every 5 seconds
    }

    loadInitialData() {
        this.refreshData();
        this.loadRepositories();
        this.loadPerformanceMetrics();
        this.loadAuthInfo();
        this.loadLogs();
    }

    async refreshData() {
        try {
            const response = await fetch('/api/v1/dashboard/overview');
            if (response.ok) {
                const data = await response.json();
                this.updateOverviewMetrics(data);
            }
        } catch (error) {
            console.error('Error refreshing data:', error);
            this.addActivityItem('error', 'Failed to refresh dashboard data');
        }
    }

    async loadRepositories() {
        try {
            const response = await fetch('/api/v1/repos');
            if (response.ok) {
                const data = await response.json();
                // Main API returns {count: N, repositories: [...]}
                const repositories = data.repositories || [];
                this.updateRepositoriesTable(repositories);
            }
        } catch (error) {
            console.error('Error loading repositories:', error);
        }
    }

    async loadPerformanceMetrics() {
        try {
            const response = await fetch('/api/v1/dashboard/performance');
            if (response.ok) {
                const data = await response.json();
                this.updatePerformanceMetrics(data);
            }
        } catch (error) {
            console.error('Error loading performance metrics:', error);
        }
    }

    async loadAuthInfo() {
        try {
            const response = await fetch('/api/v1/dashboard/auth/info');
            if (response.ok) {
                const data = await response.json();
                this.updateAuthInfo(data);
            }
        } catch (error) {
            console.error('Error loading auth info:', error);
        }
    }

    async loadLogs() {
        try {
            const response = await fetch('/api/v1/dashboard/logs?limit=100');
            if (response.ok) {
                const logs = await response.json();
                this.updateLogsDisplay(logs);
            }
        } catch (error) {
            console.error('Error loading logs:', error);
        }
    }

    showSection(sectionName) {
        // Update navigation
        document.querySelectorAll('.nav-item').forEach(item => {
            item.classList.remove('active');
        });
        document.querySelector(`[data-section="${sectionName}"]`).parentElement.classList.add('active');

        // Update content
        document.querySelectorAll('.content-section').forEach(section => {
            section.classList.remove('active');
        });
        document.getElementById(sectionName).classList.add('active');

        this.currentSection = sectionName;

        // Load section-specific data
        switch (sectionName) {
            case 'repositories':
                this.loadRepositories();
                break;
            case 'performance':
                this.loadPerformanceMetrics();
                break;
            case 'auth':
                this.loadAuthInfo();
                break;
            case 'logs':
                this.loadLogs();
                break;
        }
    }

    updateConnectionStatus(connected) {
        const indicator = document.getElementById('status-indicator');
        const text = document.getElementById('status-text');
        
        if (indicator && text) {
            if (connected) {
                indicator.className = 'status-indicator connected';
                text.textContent = 'Connected';
            } else {
                indicator.className = 'status-indicator disconnected';
                text.textContent = 'Disconnected';
            }
        }
    }

    updateOverviewMetrics(data) {
        document.getElementById('total-repos').textContent = data.totalRepositories || 0;
        document.getElementById('active-repos').textContent = data.activeRepositories || 0;
        document.getElementById('avg-response').textContent = `${data.averageResponseTime || 0}ms`;
        document.getElementById('memory-usage').textContent = `${data.memoryUsage || 0}MB`;

        // Update charts
        if (this.charts.performance && data.performanceHistory) {
            const chart = this.charts.performance;
            chart.data.labels = data.performanceHistory.labels;
            chart.data.datasets[0].data = data.performanceHistory.values;
            chart.update();
        }

        if (this.charts.activity && data.repositoryStats) {
            const chart = this.charts.activity;
            chart.data.datasets[0].data = [
                data.repositoryStats.active || 0,
                data.repositoryStats.idle || 0
            ];
            chart.update();
        }
    }

    updateRepositoriesTable(repositories) {
        const tbody = document.getElementById('repositories-tbody');
        if (!tbody) return;

        tbody.innerHTML = '';

        repositories.forEach(repo => {
            const row = document.createElement('tr');
            row.innerHTML = `
                <td>${repo.id}</td>
                <td>${repo.path}</td>
                <td><span class="status-badge ${repo.status}">${repo.status}</span></td>
                <td>${new Date(repo.lastAccessed).toLocaleString()}</td>
                <td>${repo.accessCount}</td>
                <td>
                    <button class="btn btn-sm btn-secondary" onclick="dashboard.viewRepository('${repo.id}')">
                        <i class="fas fa-eye"></i>
                    </button>
                    <button class="btn btn-sm btn-danger" onclick="dashboard.deleteRepository('${repo.id}')">
                        <i class="fas fa-trash"></i>
                    </button>
                </td>
            `;
            tbody.appendChild(row);
        });
    }

    updatePerformanceMetrics(data) {
        // Update pool stats
        document.getElementById('pool-total').textContent = data.poolStats?.total || 0;
        document.getElementById('pool-active').textContent = data.poolStats?.active || 0;
        document.getElementById('pool-idle').textContent = data.poolStats?.idle || 0;

        // Update auth stats
        document.getElementById('jwt-rate').textContent = data.authStats?.jwtRate || 0;
        document.getElementById('cache-hit-rate').textContent = `${data.authStats?.cacheHitRate || 0}%`;

        // Update benchmark results
        const benchmarkResults = document.getElementById('benchmark-results');
        if (benchmarkResults && data.benchmarks) {
            benchmarkResults.innerHTML = '';
            data.benchmarks.forEach(benchmark => {
                const div = document.createElement('div');
                div.className = 'benchmark-item';
                div.innerHTML = `
                    <div class="benchmark-name">${benchmark.name}</div>
                    <div class="benchmark-value">${benchmark.value}</div>
                    <div class="benchmark-unit">${benchmark.unit}</div>
                `;
                benchmarkResults.appendChild(div);
            });
        }
    }

    updateAuthInfo(data) {
        const userNameEl = document.getElementById('user-name');
        if (userNameEl && data.user) {
            userNameEl.textContent = data.user.username || 'User';
        }

        const permissionsList = document.getElementById('permissions-list');
        if (permissionsList && data.permissions) {
            permissionsList.innerHTML = '';
            data.permissions.forEach(permission => {
                const div = document.createElement('div');
                div.className = 'permission-item';
                div.textContent = permission;
                permissionsList.appendChild(div);
            });
        }
    }

    updateLogsDisplay(logs) {
        const logEntries = document.getElementById('log-entries');
        if (!logEntries) return;

        logEntries.innerHTML = '';
        logs.forEach(log => {
            const div = document.createElement('div');
            div.className = `log-entry log-${log.level.toLowerCase()}`;
            div.innerHTML = `
                <span class="log-timestamp">${new Date(log.timestamp).toLocaleString()}</span>
                <span class="log-level">[${log.level}]</span>
                <span class="log-message">${log.message}</span>
            `;
            logEntries.appendChild(div);
        });

        // Auto-scroll to bottom
        logEntries.scrollTop = logEntries.scrollHeight;
    }

    async createRepository() {
        const form = document.getElementById('create-repo-form');
        const formData = new FormData(form);
        
        const repoData = {
            id: formData.get('repo-id'),
            path: formData.get('repo-path') || ':memory:',
            memory_only: formData.get('memory-only') === 'on'
        };

        try {
            const headers = {
                'Content-Type': 'application/json'
            };
            
            if (this.csrfToken) {
                headers['X-CSRF-Token'] = this.csrfToken;
            }

            const response = await fetch('/api/v1/repos', {
                method: 'POST',
                headers: headers,
                body: JSON.stringify(repoData)
            });

            if (response.ok) {
                const result = await response.json();
                this.addActivityItem('success', `Repository "${repoData.id}" created successfully`);
                document.getElementById('modal-overlay').classList.remove('show');
                form.reset();
                this.loadRepositories();
            } else {
                const error = await response.text();
                this.addActivityItem('error', `Failed to create repository: ${error}`);
            }
        } catch (error) {
            console.error('Error creating repository:', error);
            this.addActivityItem('error', 'Network error while creating repository');
        }
    }

    async deleteRepository(id) {
        if (!confirm(`Are you sure you want to delete repository "${id}"?`)) {
            return;
        }

        try {
            const headers = {};
            
            if (this.csrfToken) {
                headers['X-CSRF-Token'] = this.csrfToken;
            }

            const response = await fetch(`/api/v1/repos/${id}`, {
                method: 'DELETE',
                headers: headers
            });

            if (response.ok) {
                this.addActivityItem('success', `Repository "${id}" deleted successfully`);
                this.loadRepositories();
            } else {
                const error = await response.text();
                this.addActivityItem('error', `Failed to delete repository: ${error}`);
            }
        } catch (error) {
            console.error('Error deleting repository:', error);
            this.addActivityItem('error', 'Network error while deleting repository');
        }
    }

    viewRepository(id) {
        // Show repository details modal or navigate to repository view
        this.addActivityItem('info', `Viewing repository "${id}"`);
    }

    filterRepositories(searchTerm) {
        const tbody = document.getElementById('repositories-tbody');
        if (!tbody) return;

        const rows = tbody.querySelectorAll('tr');
        rows.forEach(row => {
            const id = row.cells[0].textContent.toLowerCase();
            const path = row.cells[1].textContent.toLowerCase();
            const visible = id.includes(searchTerm.toLowerCase()) || path.includes(searchTerm.toLowerCase());
            row.style.display = visible ? '' : 'none';
        });
    }

    filterRepositoriesByStatus(status) {
        const tbody = document.getElementById('repositories-tbody');
        if (!tbody) return;

        const rows = tbody.querySelectorAll('tr');
        rows.forEach(row => {
            const statusCell = row.cells[2].querySelector('.status-badge');
            if (!status || statusCell.textContent === status) {
                row.style.display = '';
            } else {
                row.style.display = 'none';
            }
        });
    }

    showGitOperation(operation) {
        this.addActivityItem('info', `Git ${operation} operation requested`);
        // Implement specific git operation logic here
    }

    async generateToken() {
        try {
            const headers = {
                'Content-Type': 'application/json'
            };
            
            if (this.csrfToken) {
                headers['X-CSRF-Token'] = this.csrfToken;
            }

            const response = await fetch('/api/v1/dashboard/auth/token/generate', {
                method: 'POST',
                headers: headers,
                body: JSON.stringify({
                    userID: 'dashboard-user',
                    username: 'dashboard',
                    email: 'dashboard@localhost',
                    permissions: ['read', 'write']
                })
            });

            if (response.ok) {
                const result = await response.json();
                const tokenDisplay = document.getElementById('token-display');
                if (tokenDisplay) {
                    tokenDisplay.textContent = result.token;
                }
                this.addActivityItem('success', 'JWT token generated successfully');
            } else {
                this.addActivityItem('error', 'Failed to generate JWT token');
            }
        } catch (error) {
            console.error('Error generating token:', error);
            this.addActivityItem('error', 'Network error while generating token');
        }
    }

    async validateToken() {
        const tokenDisplay = document.getElementById('token-display');
        if (!tokenDisplay || !tokenDisplay.textContent) {
            this.addActivityItem('warning', 'No token to validate');
            return;
        }

        try {
            const headers = {
                'Content-Type': 'application/json'
            };
            
            if (this.csrfToken) {
                headers['X-CSRF-Token'] = this.csrfToken;
            }

            const response = await fetch('/api/v1/dashboard/auth/token/validate', {
                method: 'POST',
                headers: headers,
                body: JSON.stringify({
                    token: tokenDisplay.textContent
                })
            });

            if (response.ok) {
                const result = await response.json();
                this.addActivityItem('success', `Token is valid. User: ${result.username}`);
            } else {
                this.addActivityItem('error', 'Token validation failed');
            }
        } catch (error) {
            console.error('Error validating token:', error);
            this.addActivityItem('error', 'Network error while validating token');
        }
    }

    filterLogs(level) {
        const logEntries = document.getElementById('log-entries');
        if (!logEntries) return;

        const entries = logEntries.querySelectorAll('.log-entry');
        entries.forEach(entry => {
            if (!level || entry.classList.contains(`log-${level.toLowerCase()}`)) {
                entry.style.display = '';
            } else {
                entry.style.display = 'none';
            }
        });
    }

    clearLogs() {
        const logEntries = document.getElementById('log-entries');
        if (logEntries) {
            logEntries.innerHTML = '';
        }
        this.addActivityItem('info', 'Logs cleared');
    }

    logout() {
        // Implement logout logic
        this.addActivityItem('info', 'Logging out...');
        // Redirect to login page or clear auth token
    }

    addActivityItem(type, message) {
        const activityList = document.getElementById('activity-list');
        if (!activityList) return;

        const item = document.createElement('div');
        item.className = 'activity-item';
        item.innerHTML = `
            <i class="fas fa-${this.getActivityIcon(type)} activity-icon ${type}"></i>
            <div class="activity-content">
                <p class="activity-message">${message}</p>
                <span class="activity-time">${new Date().toLocaleString()}</span>
            </div>
        `;

        activityList.insertBefore(item, activityList.firstChild);

        // Keep only the last 20 items
        while (activityList.children.length > 20) {
            activityList.removeChild(activityList.lastChild);
        }
    }

    getActivityIcon(type) {
        const icons = {
            info: 'info-circle',
            success: 'check-circle',
            warning: 'exclamation-triangle',
            error: 'times-circle'
        };
        return icons[type] || 'info-circle';
    }

    handleWebSocketMessage(data) {
        switch (data.type) {
            case 'activity':
                this.addActivityItem(data.level || 'info', data.message);
                break;
            case 'metrics':
                this.updateOverviewMetrics(data.data);
                break;
            case 'log':
                this.addLogEntry(data);
                break;
            case 'repository_update':
                this.loadRepositories();
                break;
            default:
                console.log('Unknown WebSocket message type:', data.type);
        }
    }

    addLogEntry(logData) {
        const logEntries = document.getElementById('log-entries');
        if (!logEntries) return;

        const div = document.createElement('div');
        div.className = `log-entry log-${logData.level.toLowerCase()}`;
        div.innerHTML = `
            <span class="log-timestamp">${new Date(logData.timestamp).toLocaleString()}</span>
            <span class="log-level">[${logData.level}]</span>
            <span class="log-message">${logData.message}</span>
        `;

        logEntries.appendChild(div);
        logEntries.scrollTop = logEntries.scrollHeight;

        // Keep only the last 500 log entries
        while (logEntries.children.length > 500) {
            logEntries.removeChild(logEntries.firstChild);
        }
    }

    destroy() {
        if (this.ws) {
            this.ws.close();
        }
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
        }
        Object.values(this.charts).forEach(chart => {
            if (chart && typeof chart.destroy === 'function') {
                chart.destroy();
            }
        });
    }
}

// Initialize dashboard when DOM is loaded
let dashboard;
document.addEventListener('DOMContentLoaded', () => {
    dashboard = new Dashboard();
});

// Clean up on page unload
window.addEventListener('beforeunload', () => {
    if (dashboard) {
        dashboard.destroy();
    }
});