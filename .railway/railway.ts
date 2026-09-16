import { defineRailway, image, preserve, project, service } from "railway/iac";

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
        // Scan-matched public manifest; no mutable tag or registry credential is admitted.
        source: image(
            "public.ecr.aws/f3e3j6u2/p3-relay-gateway@" +
                "sha256:c89b062ef8e8cf026925620ad8ee63c5b096b8fed578c719fef993f9628e40b7",
        ),
        // Values enter through controlled stdin, not source files or pinned plans.
        variables: {
            GATEWAY_HOSTNAME: preserve(),
            GATEWAY_CERTIFICATE: preserve(),
            GATEWAY_BACKEND_CA: preserve(),
            GATEWAY_PRIVATE_KEY: { isSealed: true, preserveExisting: true },
            GATEWAY_RUNTIME_PASSWORD: { isSealed: true, preserveExisting: true },
            GATEWAY_MIGRATION_PASSWORD: { isSealed: true, preserveExisting: true },
        },
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
