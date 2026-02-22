import type { LoginResponse, Role } from "@/lib/types";

export type AuthSession = {
  username: string;
  accessToken: string;
  role: Role;
  expiresAt: string;
};

// In-memory token storage — avoids XSS token theft via sessionStorage/localStorage.
// The trade-off is that the session is lost on page reload (user must log in again).
let _session: AuthSession | null = null;

export function restoreAuthSession(): AuthSession | null {
  return _session;
}

export function persistAuthSession(session: AuthSession): void {
  _session = session;
}

export function clearStoredAuthSession(): void {
  _session = null;
}

export function isSessionExpired(expiresAt: string): boolean {
  const expiry = Date.parse(expiresAt);
  if (!Number.isFinite(expiry)) {
    return true;
  }
  return Date.now() >= expiry;
}

export function toAuthSession(username: string, payload: LoginResponse): AuthSession {
  return {
    username,
    accessToken: payload.access_token,
    role: payload.role,
    expiresAt: payload.expires_at,
  };
}
