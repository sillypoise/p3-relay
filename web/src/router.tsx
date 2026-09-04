import { createRootRoute, createRoute, createRouter } from "@tanstack/react-router";

import { EndpointPage, EventPage, EventsPage, OverviewPage } from "./pages";
import { Shell } from "./Shell";

const rootRoute = createRootRoute({ component: Shell });
const overviewRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/",
    component: OverviewPage,
});
const eventsRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/events",
    component: EventsPage,
});
const eventRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/events/$eventId",
    component: EventPage,
});
const endpointRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: "/endpoint",
    component: EndpointPage,
});
const routeTree = rootRoute.addChildren([overviewRoute, eventsRoute, eventRoute, endpointRoute]);
export const router = createRouter({ routeTree });

declare module "@tanstack/react-router" {
    interface Register {
        router: typeof router;
    }
}
