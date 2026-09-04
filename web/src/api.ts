export type Overview = {
    pending: number;
    delivering: number;
    delivered: number;
    retrying: number;
    dead_lettered: number;
};

export type EventSummary = {
    id: string;
    external_event_id: string;
    state: string;
    attempt_count: number;
    created_at: string;
};

export type Attempt = {
    replay_number: number;
    attempt_number: number;
    outcome: string;
    status_code: number | null;
    error_code: string | null;
    started_at: string;
    response_excerpt: string;
};

export type EventDetail = EventSummary & {
    destination_url: string;
    replay_count: number;
    attempts: Attempt[];
};

export type Endpoint = {
    source_key: string;
    destination_url: string;
    enabled: boolean;
    secrets: string;
};

export function setToken(token: string) {
    sessionStorage.setItem("relay_operator_token", token);
}

export function hasToken() {
    return sessionStorage.getItem("relay_operator_token") !== null;
}

export async function apiRequest<T>(path: string, options?: RequestInit): Promise<T> {
    const token = sessionStorage.getItem("relay_operator_token");
    if (token === null) throw new Error("Operator authorization is required.");
    const response = await fetch(path, {
        ...options,
        headers: { Authorization: `Bearer ${token}`, ...options?.headers },
    });
    if (!response.ok) {
        if (response.status === 401) sessionStorage.removeItem("relay_operator_token");
        throw new Error(`Relay request failed with status ${response.status}.`);
    }
    return (await response.json()) as T;
}
