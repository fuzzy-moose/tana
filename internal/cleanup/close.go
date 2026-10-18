package cleanup

import (
	"io"
	"log/slog"
)

func CloseAndLog(closer io.Closer, logger *slog.Logger) {
	if err := closer.Close(); err != nil {
		logger.Error("close_failed", "error", err)
	}
}
