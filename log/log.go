package log

import (
	"context"
	"os"

	"github.com/Sirupsen/logrus"
)

// WithContext returns a logger that has global and context fields set on it.
func WithContext(ctx context.Context) logrus.FieldLogger {
	return logrus.WithField("pid", os.Getpid())
}
