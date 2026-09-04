import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "@tanstack/react-router";

import type { Endpoint, EventDetail, EventSummary, Overview } from "./api";
import { apiRequest } from "./api";

const stateLabels: Record<string, string> = {
    pending: "Pending",
    delivering: "Delivering",
    delivered: "Delivered",
    retry_scheduled: "Retry scheduled",
    dead_lettered: "Dead lettered",
};

export function OverviewPage() {
    const query = useQuery({
        queryKey: ["overview"],
        queryFn: () => apiRequest<Overview>("/v1/overview"),
    });
    if (query.isPending) return <Status text="Loading delivery overview…" />;
    if (query.isError) return <Status text={query.error.message} error />;
    const cards = Object.entries(query.data);
    return (
        <section>
            <header className="page-header">
                <p>Control plane</p>
                <h1>Delivery overview</h1>
            </header>
            <div className="metrics">
                {cards.map(([state, count]) => (
                    <article className="metric" key={state}>
                        <span>{stateLabels[state]}</span>
                        <strong>{count}</strong>
                    </article>
                ))}
            </div>
            <p className="note">
                Live state from PostgreSQL. Simulated traffic is labeled in event identifiers.
            </p>
        </section>
    );
}

export function EventsPage() {
    const query = useQuery({
        queryKey: ["events"],
        queryFn: () => apiRequest<{ events: EventSummary[] }>("/v1/events"),
    });
    if (query.isPending) return <Status text="Loading events…" />;
    if (query.isError) return <Status text={query.error.message} error />;
    return (
        <section>
            <header className="page-header">
                <p>Event stream</p>
                <h1>Webhook events</h1>
            </header>
            {query.data.events.length === 0 ? (
                <Status text="No events have been received." />
            ) : (
                <div className="table-wrap">
                    <table>
                        <thead>
                            <tr>
                                <th>Event</th>
                                <th>State</th>
                                <th>Attempts</th>
                                <th>Received</th>
                            </tr>
                        </thead>
                        <tbody>
                            {query.data.events.map((event) => (
                                <tr key={event.id}>
                                    <td>
                                        <Link to="/events/$eventId" params={{ eventId: event.id }}>
                                            {event.external_event_id}
                                        </Link>
                                    </td>
                                    <td>
                                        <State value={event.state} />
                                    </td>
                                    <td>{event.attempt_count}</td>
                                    <td>{formatDate(event.created_at)}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </section>
    );
}

export function EventPage() {
    const { eventId } = useParams({ from: "/events/$eventId" });
    const queryClient = useQueryClient();
    const query = useQuery({
        queryKey: ["event", eventId],
        queryFn: () => apiRequest<EventDetail>(`/v1/events/${eventId}`),
    });
    const replay = useMutation({
        mutationFn: () => apiRequest(`/v1/events/${eventId}/replay`, { method: "POST" }),
        onSuccess: async () => {
            await queryClient.invalidateQueries({ queryKey: ["event", eventId] });
        },
    });
    if (query.isPending) return <Status text="Loading event timeline…" />;
    if (query.isError) return <Status text={query.error.message} error />;
    const event = query.data;
    return (
        <section>
            <header className="page-header">
                <p>Event detail</p>
                <h1>{event.external_event_id}</h1>
                <State value={event.state} />
            </header>
            <dl className="details">
                <div>
                    <dt>Destination</dt>
                    <dd>{event.destination_url}</dd>
                </div>
                <div>
                    <dt>Replay generation</dt>
                    <dd>{event.replay_count}</dd>
                </div>
            </dl>
            {event.state === "dead_lettered" && (
                <button disabled={replay.isPending} onClick={() => replay.mutate()}>
                    {replay.isPending ? "Replaying…" : "Replay event"}
                </button>
            )}
            {replay.isError && <p className="error">{replay.error.message}</p>}
            <h2>Attempt timeline</h2>
            {event.attempts.length === 0 ? (
                <Status text="No delivery attempts yet." />
            ) : (
                <ol className="timeline">
                    {event.attempts.map((attempt) => (
                        <li key={`${attempt.replay_number}-${attempt.attempt_number}`}>
                            <div>
                                <strong>Attempt {attempt.attempt_number}</strong>
                                <State value={attempt.outcome} />
                            </div>
                            <p>
                                {formatDate(attempt.started_at)} ·{" "}
                                {attempt.status_code ?? attempt.error_code}
                            </p>
                            {attempt.response_excerpt && <pre>{attempt.response_excerpt}</pre>}
                        </li>
                    ))}
                </ol>
            )}
        </section>
    );
}

export function EndpointPage() {
    const query = useQuery({
        queryKey: ["endpoint"],
        queryFn: () => apiRequest<Endpoint>("/v1/endpoint"),
    });
    if (query.isPending) return <Status text="Loading endpoint…" />;
    if (query.isError) return <Status text={query.error.message} error />;
    return (
        <section>
            <header className="page-header">
                <p>Configuration</p>
                <h1>Delivery endpoint</h1>
            </header>
            <dl className="details">
                <div>
                    <dt>Source key</dt>
                    <dd>{query.data.source_key}</dd>
                </div>
                <div>
                    <dt>Destination</dt>
                    <dd>{query.data.destination_url}</dd>
                </div>
                <div>
                    <dt>Status</dt>
                    <dd>{query.data.enabled ? "Enabled" : "Disabled"}</dd>
                </div>
                <div>
                    <dt>Secrets</dt>
                    <dd>{query.data.secrets}</dd>
                </div>
            </dl>
            <p className="note">
                Runtime configuration is read-only in Phase 4. Secret mutation remains
                server-controlled.
            </p>
        </section>
    );
}

function State({ value }: { value: string }) {
    return (
        <span className={`state state-${value}`}>
            {stateLabels[value] ?? value.replace("_", " ")}
        </span>
    );
}
function Status({ text, error = false }: { text: string; error?: boolean }) {
    return <div className={error ? "status error" : "status"}>{text}</div>;
}
function formatDate(value: string) {
    return new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(
        new Date(value),
    );
}
