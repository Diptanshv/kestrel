// Command seed fills a demo site with synthetic traffic so the dashboard has
// something to show: a daily rhythm, weekday dips, multi-page visits, goals,
// and realistic spreads of pages, sources, countries and devices.
//
//	go run ./cmd/seed -days 30 -visitors 400
//
// It recreates the demo site each time (its events and rollups go with it),
// writes events with COPY, and backfills the site's rollups so history older
// than the raw window survives retention. Other sites are not touched.
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"math"
	mrand "math/rand/v2"
	"os"
	"time"

	"github.com/Diptanshv/kestrel/internal/auth"
	"github.com/Diptanshv/kestrel/internal/batch"
	"github.com/Diptanshv/kestrel/internal/rollup"
	"github.com/Diptanshv/kestrel/internal/store"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "time/tzdata"
)

type weighted[T any] struct {
	v T
	w float64
}

func pick[T any](r *mrand.Rand, opts []weighted[T]) T {
	total := 0.0
	for _, o := range opts {
		total += o.w
	}
	x := r.Float64() * total
	for _, o := range opts {
		if x -= o.w; x < 0 {
			return o.v
		}
	}
	return opts[len(opts)-1].v
}

var (
	entryPages = []weighted[string]{
		{"/", 30}, {"/blog/hyperloglog-in-go", 18}, {"/blog/cookieless-analytics", 14},
		{"/pricing", 8}, {"/docs/getting-started", 8}, {"/blog/batching-postgres-writes", 7},
		{"/docs/tracker", 5}, {"/about", 4}, {"/changelog", 3}, {"/blog/sessions-without-cookies", 3},
	}
	nextPages = []weighted[string]{
		{"/pricing", 25}, {"/docs/getting-started", 20}, {"/", 15}, {"/docs/tracker", 12},
		{"/signup", 10}, {"/blog/hyperloglog-in-go", 8}, {"/about", 5}, {"/changelog", 5},
	}
	referrers = []weighted[string]{
		{"", 40}, {"google.com", 22}, {"news.ycombinator.com", 12}, {"github.com", 8},
		{"twitter.com", 6}, {"reddit.com", 5}, {"duckduckgo.com", 3}, {"linkedin.com", 2}, {"dev.to", 2},
	}
	countries = []weighted[string]{
		{"IN", 30}, {"US", 25}, {"DE", 9}, {"GB", 8}, {"FR", 5}, {"CA", 5}, {"BR", 4},
		{"NL", 3}, {"JP", 3}, {"AU", 3}, {"SG", 2}, {"ES", 2},
	}
	platforms = []weighted[[3]string]{ // browser, os, device
		{[3]string{"Chrome", "Windows", "desktop"}, 22}, {[3]string{"Chrome", "macOS", "desktop"}, 14},
		{[3]string{"Safari", "macOS", "desktop"}, 9}, {[3]string{"Firefox", "Linux", "desktop"}, 7},
		{[3]string{"Edge", "Windows", "desktop"}, 6}, {[3]string{"Firefox", "Windows", "desktop"}, 4},
		{[3]string{"Chrome", "Android", "mobile"}, 17}, {[3]string{"Safari", "iOS", "mobile"}, 15},
		{[3]string{"Safari", "iOS", "tablet"}, 3}, {[3]string{"Chrome", "Android", "tablet"}, 2},
	}
)

// hourWeight is a daily rhythm peaking mid-afternoon IST with a second
// smaller peak for US evenings.
func hourWeight(localHour int) float64 {
	ist := math.Exp(-math.Pow(float64(localHour)-15, 2) / 18)
	us := 0.6 * math.Exp(-math.Pow(float64((localHour+24-7)%24)-0, 2)/8) // ~07:00 IST
	return 0.08 + ist + us
}

func main() {
	var (
		days     = flag.Int("days", 30, "days of history to generate")
		perDay   = flag.Int("visitors", 400, "average visitors per day at the end of the range")
		domain   = flag.String("domain", "demo.kestrel.dev", "demo site domain")
		email    = flag.String("email", "demo@kestrel.dev", "demo account email")
		password = flag.String("password", "", "demo account password (random if empty)")
		slug     = flag.String("slug", "demo", "public share slug for the demo dashboard")
		tz       = flag.String("tz", envOr("TIMEZONE", "Asia/Kolkata"), "time zone for rollups; must match the server")
		seed     = flag.Uint64("seed", 0, "random seed (0 = random)")
	)
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	loc, err := time.LoadLocation(*tz)
	if err != nil {
		log.Fatal(err)
	}
	if *seed == 0 {
		var b [8]byte
		_, _ = rand.Read(b[:])
		*seed = uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
	}
	r := mrand.New(mrand.NewPCG(*seed, 42))

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	q := store.New(db)

	pw := *password
	if pw == "" {
		pw = randomPassword()
	}
	siteID, err := resetDemo(ctx, db, q, *email, pw, *domain, *slug)
	if err != nil {
		log.Fatal(err)
	}

	now := time.Now()
	start := now.AddDate(0, 0, -*days)
	events := generate(r, siteID, start, now, *perDay, loc)
	if err := (batch.CopyWriter{DB: db}).Write(ctx, events); err != nil {
		log.Fatalf("copy events: %v", err)
	}

	job := rollup.New(db, rollup.DefaultConfig(loc), slog.New(slog.DiscardHandler))
	res, err := job.Backfill(ctx, siteID, start)
	if err != nil {
		log.Fatalf("backfill: %v", err)
	}

	fmt.Printf("seeded %s (site %d): %d events over %d days; %d hourly and %d daily rollup rows in %dms\n",
		*domain, siteID, len(events), *days, res.HourlyRows, res.DailyRows, res.DurationMS)
	fmt.Printf("public dashboard: /share/%s\n", *slug)
	if *password == "" {
		fmt.Printf("login: %s / %s\n", *email, pw)
	}
}

// resetDemo recreates the demo account and site. Deleting the site cascades
// to its events and rollups, so reseeding never double-counts.
func resetDemo(ctx context.Context, db *sql.DB, q *store.Queries, email, password, domain, slug string) (int64, error) {
	if _, err := db.ExecContext(ctx, `DELETE FROM sites WHERE domain = $1`, domain); err != nil {
		return 0, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return 0, err
	}
	user, err := q.GetUserByEmail(ctx, email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if user, err = q.CreateUser(ctx, store.CreateUserParams{Email: email, PasswordHash: hash}); err != nil {
			return 0, err
		}
	case err != nil:
		return 0, err
	default:
		if _, err := db.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, user.ID); err != nil {
			return 0, err
		}
	}
	site, err := q.CreateSite(ctx, store.CreateSiteParams{
		UserID: user.ID, Domain: domain, PublicSlug: sql.NullString{String: slug, Valid: true},
	})
	return site.ID, err
}

func generate(r *mrand.Rand, siteID int64, start, end time.Time, perDay int, loc *time.Location) []batch.Event {
	var out []batch.Event
	days := int(end.Sub(start).Hours()/24) + 1
	var hourW [24]float64
	sumW := 0.0
	for h := range 24 {
		hourW[h] = hourWeight(h)
		sumW += hourW[h]
	}

	for d := range days {
		dayStart := rollup.TruncDay(start, loc).AddDate(0, 0, d)
		// Growth towards the end of the range, a weekend dip, and noise.
		growth := 0.55 + 0.45*float64(d)/float64(max(days-1, 1))
		weekday := 1.0
		if wd := dayStart.Weekday(); wd == time.Saturday || wd == time.Sunday {
			weekday = 0.7
		}
		// One launch-day spike from Hacker News, a week before the end.
		spike := 1.0
		if d == days-8 {
			spike = 2.6
		}
		n := int(float64(perDay) * growth * weekday * spike * (0.9 + 0.2*r.Float64()))

		for range n {
			// Pick the hour by the daily rhythm, then a minute inside it.
			x := r.Float64() * sumW
			hour := 0
			for ; hour < 23; hour++ {
				if x -= hourW[hour]; x < 0 {
					break
				}
			}
			ts := dayStart.Add(time.Duration(hour)*time.Hour + time.Duration(r.Int64N(int64(time.Hour))))
			if !ts.After(start) || !ts.Before(end) {
				continue
			}
			out = append(out, visit(r, siteID, ts, end, d == days-8)...)
		}
	}
	return out
}

func visit(r *mrand.Rand, siteID int64, ts, end time.Time, launchDay bool) []batch.Event {
	visitor := make([]byte, 16)
	for i := range visitor {
		visitor[i] = byte(r.Uint32())
	}
	session := r.Int64N(math.MaxInt64-1) + 1
	plat := pick(r, platforms)
	ref := pick(r, referrers)
	if launchDay && r.Float64() < 0.5 {
		ref = "news.ycombinator.com"
	}
	country := pick(r, countries)

	ev := func(name, path string, at time.Time) batch.Event {
		return batch.Event{
			SiteID: siteID, Ts: at, VisitorID: visitor, SessionID: session, Name: name, Pathname: path,
			Referrer: sql.NullString{String: ref, Valid: ref != ""},
			Country:  sql.NullString{String: country, Valid: true},
			Browser:  sql.NullString{String: plat[0], Valid: true},
			Os:       sql.NullString{String: plat[1], Valid: true},
			Device:   sql.NullString{String: plat[2], Valid: true},
		}
	}

	// About 45% of visits bounce; the rest read 2-6 pages.
	pages := 1
	if r.Float64() > 0.45 {
		pages = 2 + r.IntN(5)
	}
	out := []batch.Event{ev("pageview", pick(r, entryPages), ts)}
	at := ts
	for range pages - 1 {
		at = at.Add(time.Duration(15+r.IntN(240)) * time.Second)
		if !at.Before(end) {
			break
		}
		path := pick(r, nextPages)
		out = append(out, ev("pageview", path, at))
		if path == "/signup" && r.Float64() < 0.35 {
			out = append(out, ev("signup", path, at.Add(40*time.Second)))
		}
	}
	if pages > 1 && r.Float64() < 0.06 && at.Add(5*time.Second).Before(end) {
		out = append(out, ev("outbound-github", out[len(out)-1].Pathname, at.Add(5*time.Second)))
	}
	return out
}

func randomPassword() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
