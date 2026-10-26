import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import InlineLink from "./InlineLink";

describe("InlineLink", () => {
  it("adds noopener noreferrer for _blank", () => {
    const { container } = render(<InlineLink href="https://x.test" target="_blank">l</InlineLink>);
    expect(container.querySelector("a")?.getAttribute("rel")).toBe("noopener noreferrer");
  });
  it("does not add rel otherwise", () => {
    const { container } = render(<InlineLink href="/a">l</InlineLink>);
    expect(container.querySelector("a")?.getAttribute("rel")).toBeNull();
  });
});
