package delivery

const maximum_attempts uint8 = 8

func HTTPOutcome(status_code uint16) string {
    if status_code >= 200 {
        if status_code <= 299 {
            return "delivered"
        }
    }
    if status_code == 408 {
        return "retryable_http"
    }
    if status_code == 429 {
        return "retryable_http"
    }
    if status_code >= 500 {
        if status_code <= 599 {
            return "retryable_http"
        }
    }
    return "terminal_http"
}

func CanRetry(attempt_count uint8) bool {
    return attempt_count < maximum_attempts
}
