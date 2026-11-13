package httpapi

import (
	"log/slog"

	"github.com/Diptanshv/kestrel/internal/batch"
	"github.com/Diptanshv/kestrel/internal/config"
	"github.com/Diptanshv/kestrel/internal/enrich"
	"github.com/Diptanshv/kestrel/internal/stats"
	"github.com/Diptanshv/kestrel/internal/store"
)

// EventSink accepts enriched events from the ingest handler. It must not
// block; false means the event was dropped.
type EventSink interface {
	Enqueue(ev batch.Event) bool
}

// Deps are the collaborators the router needs. cmd/server builds the real
// ones; tests swap in synchronous or fake pieces.
type Deps struct {
	Log     *slog.Logger
	Queries *store.Queries
	Config  config.Config
	Enrich  *enrich.Enricher
	Sink    EventSink
}

type API struct {
	Log    *slog.Logger
	Q      *store.Queries
	Config config.Config
	Stats  *stats.Service
	Enrich *enrich.Enricher
	Sink   EventSink
	sites  *siteCache
}
