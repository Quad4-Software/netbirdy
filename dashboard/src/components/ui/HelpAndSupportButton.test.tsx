import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

vi.mock("@utils/netbirdy", () => ({
  isNetBirdCloud: () => false,
}));

import HelpAndSupportButton from "./HelpAndSupportButton";

describe("HelpAndSupportButton", () => {
  it("links to the NetBirdy source code repository", async () => {
    render(<HelpAndSupportButton />);
    await userEvent.click(screen.getByRole("button"));
    const link = await screen.findByRole("menuitem", { name: /source code/i });
    expect(link.getAttribute("href")).toBe(
      "https://github.com/Sudo-Ivan/netbirdy",
    );
  });
});
