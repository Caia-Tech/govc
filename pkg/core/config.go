package core

// Config manages repository configuration
type Config struct {
	store ConfigStore
}

// NewConfig creates a new config manager
func NewConfig(store ConfigStore) *Config {
	return &Config{store: store}
}

// Get retrieves a configuration value
func (c *Config) Get(key string) (string, error) {
	return c.store.Get(key)
}

// Set sets a configuration value
func (c *Config) Set(key, value string) error {
	return c.store.Set(key, value)
}

// Delete removes a configuration value
func (c *Config) Delete(key string) error {
	return c.store.Delete(key)
}

// List returns all configuration values
func (c *Config) List() (map[string]string, error) {
	return c.store.List()
}