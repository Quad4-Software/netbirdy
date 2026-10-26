import { describe, expect, it } from "vitest";
import { OperatingSystem } from "@/interfaces/OperatingSystem";
import { getOperatingSystem } from "./useOperatingSystem";

describe("getOperatingSystem", () => {
  const cases: [string, OperatingSystem][] = [
    ["OPNsense 25.1", OperatingSystem.FREEBSD],
    ["pfSense 2.7.2-RELEASE", OperatingSystem.FREEBSD],
    ["FreeBSD 14.2", OperatingSystem.FREEBSD],
    ["GhostBSD 24", OperatingSystem.FREEBSD],
    ["HardenedBSD 15", OperatingSystem.FREEBSD],
    ["MidnightBSD 3", OperatingSystem.FREEBSD],
    ["Ubuntu 24.04", OperatingSystem.LINUX],
    ["Windows 11", OperatingSystem.WINDOWS],
    ["darwin", OperatingSystem.APPLE],
    ["macOS 15", OperatingSystem.APPLE],
    ["Android 14", OperatingSystem.ANDROID],
    ["iOS 17", OperatingSystem.IOS],
    ["", OperatingSystem.LINUX],
  ];
  it.each(cases)("%s -> %s", (input, want) => {
    expect(getOperatingSystem(input)).toBe(want);
  });
});
