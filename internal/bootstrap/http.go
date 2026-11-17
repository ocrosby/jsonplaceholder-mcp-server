package bootstrap

import (
	"context"
	"net"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/jedi-knights/go-platform/httpmw"
	"github.com/jedi-knights/go-platform/httpserver"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"

	"github.com/ocrosby/jsonplaceholder-mcp-server/internal/config"
)

// probePaths are the health endpoints. They are neither traced nor access
// logged, so orchestrator probes do not drown real traffic.
var probePaths = []string{"/livez", "/readyz", "/health"}

// Write-timeout sizing. One MCP call can make several upstream attempts inside
// the SDK (its default is one try plus two retries), each bounded by
// HTTP_TIMEOUT_SECONDS, plus backoff of at most two seconds apiece.
const (
	attemptsPerCall = 3
	writeHeadroom   = 15 * time.Second
)

// WrapHTTP puts the portfolio's standard server chain around next: otelhttp
// outermost (so a server span exists and continues the caller's trace), then
// go-platform's httpmw stack (request id, trace id header, access log, panic
// recovery).
func WrapHTTP(next http.Handler, rt *Runtime) http.Handler {
	stack := httpmw.Stack(rt.Logger, httpmw.WithSkipLogPaths(probePaths...))
	opts := []otelhttp.Option{
		otelhttp.WithFilter(func(r *http.Request) bool { return !slices.Contains(probePaths, r.URL.Path) }),
	}
	if rt.Mode == MetricsOnly {
		// Metrics-only promises no spans. go-platform's sampler is parent-based,
		// so a caller's sampled traceparent would force recording and its
		// default exporter would print the spans to stdout. Extract nothing, so
		// every request is a root span and the never-sample ratio applies.
		opts = append(opts, otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator()))
	}
	return otelhttp.NewHandler(stack(next), rt.ServiceName, opts...)
}

// ServeHTTP runs handler on cfg's address until SIGINT or SIGTERM, then drains
// in-flight requests. go-platform's default 15s write timeout would cut off a
// tool call that is retrying upstream, so it is sized from HTTP_TIMEOUT_SECONDS.
func ServeHTTP(ctx context.Context, cfg config.Config, handler http.Handler) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	writeTimeout := attemptsPerCall*cfg.HTTPTimeout + writeHeadroom
	return httpserver.New(addr, handler, httpserver.WithWriteTimeout(writeTimeout)).Run(ctx)
}
