import { create } from "zustand";

import type { AuthUser } from "@/lib/auth/types";

type AuthState = {
  user: AuthUser | null;
  bootstrapped: boolean;
  setUser: (user: AuthUser | null) => void;
  setBootstrapped: (value: boolean) => void;
  clear: () => void;
};

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  bootstrapped: false,
  setUser: (user) => set({ user }),
  setBootstrapped: (bootstrapped) => set({ bootstrapped }),
  clear: () => set({ user: null }),
}));

export type { AuthUser };
