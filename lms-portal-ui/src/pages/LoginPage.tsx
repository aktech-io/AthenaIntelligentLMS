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
import { branding, isInvoiceAdminMode } from "@/lib/appConfig";

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
    <div className="min-h-screen grid lg:grid-cols-[1.1fr_1fr] bg-background text-foreground">
      {/* Brand panel */}
      <aside className="hidden lg:flex flex-col justify-between p-12 bg-[hsl(var(--nemo-abyss))] text-[hsl(var(--nemo-stripe))] relative overflow-hidden">
        <div className="absolute inset-x-0 top-0 h-2 nemo-stripes opacity-90" aria-hidden />
        <div
          className="absolute -right-32 -bottom-32 h-[520px] w-[520px] rounded-full opacity-60"
          style={{ background: "radial-gradient(closest-side, hsl(var(--nemo-coral) / .55), transparent)" }}
          aria-hidden
        />
        <div className="flex items-center gap-3 relative">
          <NemoMark className="h-11 w-11 rounded-xl" />
          <div>
            <p className="font-heading text-xl font-bold tracking-[0.08em] uppercase leading-none">Nemo</p>
            <p className="text-[11px] uppercase tracking-[0.2em] text-[hsl(var(--nemo-glass))] mt-1">Neobank in a Box</p>
          </div>
        </div>
        <div className="relative max-w-md">
          <p className="eyebrow !text-[hsl(var(--nemo-aqua))]">Invoice finance · Lending · Deposits · Compliance</p>
          <h2 className="font-heading text-4xl font-semibold mt-3 leading-tight">One bank in a box, from KYC to the general ledger.</h2>
          <p className="mt-4 text-base text-[hsl(var(--nemo-glass))]">Automated decisions with people at the money gates. Every action lands in a tamper-evident ledger.</p>
        </div>
        <p className="relative text-xs text-[hsl(var(--nemo-glass))]">© 2026 Nemo · aktech</p>
      </aside>

      <div className="flex items-center justify-center p-6 sm:p-12">

      <motion.div
        initial={{ opacity: 0, y: 16 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.4 }}
        className="relative z-10 w-full max-w-md"
      >
        {/* Logo & Brand (small screens only — the brand panel is hidden there) */}
        <div className="lg:hidden flex items-center gap-3 mb-8">
          <NemoMark className="h-11 w-11 rounded-xl" />
          <div>
            <h1 className="font-heading text-xl font-bold tracking-[0.08em] uppercase leading-none">Nemo</h1>
            <p className="text-[11px] text-muted-foreground mt-1 font-sans uppercase tracking-[0.2em]">Neobank in a Box</p>
          </div>
        </div>

        <Card className="border-border">
          <CardContent className="p-8">
            <h2 className="font-heading text-2xl font-semibold mb-1">Welcome back</h2>
            <p className="text-sm text-muted-foreground mb-6 font-sans">
              {isInvoiceAdminMode() ? "Staff sign-in — invoice finance administration" : "Sign in to your account to continue"}
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
              <p className="text-[11px] uppercase tracking-wider text-muted-foreground font-sans font-semibold mb-2">
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
                    <span className="text-[11px] font-sans text-accent font-medium">{cred.role}</span>
                  </button>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>

        <p className="text-center text-xs text-muted-foreground mt-6 font-sans">
          © 2026 Nemo. All rights reserved.
        </p>
      </motion.div>
      </div>
    </div>
  );
};

export default LoginPage;
