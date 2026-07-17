import React, { createContext, useContext, useState, useEffect, useCallback } from "react";
import { authService } from "@/services/authService";

export interface User {
  id: string;
  email: string;
  name: string;
  initials: string;
  role: string;
  avatar?: string;
  tenantId?: string;
}

interface AuthContextType {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<{ success: boolean; error?: string }>;
  logout: () => void;
  isBiometricAvailable: boolean;
  enableBiometric: () => Promise<boolean>;
  loginWithBiometric: () => Promise<{ success: boolean; error?: string }>;
  biometricEnabled: boolean;
}

const AuthContext = createContext<AuthContextType | null>(null);

// Demo users — fallback when backend is unreachable
const DEMO_USERS: Record<string, { password: string; user: User }> = {
  "admin@athena.com": {
    password: "admin123",
    user: {
      id: "usr-001",
      email: "admin@athena.com",
      name: "John Mwangi",
      initials: "JM",
      role: "System Administrator",
    },
  },
  "teller@athena.com": {
    password: "teller123",
    user: {
      id: "usr-002",
      email: "teller@athena.com",
      name: "Grace Wanjiku",
      initials: "GW",
      role: "Senior Teller",
    },
  },
  "manager@athena.com": {
    password: "manager123",
    user: {
      id: "usr-003",
      email: "manager@athena.com",
      name: "Peter Ochieng",
      initials: "PO",
      role: "Branch Manager",
    },
  },
  // Also allow backend demo credentials as demo fallback
  "admin": {
    password: "admin123",
    user: {
      id: "usr-001",
      email: "admin@athena.com",
      name: "Admin User",
      initials: "AU",
      role: "System Administrator",
    },
  },
  "manager": {
    password: "manager123",
    user: {
      id: "usr-003",
      email: "manager@athena.com",
      name: "Manager User",
      initials: "MU",
      role: "Branch Manager",
    },
  },
  "officer": {
    password: "officer123",
    user: {
      id: "usr-004",
      email: "officer@athena.com",
      name: "Loan Officer",
      initials: "LO",
      role: "Loan Officer",
    },
  },
};

const AUTH_KEY = "athena_auth_user";
const JWT_KEY = "athena_jwt";
const BIO_KEY = "athena_biometric_enabled";

function makeInitials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0].toUpperCase())
    .join("");
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isBiometricAvailable, setIsBiometricAvailable] = useState(false);
  const [biometricEnabled, setBiometricEnabled] = useState(false);

  // Check biometric availability
  useEffect(() => {
    const checkBiometric = async () => {
      try {
        if (window.PublicKeyCredential) {
          const available = await PublicKeyCredential.isUserVerifyingPlatformAuthenticatorAvailable();
          setIsBiometricAvailable(available);
        }
      } catch {
        setIsBiometricAvailable(false);
      }
    };
    checkBiometric();
  }, []);

  // Restore session from localStorage
  useEffect(() => {
    try {
      const stored = localStorage.getItem(AUTH_KEY);
      if (stored) {
        setUser(JSON.parse(stored));
      }
      const storedToken = localStorage.getItem(JWT_KEY);
      if (storedToken) {
        setToken(storedToken);
      }
      const bioEnabled = localStorage.getItem(BIO_KEY) === "true";
      setBiometricEnabled(bioEnabled);
    } catch {
      // ignore
    }
    setIsLoading(false);
  }, []);

  const login = useCallback(async (email: string, password: string) => {
    // 1. Try real backend
    try {
      const authResp = await authService.login(email, password);
      const userObj: User = {
        id: authResp.username,
        email: authResp.email || email,
        name: authResp.name || authResp.username,
        initials: makeInitials(authResp.name || authResp.username),
        role: authResp.role || (authResp.roles?.[0] ?? "User"),
        tenantId: authResp.tenantId,
      };
      setUser(userObj);
      setToken(authResp.token);
      localStorage.setItem(JWT_KEY, authResp.token);
      localStorage.setItem(AUTH_KEY, JSON.stringify(userObj));
      return { success: true };
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      // 401 or explicit credential error — don't fall back
      if (
        message.toLowerCase().includes("invalid") ||
        message.toLowerCase().includes("unauthorized") ||
        message.toLowerCase().includes("credentials") ||
        message.includes("401")
      ) {
        return { success: false, error: "Invalid credentials" };
      }

      // 2. Network/service unavailable — fall back to DEMO_USERS
      console.warn("[AuthContext] Backend unreachable, falling back to demo users:", message);
      const key = email.toLowerCase();
      const entry = DEMO_USERS[key];
      if (!entry || entry.password !== password) {
        return { success: false, error: "Invalid email or password" };
      }
      setUser(entry.user);
      setToken(null);
      localStorage.removeItem(JWT_KEY);
      localStorage.setItem(AUTH_KEY, JSON.stringify(entry.user));
      return { success: true };
    }
  }, []);

  const logout = useCallback(() => {
    setUser(null);
    setToken(null);
    localStorage.removeItem(AUTH_KEY);
    localStorage.removeItem(JWT_KEY);
    // Keep biometric preference
  }, []);

  const enableBiometric = useCallback(async () => {
    if (!isBiometricAvailable || !user) return false;

    try {
      const challenge = new Uint8Array(32);
      crypto.getRandomValues(challenge);

      const credential = await navigator.credentials.create({
        publicKey: {
          challenge,
          rp: { name: "Nemo", id: window.location.hostname },
          user: {
            id: new TextEncoder().encode(user.id),
            name: user.email,
            displayName: user.name,
          },
          pubKeyCredParams: [
            { type: "public-key", alg: -7 },
            { type: "public-key", alg: -257 },
          ],
          authenticatorSelection: {
            authenticatorAttachment: "platform",
            userVerification: "required",
          },
          timeout: 60000,
        },
      });

      if (credential) {
        localStorage.setItem(BIO_KEY, "true");
        localStorage.setItem(
          "athena_bio_cred_id",
          btoa(String.fromCharCode(...new Uint8Array((credential as PublicKeyCredential).rawId)))
        );
        setBiometricEnabled(true);
        return true;
      }
      return false;
    } catch {
      return false;
    }
  }, [isBiometricAvailable, user]);

  const loginWithBiometric = useCallback(async () => {
    if (!isBiometricAvailable || !biometricEnabled) {
      return { success: false, error: "Biometric login not available" };
    }

    try {
      const storedCredId = localStorage.getItem("athena_bio_cred_id");
      if (!storedCredId) {
        return { success: false, error: "No biometric credential found. Please log in with password first." };
      }

      const challenge = new Uint8Array(32);
      crypto.getRandomValues(challenge);

      const credIdArray = Uint8Array.from(atob(storedCredId), (c) => c.charCodeAt(0));

      const assertion = await navigator.credentials.get({
        publicKey: {
          challenge,
          allowCredentials: [
            {
              type: "public-key",
              id: credIdArray,
              transports: ["internal"],
            },
          ],
          userVerification: "required",
          timeout: 60000,
        },
      });

      if (assertion) {
        const stored = localStorage.getItem(AUTH_KEY);
        if (stored) {
          const restoredUser = JSON.parse(stored);
          setUser(restoredUser);
          return { success: true };
        }
        return { success: false, error: "No previous session found" };
      }
      return { success: false, error: "Biometric verification failed" };
    } catch {
      return { success: false, error: "Biometric authentication cancelled or failed" };
    }
  }, [isBiometricAvailable, biometricEnabled]);

  return (
    <AuthContext.Provider
      value={{
        user,
        token,
        isAuthenticated: !!user,
        isLoading,
        login,
        logout,
        isBiometricAvailable,
        enableBiometric,
        loginWithBiometric,
        biometricEnabled,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
