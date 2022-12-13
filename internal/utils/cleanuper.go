package utils

type Cleanup struct {
	cleanupErr func() error
	cleanup    func()
}

func NewCleanupErr(cl func() error) *Cleanup {
	return &Cleanup{cleanupErr: cl}
}

func NewCleanup(cl func()) *Cleanup {
	return &Cleanup{cleanup: cl}
}

func (c *Cleanup) Disarm() {
	c.cleanupErr = nil
	c.cleanup = nil
}

func (c *Cleanup) Cleanup() {
	if c.cleanupErr != nil {
		_ = c.cleanupErr()
	}

	if c.cleanup != nil {
		c.cleanup()
	}
}
