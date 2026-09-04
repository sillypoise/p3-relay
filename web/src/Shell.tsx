import { Link, Outlet } from "@tanstack/react-router";

export function Shell() {
    return (
        <>
            <aside>
                <Link to="/" className="brand">
                    Relay<span>Webhook operations</span>
                </Link>
                <nav>
                    <Link to="/">Overview</Link>
                    <Link to="/events">Events</Link>
                    <Link to="/endpoint">Endpoint</Link>
                </nav>
                <footer>Independent product concept</footer>
            </aside>
            <main>
                <Outlet />
            </main>
        </>
    );
}
