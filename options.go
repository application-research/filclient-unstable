package filclient

type Option func(*Config)

// Replaces the entire config - if used, should always be the first option
func WithConfig(cfg Config) Option {
	return func(oldCfg *Config) {
		*oldCfg = cfg
	}
}
