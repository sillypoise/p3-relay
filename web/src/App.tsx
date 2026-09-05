import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { useState } from "react";

import { hasToken, setToken } from "./api";
import { router } from "./router";
import "./App.css";
import { Sandbox } from "./Sandbox";

const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: 1, staleTime: 5_000 } },
});

export function App() {
    const [authorized, setAuthorized] = useState(hasToken());
    const [sandbox, setSandbox] = useState(false);
    if (sandbox) return <Sandbox onExit={() => setSandbox(false)} />;
    if (!authorized)
        return (
            <>
                <Authorization onAuthorized={() => setAuthorized(true)} />
                <section className="sandbox-entry">
                    <button onClick={() => setSandbox(true)}>Try the public sandbox</button>
                    <p>Simulated receivers. Isolated visitor data. No operator token required.</p>
                </section>
            </>
        );
    return (
        <QueryClientProvider client={queryClient}>
            <RouterProvider router={router} />
        </QueryClientProvider>
    );
}

function Authorization({ onAuthorized }: { onAuthorized: () => void }) {
    const [token, updateToken] = useState("");
    return (
        <main className="authorization">
            <p className="eyebrow">Local operator access</p>
            <h1>Relay</h1>
            <p>
                Enter the operator token to inspect delivery state. The token remains in this
                browser tab.
            </p>
            <form
                onSubmit={(event) => {
                    event.preventDefault();
                    if (token.length >= 16) {
                        setToken(token);
                        onAuthorized();
                    }
                }}
            >
                <label htmlFor="token">Operator token</label>
                <input
                    id="token"
                    type="password"
                    minLength={16}
                    required
                    value={token}
                    onChange={(event) => updateToken(event.target.value)}
                />
                <button type="submit">Open dashboard</button>
            </form>
        </main>
    );
}
