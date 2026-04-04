package command

// LoadAdaptiveRateLimiterConfig loads configuration values from the given config map into cfg.
// Deprecated: use AdaptiveRateLimiterConfig.Load instead.
func LoadAdaptiveRateLimiterConfig(cfg *AdaptiveRateLimiterConfig, cfgmap map[interface{}]interface{}) error {
	return cfg.Load(cfgmap)
}

// NewDefaultAdaptiveRateLimitTrackerConfig creates an AdaptiveRateLimitTrackerConfig from a base
// AdaptiveRateLimiterConfig, filling in the tracker-specific fields with defaults.
func NewDefaultAdaptiveRateLimitTrackerConfig(base AdaptiveRateLimiterConfig) AdaptiveRateLimitTrackerConfig {
	return AdaptiveRateLimitTrackerConfig{
		AdaptiveRateLimiterConfig: base,
		SuccessThreshold:          DefaultAdaptiveRateLimiterSuccessThreshold,
		IncreaseFactor:            DefaultAdaptiveRateLimiterIncreaseFactor,
		DecreaseFactor:            DefaultAdaptiveRateLimiterDecreaseFactor,
		IncreaseCheckInterval:     DefaultAdaptiveRateLimiterIncreaseCheckInterval,
		DecreaseCheckInterval:     DefaultAdaptiveRateLimiterDecreaseCheckInterval,
	}
}
