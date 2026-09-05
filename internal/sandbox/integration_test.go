//go:build integration

package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sillypoise/p3-relay/internal/delivery"
	"github.com/sillypoise/p3-relay/internal/postgres"
)

// This test requires an empty, disposable database named p3_relay_test, never Railway.
func TestSandboxIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database := os.Getenv("RELAY_TEST_DATABASE_URL")
	if database == "" {
		t.Fatal("RELAY_TEST_DATABASE_URL required")
	}
	pool, err := pgxpool.New(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var name string
	var exists bool
	if err = pool.QueryRow(ctx, `SELECT current_database(),to_regclass('p3_relay.events') IS NOT NULL`).Scan(&name, &exists); err != nil {
		t.Fatal(err)
	}
	if name != "p3_relay_test" || exists {
		t.Fatal("requires empty disposable p3_relay_test database")
	}
	for _, path := range []string{"../../migrations/001_initial.sql", "../../migrations/002_sandbox.sql"} {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	store := &Store{Pool: pool}
	reads := postgres.NewStore(pool)
	key := []byte(strings.Repeat("k", 32))
	h := &Handler{Store: store, Reads: reads, Key: key, Origin: "https://relay.test"}
	// Verify cookie round-trip through TLS, not just manually injected test cookies.
	tlsServer := httptest.NewTLSServer(h)
	h.Origin = tlsServer.URL
	client := tlsServer.Client()
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(ctx, "POST", tlsServer.URL+"/v1/sandbox/session", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", tlsServer.URL)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 201 {
		t.Fatalf("TLS session creation: %d", response.StatusCode)
	}
	if _, err = io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}
	if err = response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	request, err = http.NewRequestWithContext(ctx, "GET", tlsServer.URL+"/v1/sandbox/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("TLS cookie authorization: %d", response.StatusCode)
	}
	if err = response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	tlsServer.Close()
	h.Origin = "https://relay.test"
	cookieA, idA, err := Issue(key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cookieB, idB, err := Issue(key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for id, expiry := range map[string]time.Time{idA: cookieA.Expires, idB: cookieB.Expires} {
		if err = store.Create(ctx, id, expiry); err != nil {
			t.Fatal(err)
		}
	}
	call := func(method, path, body string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	assertStatus := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("status %d want %d: %s", w.Code, status, w.Body)
		}
	}
	assertStatus(call("POST", "/v1/sandbox/events", `{"scenario":"success"}`, &cookieA, "https://evil.test"), 403)
	assertStatus(call("POST", "/v1/sandbox/events", `{"scenario":"success","url":"http://169.254.169.254"}`, &cookieA, h.Origin), 400)
	assertStatus(call("GET", "/v1/sandbox/events", "", nil, ""), 401)
	w := call("POST", "/v1/sandbox/events", `{"scenario":"permanent_failure"}`, &cookieA, h.Origin)
	assertStatus(w, 202)
	var receipt struct {
		ID string `json:"event_id"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("GET", "/v1/sandbox/events/"+receipt.ID, "", &cookieB, ""), 404)
	assertStatus(call("POST", "/v1/sandbox/events/"+receipt.ID+"/replay", "{}", &cookieB, h.Origin), 404)
	assertStatus(call("POST", "/v1/sandbox/events/"+receipt.ID+"/replay", "{}", &cookieA, h.Origin), 409)
	sender := delivery.NewHTTPSender(&http.Client{Timeout: time.Second}, []byte(strings.Repeat("s", 32)), false)
	worker := delivery.NewWorker(reads, sender, 30*time.Second)
	for generation := 0; generation < 3; generation++ {
		processed, err := worker.RunOnce(ctx, time.Now())
		if err != nil || !processed {
			t.Fatalf("worker: %v", err)
		}
		detail, err := reads.Detail(ctx, receipt.ID, "sandbox:"+idA)
		if err != nil {
			t.Fatal(err)
		}
		if detail.State != "dead_lettered" || len(detail.Attempts) != generation+1 {
			t.Fatal("attempt history lost")
		}
		want := 202
		if generation == 2 {
			want = 429
		}
		assertStatus(call("POST", "/v1/sandbox/events/"+receipt.ID+"/replay", "{}", &cookieA, h.Origin), want)
	}
	// Recover a lost lease and reject the stale worker, then exercise real retry persistence.
	temporary, err := store.Submit(ctx, idA, "temporary_failure")
	if err != nil {
		t.Fatal(err)
	}
	first, err := reads.Claim(ctx, time.Now(), 30*time.Second)
	if err != nil || first == nil {
		t.Fatalf("first claim: %v", err)
	}
	second, err := reads.Claim(ctx, time.Now().Add(31*time.Second), 30*time.Second)
	if err != nil || second == nil || second.ClaimID == first.ClaimID {
		t.Fatalf("recovery claim: %v", err)
	}
	if err = reads.Record(ctx, first, sender.Send(ctx, first)); err == nil {
		t.Fatal("stale claim committed")
	}
	if err = reads.Record(ctx, second, sender.Send(ctx, second)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = pool.Exec(ctx, `UPDATE p3_relay.events SET next_attempt_at=now() WHERE id=$1`, temporary); err != nil {
			t.Fatal(err)
		}
		if processed, err := worker.RunOnce(ctx, time.Now()); err != nil || !processed {
			t.Fatalf("retry: %v", err)
		}
	}
	detail, err := reads.Detail(ctx, temporary, "sandbox:"+idA)
	if err != nil || detail.State != "delivered" || len(detail.Attempts) != 3 {
		t.Fatalf("retry completion: %v", err)
	}
	verifyNotificationFailures(t, ctx, pool, store, reads, worker, idA)
	// Concurrent admissions share one transactionally enforced per-session quota.
	var admitted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 24; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.Submit(ctx, idB, "success")
			if err == nil {
				admitted.Add(1)
			} else if err != ErrQuota {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if admitted.Load() != 20 {
		t.Fatalf("admitted %d want 20", admitted.Load())
	}
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.sandbox_budget SET admissions=1000`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Submit(ctx, idA, "success"); err != ErrQuota {
		t.Fatalf("global quota: %v", err)
	}
	// Preserve active delivery leases during cleanup, then recover after their expiry.
	leased, err := reads.Claim(ctx, time.Now(), 30*time.Second)
	if err != nil || leased == nil {
		t.Fatalf("cleanup lease fixture: %v", err)
	}
	// Expiry denies reads, suppresses claims, and cleanup removes only expired owners.
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.sandbox_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, idB); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("GET", "/v1/sandbox/events", "", &cookieB, ""), 401)
	if claimed, err := reads.Claim(ctx, time.Now(), 30*time.Second); err != nil || claimed != nil {
		t.Fatalf("expired claim: %v", err)
	}
	if err = store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	var retained int32
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM p3_relay.events WHERE sandbox_id=$1`, idB).Scan(&retained); err != nil || retained != 1 {
		t.Fatal("cleanup deleted active lease")
	}
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.events SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, leased.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	var count int32
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM p3_relay.events WHERE sandbox_id=$1`, idB).Scan(&count); err != nil || count != 0 {
		t.Fatal("cleanup retained expired events")
	}
	if err = store.Active(ctx, idA); err != nil {
		t.Fatal("cleanup crossed owner boundary")
	}
	// Shared global issuance budget cannot be bypassed by clearing the browser cookie.
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.sandbox_budget SET sessions=100`); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("POST", "/v1/sandbox/session", "{}", nil, h.Origin), 429)
	// The next UTC window resets hourly budgets, but never the retained-session capacity.
	if _, err = pool.Exec(ctx, `UPDATE p3_relay.sandbox_budget SET window_start=window_start-interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("POST", "/v1/sandbox/session", "{}", nil, h.Origin), 201)
	if _, err = pool.Exec(ctx, `INSERT INTO p3_relay.sandbox_sessions(id,expires_at)
 SELECT lpad(to_hex(n),32,'0'),now()+interval '30 minutes'
 FROM generate_series(1,1000-(SELECT count(*)::integer FROM p3_relay.sandbox_sessions)) n`); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("POST", "/v1/sandbox/session", "{}", nil, h.Origin), 429)
	if _, err = pool.Exec(ctx, `DELETE FROM p3_relay.sandbox_budget`); err != nil {
		t.Fatal(err)
	}
	assertStatus(call("POST", "/v1/sandbox/session", "{}", nil, h.Origin), 503)
	// Database failure is a bounded unavailable response, never anonymous authorization.
	pool.Close()
	assertStatus(call("GET", "/v1/sandbox/events", "", &cookieA, ""), 503)
}
