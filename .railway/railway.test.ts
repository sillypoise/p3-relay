import assert from "node:assert/strict";
import test from "node:test";
import { createRailwayContext, project } from "railway/iac";
import configuration, { partial } from "./railway.ts";

const target = {
    projectId: "8316dfb8-ff8c-4b7f-a7ea-3882f699e9e9",
    environmentId: "36c8599e-be75-420d-9f6c-bbe9692dc26b",
};

// Check ownership and launch boundaries without fetching credentials or contacting Railway.
test("gateway partial owns one empty service with bounded deployment settings", async () => {
    const value = await configuration(createRailwayContext(target), project);
    assert.equal(partial, "p3-relay-gateway");
    assert.equal(value.name, "upwork-portfolio");
    assert.equal(value.resources?.length, 1);
    const gateway = value.resources?.[0];
    assert.ok(gateway);
    assert.ok(!Array.isArray(gateway));
    if (gateway.type !== "service") assert.fail("Only a service may be managed here.");
    assert.equal(gateway.name, "p3-relay-db-gateway");
    assert.equal(gateway.kind, "empty");
    assert.deepEqual(gateway.source, { type: "empty" });
    assert.deepEqual(gateway.networking?.tcpProxies, { "6432": {} });
    assert.equal(gateway.networking?.serviceDomains, undefined);
    assert.equal(gateway.variables, undefined);
    assert.equal(gateway.volumeMounts, undefined);
    assert.deepEqual(gateway.deploy?.limitOverride, {
        containers: { cpu: 0.25, memoryBytes: 134217728 },
    });
    assert.deepEqual(gateway.deploy?.multiRegionConfig, {
        "us-east4-eqdc4a": { numReplicas: 1 },
    });
    assert.equal(gateway.deploy?.restartPolicyMaxRetries, 3);
    assert.equal(gateway.deploy?.overlapSeconds, 0);
    assert.equal(gateway.deploy?.drainingSeconds, 15);
});

// Missing context and wrong project/environment must fail before constructing a desired graph.
test("reject absent context", () => {
    assert.throws(
        () => configuration(createRailwayContext(), project),
        /verified portfolio project/,
    );
});

test("reject another project", () => {
    const context = createRailwayContext({ ...target, projectId: "another-project" });
    assert.throws(() => configuration(context, project), /verified portfolio project/);
});

test("reject another environment", () => {
    const context = createRailwayContext({ ...target, environmentId: "another-environment" });
    assert.throws(() => configuration(context, project), /verified production environment/);
});
