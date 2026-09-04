import "@testing-library/jest-dom/vitest";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "./App";

describe("App", () => {
    beforeEach(() => sessionStorage.clear());

    it("requires operator authorization before showing delivery data", () => {
        render(<App />);

        expect(screen.getByRole("heading", { name: "Relay" })).toBeInTheDocument();
        expect(screen.getByLabelText("Operator token")).toHaveAttribute("type", "password");
        expect(screen.queryByText("Delivery overview")).not.toBeInTheDocument();
    });
});
