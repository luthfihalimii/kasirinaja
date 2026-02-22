// Auth store using Zustand - replaces useState for auth state in pos-terminal.
// Uses the existing auth-session.ts functions for in-memory storage.
import { create } from "zustand";

import type { AuthSession } from "@/lib/auth-session";

type AuthState = {
  session: AuthSession | null;
  isLoggedIn: boolean;
  setSession: (session: AuthSession | null) => void;
  clearSession: () => void;
};

export const useAuthStore = create<AuthState>((set) => ({
  session: null,
  isLoggedIn: false,

  setSession(session) {
    set({ session, isLoggedIn: session !== null });
  },

  clearSession() {
    set({ session: null, isLoggedIn: false });
  },
}));
