import { defineRailway, empty, project, service } from "railway/iac";

// Independent portfolio repositories cannot safely share one environment-wide owner.
// Never rename this partial after applying it: omission/deletion is scoped by this identity.
export const partial = "p3-relay-gateway";

export default defineRailway((context) => {
    if (context.projectId !== "8316dfb8-ff8c-4b7f-a7ea-3882f699e9e9") {
        throw new Error("Gateway configuration requires the verified portfolio project.");
    }
    if (context.environmentId !== "36c8599e-be75-420d-9f6c-bbe9692dc26b") {
        throw new Error("Gateway configuration requires the verified production environment.");
    }

    const gateway = service("p3-relay-db-gateway", {
        // Reserve the endpoint without starting an unconfigured image or automatic Git deployment.
        source: empty(),
        replicas: { "us-east4-eqdc4a": 1 },
        tcp: [6432],
        deploy: {
            limitOverride: { containers: { cpu: 0.25, memoryBytes: 134217728 } },
            restartPolicyType: "ON_FAILURE",
            restartPolicyMaxRetries: 3,
            overlapSeconds: 0,
            drainingSeconds: 15,
            sleepApplication: false,
        },
    });

    return project("upwork-portfolio", { resources: [gateway] });
});
