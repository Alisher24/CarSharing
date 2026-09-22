package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/httpheader"
)

// The two streaming operations, declared by the contract as `text/event-stream`. What a stream
// carries is the signals feature's business; what this layer owes is the handshake the generated
// streaming response writes, the refusal of a caller without a session, and the transport rules
// every response shares.
const (
	publicEventsPath  = "/api/v1/events"
	privateEventsPath = "/api/v1/me/events"
)

// handshake is what the stub fan-out writes, so a test asserts the boundary and the generated
// response around it rather than the frames themselves.
var handshake = []byte("retry: 3000\nevent: ready\ndata: {\"server_time\":\"2026-09-12T07:15:30.000000Z\"}\n\n")

// fixedStreams answers the streaming operations from a fixed handshake, which is the seam that lets
// the served router be exercised without a database to publish signals into.
type fixedStreams struct {
	frames []byte
	fails  bool
}

func (s fixedStreams) Serve(_ context.Context, w io.Writer, _ events.StreamOptions) error {
	if s.fails {
		return events.ErrClientGone
	}
	_, err := w.Write(s.frames)
	return err
}

func (s fixedStreams) ServerTime(context.Context) (time.Time, error) {
	if s.fails {
		return time.Time{}, errCatalogUnreadable
	}
	return time.Date(2026, time.September, 12, 7, 15, 30, 0, time.UTC), nil
}

func TestAServedStreamWritesTheHandshakeAsItIsProduced(t *testing.T) {
	stream := read(testStreamRouter(t, fixedStreams{frames: handshake}), publicEventsPath)
	if stream.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", stream.Code, stream.Body.String())
	}
	if got := stream.Header().Get(httpheader.ContentType); got != streamMediaType {
		t.Fatalf("content type = %q", got)
	}
	if got := stream.Header().Get(cacheControlHeader); got != noStoreCacheControl {
		t.Fatalf("cache control = %q", got)
	}
	if stream.Header().Get(httpheader.RequestID) == "" {
		t.Fatal("the stream states no request identifier")
	}
	if stream.Body.String() != string(handshake) {
		t.Fatalf("body = %q", stream.Body.String())
	}
}

// The private stream is refused before a handler it was not given is reached, because a stream that
// started without a session would have nothing to prove and nothing to filter by.
func TestThePrivateStreamRefusesACallerWithoutASession(t *testing.T) {
	refused := read(testStreamRouter(t, fixedStreams{frames: handshake}), privateEventsPath)
	if refused.Code != http.StatusUnauthorized || !hasErrorCode(refused, "AUTHENTICATION_REQUIRED") {
		t.Fatalf("status = %d: %s", refused.Code, refused.Body.String())
	}
	if refused.Header().Get(httpheader.ContentType) != httpheader.JSON {
		t.Fatal("a refused stream did not answer with the error contract")
	}
}

// A stream that could not be established is answered as a service that is unavailable, not as a
// stream that opened and stayed empty.
func TestAStreamThatCannotBeEstablishedIsReportedAsUnavailable(t *testing.T) {
	response := read(testStreamRouter(t, fixedStreams{fails: true}), publicEventsPath)
	if response.Code != http.StatusServiceUnavailable || !hasErrorCode(response, "SERVICE_UNAVAILABLE") {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

// No operation of the contract takes a body, and a stream is no exception: a request that carries
// one is refused rather than streamed.
func TestAStreamRefusesARequestBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, publicEventsPath, strings.NewReader("{}"))
	response := httptest.NewRecorder()
	testStreamRouter(t, fixedStreams{frames: handshake}).ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity || !hasErrorCode(response, "VALIDATION_FAILED") {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
}

// The router a container health check and the routing tests build holds no signals, so it answers a
// streaming operation as unavailable rather than as an unknown resource: the operation exists in the
// contract, and this process cannot serve it.
func TestARouterWithoutSignalsRefusesTheStreamsItDeclares(t *testing.T) {
	for _, path := range []string{publicEventsPath, privateEventsPath} {
		response := read(testRouter(t, servedapi.ReadyStatus{}), path)
		if response.Code == http.StatusNotFound && hasErrorCode(response, "RESOURCE_NOT_FOUND") {
			t.Errorf("%s is declared implemented but is not routed", path)
		}
	}
	public := read(testRouter(t, servedapi.ReadyStatus{}), publicEventsPath)
	if public.Code != http.StatusServiceUnavailable {
		t.Fatalf("a router without signals answered %d: %s", public.Code, public.Body.String())
	}
}

// testStreamRouter is the served router over a stub fan-out: the same boundary, contract and
// generated responses the process serves, with the signals themselves replaced by a fixture.
func testStreamRouter(t *testing.T, hub EventStream) http.Handler {
	t.Helper()
	handlers, err := newCatalogHandlers(fixedCatalog{
		snapshot: demoSnapshot(),
		zones:    demoZones(),
		tariffs:  demoTariffs(),
	}.asCatalog())
	if err != nil {
		t.Fatal(err)
	}
	served := server{
		health:          health{probe: readyProbe},
		catalogHandlers: handlers,
		streams:         streams{timing: events.DefaultStreamTiming(), hub: hub},
	}
	policy := Policy{AllowedOrigins: map[string]bool{}, Authenticate: refuseCredentials}
	strict := servedapi.NewStrictHandlerWithOptions(served, nil, strictErrorHandlers())
	return servedRouter(servedapi.Handler(strict), policy)
}
