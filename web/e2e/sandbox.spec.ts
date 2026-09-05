import { test, expect } from "@playwright/test";

// Browser fixtures test UI transitions only. PostgreSQL integration tests prove ownership/quotas.
test("visitor flow, simulation labels, quota and expiry recovery", async ({ page }) => {
    let created = false;
    let expired = false;
    const event = {
        id: "5a9c38c7-e229-4dad-a702-b03780ba69a7",
        external_event_id: "simulated-permanent_failure",
        state: "dead_lettered",
        attempt_count: 1,
        created_at: "2026-01-01T00:00:00Z",
    };
    await page.route("**/v1/sandbox/**", async (route) => {
        expect(route.request().headers()["authorization"]).toBeUndefined();
        const path = new URL(route.request().url()).pathname;
        if (path.endsWith("/session")) return route.fulfill({ json: { active: true } });
        if (expired)
            return route.fulfill({ status: 401, json: { error: { code: "invalid_session" } } });
        if (path.endsWith("/replay"))
            return route.fulfill({ status: 429, json: { error: { code: "quota_exceeded" } } });
        if (path.endsWith("/events") && route.request().method() === "POST") {
            expect(route.request().postDataJSON()).toEqual({ scenario: "permanent_failure" });
            created = true;
            return route.fulfill({ status: 202, json: { event_id: event.id } });
        }
        if (path.endsWith("/events"))
            return route.fulfill({ json: { events: created ? [event] : [] } });
        return route.fulfill({
            json: {
                ...event,
                replay_count: 0,
                destination_url: "relay-simulator://permanent_failure",
                attempts: [
                    {
                        replay_number: 0,
                        attempt_number: 1,
                        outcome: "terminal_http",
                        status_code: 422,
                        error_code: null,
                        started_at: event.created_at,
                        response_excerpt: "Simulated receiver response; no network request.",
                    },
                ],
            },
        });
    });
    await page.goto("/");
    await page.getByRole("button", { name: "Try the public sandbox" }).click();
    await expect(page.getByText(/sends no external network traffic/)).toBeVisible();
    await page.getByRole("button", { name: "Start isolated sandbox" }).click();
    await expect(page.getByText(/No events yet/)).toBeVisible();
    await page.getByLabel("Receiver scenario").selectOption("permanent_failure");
    await page.getByRole("button", { name: "Send synthetic event" }).click();
    await expect(page.getByRole("heading", { name: "Delivery timeline" })).toBeVisible();
    await page.getByRole("button", { name: "Replay synthetic event" }).click();
    await expect(page.getByRole("alert")).toContainText("quota reached");
    expired = true;
    await expect(page.getByRole("button", { name: "Return to session start" })).toBeVisible({
        timeout: 10000,
    });
    await page.getByRole("button", { name: "Return to session start" }).click();
    await expect(page.getByRole("button", { name: "Start isolated sandbox" })).toBeVisible();
    await expect(page.locator("body")).toHaveJSProperty(
        "scrollWidth",
        await page.locator("body").evaluate((el) => el.clientWidth),
    );
});

test("disabled backend leaves visitor access closed", async ({ page }) => {
    await page.route("**/v1/sandbox/**", (route) =>
        route.fulfill({ status: 404, body: "Not found" }),
    );
    await page.goto("/");
    await page.getByRole("button", { name: "Try the public sandbox" }).click();
    await page.getByRole("button", { name: "Start isolated sandbox" }).click();
    await expect(page.getByRole("alert")).toContainText("unavailable");
    await expect(page.getByRole("button", { name: "Send synthetic event" })).toHaveCount(0);
});
