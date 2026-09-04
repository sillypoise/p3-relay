package delivery

import "testing"

func TestHTTPOutcomeBoundaries(t *testing.T) {
    test_cases := map[uint16]string{
        199: "terminal_http",
        200: "delivered",
        299: "delivered",
        300: "terminal_http",
        408: "retryable_http",
        429: "retryable_http",
        499: "terminal_http",
        500: "retryable_http",
        599: "retryable_http",
        600: "terminal_http",
    }
    for status_code, expected := range test_cases {
        if actual := HTTPOutcome(status_code); actual != expected {
            t.Fatalf("status %d outcome = %q, want %q", status_code, actual, expected)
        }
    }
}

func TestCanRetryAttemptBoundary(t *testing.T) {
    if !CanRetry(7) {
        t.Fatal("attempt 7 should allow the final attempt")
    }
    if CanRetry(8) {
        t.Fatal("attempt 8 should exhaust the attempt budget")
    }
}
