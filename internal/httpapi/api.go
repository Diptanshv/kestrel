package httpapi

import (
	"log/slog"

	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/stats"
	"github.com/Diptanshv/kestrel/internal/store"
)

type API struct {
	Log    *slog.Logger
	Q      *store.Queries
	Config config.Config
	Stats  *stats.Service
}
