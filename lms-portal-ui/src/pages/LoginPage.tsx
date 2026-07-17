import { NemoMark } from "@/components/NemoMark";
import { useState } from "react";
import { useAuth } from "@/contexts/AuthContext";
import { Navigate } from "react-router-dom";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Eye, EyeOff, Fingerprint, Loader2, AlertCircle } from "lucide-react";
import { motion, AnimatePresence } from "framer-motion";

const LoginPage = () => {
  const { login, loginWithBiometric, isAuthenticated, isBiometricAvailable, biometricEnabled } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [biometricLoading, setBiometricLoading] = useState(false);

  if (isAuthenticated) {
    return <Navigate to="/" replace />;
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    const result = await login(email, password);
    if (!result.success) {
      setError(result.error || "Login failed");
    }
    setLoading(false);
  };

  const handleBiometric = async () => {
    setError("");
    setBiometricLoading(true);
    const result = await loginWithBiometric();
    if (!result.success) {
      setError(result.error || "Biometric login failed");
    }
    setBiometricLoading(false);
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-[hsl(var(--navy-950))] via-[hsl(var(--navy-900))] to-[hsl(var(--primary))]">
      {/* Background pattern */}
      <div className="absolute inset-0 opacity-5">
        <div
          className="absolute inset-0"
          style={{
            backgroundImage:
              "radial-gradient(circle at 25% 25%, hsl(var(--accent)) 0%, transparent 50%), radial-gradient(circle at 75% 75%, hsl(var(--accent)) 0%, transparent 50%)",
          }}
        />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5 }}
        className="relative z-10 w-full max-w-md px-4"
      >
        {/* Logo & Brand */}
        <div className="text-center mb-8">
          <NemoMark className="inline-flex h-16 w-16 mb-4 rounded-2xl shadow-lg shadow-accent/20" />
          <h1 className="font-heading text-3xl text-white">Nemo</h1>
          <p className="text-sm text-white/50 mt-1 font-sans uppercase tracking-widest">
            Neobank in a Box
          </p>
        </div>

        <Card className="border-0 shadow-2xl shadow-black/30">
          <CardContent className="p-8">
            <h2 className="font-heading text-xl text-center mb-1">Welcome back</h2>
            <p className="text-xs text-muted-foreground text-center mb-6 font-sans">
              Sign in to your account to continue
            </p>

            <AnimatePresence mode="wait">
              {error && (
                <motion.div
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="mb-4 flex items-center gap-2 rounded-lg bg-destructive/10 border border-destructive/20 px-3 py-2.5 text-xs text-destructive font-sans"
                >
                  <AlertCircle className="h-3.5 w-3.5 shrink-0" />
                  {error}
                </motion.div>
              )}
            </AnimatePresence>

            {/* Biometric login button */}
            {isBiometricAvailable && biometricEnabled && (
              <div className="mb-6">
                <Button
                  variant="outline"
                  className="w-full h-14 gap-3 border-2 border-dashed hover:border-accent hover:bg-accent/5 transition-all"
                  onClick={handleBiometric}
                  disabled={biometricLoading}
                >
                  {biometricLoading ? (
                    <Loader2 className="h-5 w-5 animate-spin" />
                  ) : (
                    <Fingerprint className="h-5 w-5 text-accent" />
                  )}
                  <span className="font-sans text-sm">
                    {biometricLoading ? "Verifying..." : "Sign in with Biometrics"}
                  </span>
                </Button>
                <div className="relative my-5">
                  <div className="absolute inset-0 flex items-center">
                    <span className="w-full border-t" />
                  </div>
                  <div className="relative flex justify-center text-xs uppercase">
                    <span className="bg-card px-3 text-muted-foreground font-sans">
                      or use email
                    </span>
                  </div>
                </div>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="email" className="text-xs font-sans font-medium">
                  Email Address
                </Label>
                <Input
                  id="email"
                  type="email"
                  placeholder="admin@athena.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="h-10 font-sans text-sm"
                  required
                  autoComplete="email"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="password" className="text-xs font-sans font-medium">
                  Password
                </Label>
                <div className="relative">
                  <Input
                    id="password"
                    type={showPassword ? "text" : "password"}
                    placeholder="••••••••"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="h-10 font-sans text-sm pr-10"
                    required
                    autoComplete="current-password"
                  />
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {showPassword ? (
                      <EyeOff className="h-4 w-4" />
                    ) : (
                      <Eye className="h-4 w-4" />
                    )}
                  </button>
                </div>
              </div>

              <Button
                type="submit"
                className="w-full h-10 font-sans text-sm mt-2"
                disabled={loading}
              >
                {loading ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin mr-2" />
                    Signing in…
                  </>
                ) : (
                  "Sign In"
                )}
              </Button>
            </form>

            {/* Demo credentials */}
            <div className="mt-6 rounded-lg bg-muted/50 p-3">
              <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans font-semibold mb-2">
                Demo Credentials
              </p>
              <div className="space-y-1">
                {[
                  { email: "admin@athena.com", pw: "admin123", role: "Admin" },
                  { email: "teller@athena.com", pw: "teller123", role: "Teller" },
                  { email: "manager@athena.com", pw: "manager123", role: "Manager" },
                ].map((cred) => (
                  <button
                    key={cred.email}
                    type="button"
                    onClick={() => {
                      setEmail(cred.email);
                      setPassword(cred.pw);
                      setError("");
                    }}
                    className="w-full flex items-center justify-between text-xs font-mono px-2 py-1.5 rounded hover:bg-muted transition-colors"
                  >
                    <span className="text-muted-foreground">{cred.email}</span>
                    <span className="text-[10px] font-sans text-accent font-medium">{cred.role}</span>
                  </button>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>

        <p className="text-center text-[10px] text-white/30 mt-6 font-sans">
          © 2026 Nemo. All rights reserved.
        </p>
      </motion.div>
    </div>
  );
};

export default LoginPage;
