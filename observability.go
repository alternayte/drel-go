package drel

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Span is a minimal tracing span. It is satisfied by a thin adapter over any
// tracing library (e.g. OpenTelemetry), keeping drel free of tracing
// dependencies. End is called when the traced operation completes; RecordError
// attaches an error to the span (a nil error is ignored).
type Span interface {
	End()
	RecordError(err error)
}

// Tracer starts spans for drel operations. Provide one via WithTracer to emit a
// span per executed query. A nil Tracer disables tracing.
type Tracer interface {
	// Start begins a span named name and returns a context carrying it.
	Start(ctx context.Context, name string) (context.Context, Span)
}

// WithLogger attaches a structured logger. Combined with WithQueryLog,
// WithSlowQueryThreshold, or WithDevMode it emits query and diagnostic logs.
func WithLogger(logger *slog.Logger) Option {
	return func(cfg *engineConfig) { cfg.logger = logger }
}

// WithQueryLog logs every executed query (SQL, arg count, duration) at debug
// level. Requires a logger (WithLogger); if none is set, slog.Default is used.
func WithQueryLog(enabled bool) Option {
	return func(cfg *engineConfig) { cfg.queryLog = enabled }
}

// WithSlowQueryThreshold logs a warning for any query whose execution exceeds d.
func WithSlowQueryThreshold(d time.Duration) Option {
	return func(cfg *engineConfig) { cfg.slowThreshold = d }
}

// WithTracer emits an OpenTelemetry-style span per executed query.
func WithTracer(tracer Tracer) Option {
	return func(cfg *engineConfig) { cfg.tracer = tracer }
}

// WithDevMode enables development-time diagnostics: warnings for unbounded
// queries (SELECT without LIMIT), likely N+1 access patterns (the same query
// shape executed repeatedly in a short window), slow queries, and tracked
// entities that were loaded but never modified. Do not enable in production.
func WithDevMode() Option {
	return func(cfg *engineConfig) { cfg.devMode = true }
}

// installObservability wires the configured logging/tracing/dev-mode behavior
// onto the engine via the query-hook mechanism.
func (e *Engine) installObservability(cfg *engineConfig) {
	e.logger = cfg.logger
	e.tracer = cfg.tracer
	e.devMode = cfg.devMode
	e.slowThreshold = cfg.slowThreshold

	if e.devMode {
		if e.logger == nil {
			e.logger = slog.Default()
		}
		// Dev mode implies a default slow-query threshold if none was set.
		if e.slowThreshold == 0 {
			e.slowThreshold = 200 * time.Millisecond
		}
		e.n1 = newN1Detector()
	}

	queryLog := cfg.queryLog
	if queryLog && e.logger == nil {
		e.logger = slog.Default()
	}

	if e.logger == nil && !e.devMode {
		return // nothing to observe
	}

	e.OnQuery(func(ctx context.Context, ev QueryEvent) {
		log := e.logger
		if log == nil {
			return
		}
		// Never observe our own EXPLAIN probes (prevents recursion/noise).
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(ev.SQL)), "EXPLAIN") {
			return
		}
		if ev.Err != nil {
			log.ErrorContext(ctx, "drel query error",
				"sql", ev.SQL, "args", len(ev.Args), "duration", ev.Duration, "err", ev.Err)
			return
		}
		if queryLog {
			log.DebugContext(ctx, "drel query",
				"sql", ev.SQL, "args", len(ev.Args), "duration", ev.Duration)
		}
		if e.slowThreshold > 0 && ev.Duration >= e.slowThreshold {
			log.WarnContext(ctx, "drel slow query",
				"sql", ev.SQL, "duration", ev.Duration, "threshold", e.slowThreshold)
		}
		if e.devMode {
			if isUnboundedSelect(ev.SQL) {
				log.WarnContext(ctx, "drel dev: unbounded query (SELECT without LIMIT)", "sql", ev.SQL)
			}
			if e.n1 != nil && e.n1.observe(ev.SQL) {
				log.WarnContext(ctx, "drel dev: possible N+1 — same query shape executed repeatedly",
					"sql", ev.SQL)
			}
			// Missing-index hint: for slow SELECTs on dialects that support it,
			// inspect the plan for a sequential scan.
			if e.slowThreshold > 0 && ev.Duration >= e.slowThreshold && isSelect(ev.SQL) {
				e.checkMissingIndex(ctx, ev.SQL, ev.Args)
			}
		}
	})
}

// devWarn logs a development-mode warning when dev mode is enabled.
func (e *Engine) devWarn(ctx context.Context, msg string, args ...any) {
	if e.devMode && e.logger != nil {
		e.logger.WarnContext(ctx, msg, args...)
	}
}

// isSelect reports whether sql is a SELECT statement.
func isSelect(sql string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT ")
}

// checkMissingIndex runs the dialect's EXPLAIN for a slow query and warns if the
// plan contains a sequential scan. Best-effort: any error is logged at debug and
// otherwise ignored. Only dialects that support plan inspection (Postgres) act.
func (e *Engine) checkMissingIndex(ctx context.Context, sql string, args []any) {
	explainSQL, ok := e.dia.Explain(sql)
	if !ok {
		return
	}
	rows, err := e.drv.Query(ctx, explainSQL, args...)
	if err != nil {
		e.logger.DebugContext(ctx, "drel dev: EXPLAIN failed", "err", err)
		return
	}
	defer rows.Close()
	seqScan := false
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return
		}
		if strings.Contains(line, "Seq Scan") {
			seqScan = true
		}
	}
	if seqScan {
		e.logger.WarnContext(ctx, "drel dev: slow query uses a sequential scan — consider adding an index", "sql", sql)
	}
}

// isUnboundedSelect reports whether sql is a row-returning SELECT with no LIMIT.
// COUNT/EXISTS aggregates and non-SELECT statements are never flagged.
func isUnboundedSelect(sql string) bool {
	u := strings.ToUpper(strings.TrimSpace(sql))
	if !strings.HasPrefix(u, "SELECT ") {
		return false
	}
	if strings.Contains(u, " LIMIT ") || strings.HasSuffix(u, ")") {
		// Trailing ")" catches the EXISTS(...) wrapper form.
		return false
	}
	if strings.HasPrefix(u, "SELECT COUNT(") || strings.Contains(u, "SELECT EXISTS(") {
		return false
	}
	return true
}

// n1Detector flags query shapes executed many times within a short window, a
// hallmark of N+1 access. It is safe for concurrent use.
type n1Detector struct {
	mu        sync.Mutex
	counts    map[string]*n1Entry
	threshold int
	window    time.Duration
	now       func() time.Time
}

type n1Entry struct {
	count  int
	first  time.Time
	warned bool
}

func newN1Detector() *n1Detector {
	return &n1Detector{
		counts:    make(map[string]*n1Entry),
		threshold: 10,
		window:    1 * time.Second,
		now:       time.Now,
	}
}

// observe records an execution of sql and returns true exactly once when the
// shape crosses the threshold within the window.
func (d *n1Detector) observe(sql string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	e, ok := d.counts[sql]
	if !ok || now.Sub(e.first) > d.window {
		d.counts[sql] = &n1Entry{count: 1, first: now}
		return false
	}
	e.count++
	if e.count >= d.threshold && !e.warned {
		e.warned = true
		return true
	}
	return false
}
