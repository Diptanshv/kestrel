package httpapi

import (
	"log/slog"

	"github.com/Diptanshv/kestrel/internal/store"
)

type API struct {
	Log *slog.Logger
	Q   *store.Queries
}
