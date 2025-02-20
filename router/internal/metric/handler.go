package metric

import (
	"fmt"
	"github.com/go-chi/chi/middleware"
	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"net/http"
	"time"
)

// Server HTTP metrics.
const (
	RequestCount          = "router.http.requests"                      // Incoming request count total
	ServerLatency         = "router.http.request.duration_milliseconds" // Incoming end to end duration, milliseconds
	RequestContentLength  = "router.http.request.content_length"        // Incoming request bytes total
	ResponseContentLength = "router.http.response.content_length"       // Outgoing response bytes total
	InFlightRequests      = "router.http.requests.in_flight.count"      // Number of requests in flight
)

type Option func(svr *Handler)

type Handler struct {
	meterProvider *metric.MeterProvider

	counters       map[string]otelmetric.Int64Counter
	valueRecorders map[string]otelmetric.Float64Histogram
	upDownCounters map[string]otelmetric.Int64UpDownCounter

	baseFields         []attribute.KeyValue
	requestAttrHandler func(r *http.Request) []attribute.KeyValue
}

func NewMetricHandler(meterProvider *metric.MeterProvider, opts ...Option) (*Handler, error) {
	h := &Handler{
		meterProvider: meterProvider,
	}

	for _, opt := range opts {
		opt(h)
	}

	if err := h.createMeasures(); err != nil {
		return nil, err
	}

	return h, nil
}

func (h *Handler) createMeasures() error {
	h.counters = make(map[string]otelmetric.Int64Counter)
	h.valueRecorders = make(map[string]otelmetric.Float64Histogram)
	h.upDownCounters = make(map[string]otelmetric.Int64UpDownCounter)

	routerMeter := h.meterProvider.Meter("cosmo.router")
	requestCounter, err := routerMeter.Int64Counter(
		RequestCount,
		otelmetric.WithDescription("Total number of requests"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request counter: %w", err)
	}
	h.counters[RequestCount] = requestCounter

	serverLatencyMeasure, err := routerMeter.Float64Histogram(
		ServerLatency,
		otelmetric.WithDescription("Server latency in milliseconds"),
	)
	if err != nil {
		return fmt.Errorf("failed to create server latency measure: %w", err)
	}
	h.valueRecorders[ServerLatency] = serverLatencyMeasure

	requestContentLengthCounter, err := routerMeter.Int64Counter(
		RequestContentLength,
		otelmetric.WithDescription("Total number of request bytes"),
	)
	if err != nil {
		return fmt.Errorf("failed to create request content length counter: %w", err)
	}
	h.counters[RequestContentLength] = requestContentLengthCounter

	responseContentLengthCounter, err := routerMeter.Int64Counter(
		ResponseContentLength,
		otelmetric.WithDescription("Total number of response bytes"),
	)
	if err != nil {
		return fmt.Errorf("failed to create response content length counter: %w", err)
	}

	h.counters[ResponseContentLength] = responseContentLengthCounter

	inFlightRequestsGauge, err := routerMeter.Int64UpDownCounter(
		InFlightRequests,
		otelmetric.WithDescription("Number of requests in flight"),
	)
	if err != nil {
		return fmt.Errorf("failed to create in flight requests gauge: %w", err)
	}
	h.upDownCounters[InFlightRequests] = inFlightRequestsGauge

	return nil
}

func (h *Handler) Handler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		requestStartTime := time.Now()

		h.upDownCounters[InFlightRequests].Add(r.Context(), 1)
		defer h.upDownCounters[InFlightRequests].Add(r.Context(), -1)

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Process request
		next.ServeHTTP(ww, r)

		ctx := r.Context()

		statusCode := ww.Status()

		var baseKeys []attribute.KeyValue

		baseKeys = append(baseKeys, h.baseFields...)
		baseKeys = append(baseKeys, semconv.HTTPStatusCode(statusCode))
		baseKeys = append(baseKeys, h.requestAttrHandler(r)...)

		baseAttributes := otelmetric.WithAttributes(baseKeys...)

		// Use floating point division here for higher precision (instead of Millisecond method).
		elapsedTime := float64(time.Since(requestStartTime)) / float64(time.Millisecond)
		h.valueRecorders[ServerLatency].Record(ctx, elapsedTime, baseAttributes)

		if r.ContentLength > 0 {
			h.counters[RequestContentLength].Add(ctx, r.ContentLength, baseAttributes)
		}
		h.counters[ResponseContentLength].Add(ctx, int64(ww.BytesWritten()), baseAttributes)

		h.counters[RequestCount].Add(ctx, 1, baseAttributes)
	}

	return http.HandlerFunc(fn)
}

// WithRequestAttributes allows to set a request handler that returns metric attributes for the request
func WithRequestAttributes(handler func(h *http.Request) []attribute.KeyValue) Option {
	return func(h *Handler) {
		h.requestAttrHandler = handler
	}
}

// WithAttributes adds attributes to the base attributes
func WithAttributes(attrs ...attribute.KeyValue) Option {
	return func(h *Handler) {
		h.baseFields = append(h.baseFields, attrs...)
	}
}
