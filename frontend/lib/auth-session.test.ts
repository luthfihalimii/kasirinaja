import { beforeEach, describe, expect, it } from "vitest";

import {
  clearStoredAuthSession,
  isSessionExpired,
  persistAuthSession,
  restoreAuthSession,
  toAuthSession,
} from "@/lib/auth-session";

describe("auth-session", () => {
  beforeEach(() => {
    clearStoredAuthSession();
  });

  it("stores and restores session payload", () => {
    const session = toAuthSession("admin", {
      access_token: "token-123",
      role: "admin",
      expires_at: new Date(Date.now() + 60_000).toISOString(),
    });
    persistAuthSession(session);

    const restored = restoreAuthSession();
    expect(restored).toEqual(session);
  });

  it("returns null for malformed session payload", () => {
    window.sessionStorage.setItem("kasirinaja_auth_session", "{bad json");
    expect(restoreAuthSession()).toBeNull();
  });

  it("detects expired session correctly", () => {
    expect(isSessionExpired(new Date(Date.now() - 5_000).toISOString())).toBe(true);
    expect(isSessionExpired(new Date(Date.now() + 5_000).toISOString())).toBe(false);
  });
});
