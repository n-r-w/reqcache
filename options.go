package reqcache

// Option is a function for configuring ReqCache.
type Option func(*options)

type options struct {
	name   string
	logger ILogger
}

// WithLogger sets a logger for displaying/metrics new object pool overflows.
// By default, the logger is nil.
func WithLogger(name string, logger ILogger) Option {
	return func(c *options) {
		c.name = name
		c.logger = logger
	}
}
