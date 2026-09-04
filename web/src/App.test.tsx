import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { App } from "./App";

describe("App", () => {
    it("identifies the product and current stage", () => {
        render(<App />);

        expect(screen.getByRole("heading", { name: "Relay" })).toBeInTheDocument();
        expect(screen.getByText(/Foundation ready/)).toBeInTheDocument();
    });
});
