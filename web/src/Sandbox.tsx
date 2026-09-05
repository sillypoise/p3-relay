import {
    QueryClient,
    QueryClientProvider,
    useMutation,
    useQuery,
    useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import type { EventDetail, EventSummary } from "./api";

class SandboxError extends Error {
    constructor(status: number) {
        super(
            status === 401
                ? "Your session expired. Start a new sandbox to continue."
                : status === 429
                  ? "Sandbox quota reached. Please try again later."
                  : status === 503 || status === 404
                    ? "Sandbox is unavailable. Please try again later."
                    : "The sandbox request could not be completed.",
        );
    }
}

async function request<T>(path: string, body?: object): Promise<T> {
    const response = await fetch(`/v1/sandbox${path}`, {
        method: body === undefined ? "GET" : "POST",
        credentials: "same-origin",
        headers: body === undefined ? {} : { "Content-Type": "application/json" },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
    if (!response.ok) throw new SandboxError(response.status);
    return (await response.json()) as T;
}

export function Sandbox({ onExit }: { onExit: () => void }) {
    const [client] = useState(
        () => new QueryClient({ defaultOptions: { queries: { retry: false } } }),
    );
    return (
        <QueryClientProvider client={client}>
            <SandboxSession onExit={onExit} />
        </QueryClientProvider>
    );
}

function SandboxSession({ onExit }: { onExit: () => void }) {
    const [active, setActive] = useState(false);
    const start = useMutation({
        mutationFn: () => request("/session", {}),
        onSuccess: () => setActive(true),
    });
    return (
        <main className="sandbox">
            <p className="eyebrow">Independent product concept · simulated receivers</p>
            <h1>Try Relay</h1>
            <p>
                Real PostgreSQL receipt, queueing, retries and attempt history. Receiver responses
                are simulated inside the worker; this sandbox sends no external network traffic.
            </p>
            <p>
                Private 30-minute session · 20 events · two replays per event. No personal data or
                URLs.
            </p>
            <button onClick={onExit}>Back to operator access</button>
            {!active && (
                <button disabled={start.isPending} onClick={() => start.mutate()}>
                    {start.isPending ? "Starting…" : "Start isolated sandbox"}
                </button>
            )}
            {start.isError && <p role="alert">{start.error.message}</p>}
            {active && <SandboxEvents onExpired={() => setActive(false)} />}
        </main>
    );
}

function SandboxEvents({ onExpired }: { onExpired: () => void }) {
    const client = useQueryClient();
    const [scenario, setScenario] = useState("success");
    const [selected, setSelected] = useState<string | null>(null);
    const events = useQuery({
        queryKey: ["sandbox-events"],
        queryFn: () => request<{ events: EventSummary[] }>("/events"),
        refetchInterval: (query) => (query.state.error ? false : 5000),
    });
    const create = useMutation({
        mutationFn: () => request<{ event_id: string }>("/events", { scenario }),
        onSuccess: async (result) => {
            setSelected(result.event_id);
            await client.invalidateQueries({ queryKey: ["sandbox-events"] });
        },
    });
    if (events.isPending) return <p role="status">Loading your events…</p>;
    if (events.isError)
        return (
            <div role="alert">
                <p>{events.error.message}</p>
                <button
                    onClick={() => {
                        client.clear();
                        onExpired();
                    }}
                >
                    Return to session start
                </button>
            </div>
        );
    return (
        <section>
            <label htmlFor="scenario">Receiver scenario</label>
            <select id="scenario" value={scenario} onChange={(e) => setScenario(e.target.value)}>
                <option value="success">Immediate success</option>
                <option value="temporary_failure">Fail twice, then succeed</option>
                <option value="permanent_failure">Permanent failure</option>
            </select>
            <button
                disabled={create.isPending || events.data.events.length >= 20}
                onClick={() => create.mutate()}
            >
                {create.isPending ? "Receiving…" : "Send synthetic event"}
            </button>
            {create.isError && <p role="alert">{create.error.message}</p>}
            <h2>Your events ({events.data.events.length}/20)</h2>
            {events.data.events.length === 0 && (
                <p>No events yet. Choose a receiver scenario to start.</p>
            )}
            <ul>
                {events.data.events.map((event) => (
                    <li key={event.id}>
                        <button onClick={() => setSelected(event.id)}>
                            {event.external_event_id}
                        </button>
                        <span>
                            {" "}
                            {event.state} · {event.attempt_count} attempts
                        </span>
                    </li>
                ))}
            </ul>
            {selected !== null && <SandboxDetail key={selected} id={selected} />}
        </section>
    );
}

function SandboxDetail({ id }: { id: string }) {
    const client = useQueryClient();
    const detail = useQuery({
        queryKey: ["sandbox-detail", id],
        queryFn: () => request<EventDetail>(`/events/${id}`),
        refetchInterval: (query) => (query.state.error ? false : 5000),
    });
    const replay = useMutation({
        mutationFn: () => request(`/events/${id}/replay`, {}),
        onSuccess: async () => {
            await client.invalidateQueries({ queryKey: ["sandbox-detail", id] });
            await client.invalidateQueries({ queryKey: ["sandbox-events"] });
        },
    });
    if (detail.isPending) return <p role="status">Loading attempts…</p>;
    if (detail.isError) return <p role="alert">{detail.error.message}</p>;
    return (
        <section>
            <h2>Delivery timeline</h2>
            <p>
                {detail.data.state} · Replay {detail.data.replay_count}/2
            </p>
            {detail.data.state === "dead_lettered" && (
                <button
                    disabled={replay.isPending || detail.data.replay_count >= 2}
                    onClick={() => replay.mutate()}
                >
                    Replay synthetic event
                </button>
            )}
            {replay.isError && <p role="alert">{replay.error.message}</p>}
            {detail.data.attempts.length === 0 && <p>Waiting for the worker…</p>}
            <ol className="timeline">
                {detail.data.attempts.map((attempt) => (
                    <li key={`${attempt.replay_number}-${attempt.attempt_number}`}>
                        Generation {attempt.replay_number}, attempt {attempt.attempt_number}:{" "}
                        {attempt.outcome} ({attempt.status_code})<p>{attempt.response_excerpt}</p>
                    </li>
                ))}
            </ol>
        </section>
    );
}
