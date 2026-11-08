import { useEffect, useRef, useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { motion, AnimatePresence } from "framer-motion";
import { AlertCircle, ArrowLeft, Loader2, Phone, ShieldCheck } from "lucide-react";
import { NemoMark } from "@/components/NemoMark";
import { ThemeToggle } from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/contexts/AuthContext";
import { appConfig } from "@/lib/appConfig";
import { sellerAuthService, normalisePhone, decodeJwt, type SendOtpResponse } from "@/services/sellerAuthService";

function initialsOf(name: string): string {
  return name.split(" ").filter(Boolean).slice(0, 2).map((p) => p[0].toUpperCase()).join("") || "S";
}

/** Phone + OTP sign-in for sellers via bff-gateway. Token is stored under `athena_jwt`. */
const SellerLoginPage = () => {
  const { isAuthenticated, loginWithToken } = useAuth();
  const navigate = useNavigate();
  const [step, setStep] = useState<"phone" | "otp">("phone");
  const [phone, setPhone] = useState(() => sellerAuthService.lastPhone());
  const [otp, setOtp] = useState("");
  const [sent, setSent] = useState<SendOtpResponse | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const otpRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (cooldown <= 0) return;
    const t = setTimeout(() => setCooldown((c) => c - 1), 1000);
    return () => clearTimeout(t);
  }, [cooldown]);

  useEffect(() => {
    if (step === "otp") otpRef.current?.focus();
  }, [step]);

  if (isAuthenticated) return <Navigate to="/" replace />;

  const normalised = normalisePhone(phone);
  const phoneValid = /^\+\d{9,15}$/.test(normalised);

  const send = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (!phoneValid) {
      setError("Enter a valid mobile number, e.g. 0712 345 678");
      return;
    }
    setError("");
    setBusy(true);
    try {
      const r = await sellerAuthService.sendOtp(normalised);
      setSent(r);
      setStep("otp");
      setCooldown(30);
      // Dev/sandbox: bff echoes the OTP when OTP_ECHO_ENABLED=true — prefill it in demo mode.
      if (appConfig.demo && r?.otp) setOtp(r.otp);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not send the code");
    } finally {
      setBusy(false);
    }
  };

  const verify = async (e?: React.FormEvent) => {
    e?.preventDefault();
    if (otp.trim().length < 4) {
      setError("Enter the code we sent you");
      return;
    }
    setError("");
    setBusy(true);
    try {
      const r = await sellerAuthService.verifyOtp(normalised, otp);
      const claims = decodeJwt(r.accessToken) ?? {};
      const name = r.user.fullName || (claims.name as string) || r.user.businessName || normalised;
      loginWithToken(r.accessToken, {
        id: r.user.id || (claims.sub as string) || normalised,
        email: (claims.email as string) || "",
        name,
        initials: initialsOf(name),
        role: "SELLER",
        tenantId: (claims.tenantId as string) || undefined,
      });
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign-in failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col">
      <div className="h-1.5 nemo-stripes opacity-90" aria-hidden />
      <header className="px-4 sm:px-8 py-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <NemoMark className="h-9 w-9" />
          <div>
            <p className="font-heading text-base tracking-wide leading-none">Nemo Invoice Finance</p>
            <p className="text-[11px] uppercase tracking-widest text-muted-foreground mt-0.5">Seller app</p>
          </div>
        </div>
        <ThemeToggle />
      </header>

      <main className="flex-1 px-4 sm:px-8 pb-10 flex items-start justify-center">
        <motion.div initial={{ opacity: 0, y: 16 }} animate={{ opacity: 1, y: 0 }} transition={{ duration: 0.4 }} className="w-full max-w-md mt-6 sm:mt-14">
          <div className="mb-6">
            <p className="eyebrow">Get paid today</p>
            <h1 className="font-heading text-3xl font-semibold tracking-tight mt-2">Turn invoices into cash</h1>
            <p className="text-base text-muted-foreground mt-2">Upload an invoice, your buyer confirms it, and we advance you most of it now.</p>
          </div>

          <div data-nemo-card="" className="rounded-xl border bg-card p-6">
            <AnimatePresence mode="wait">
              {error && (
                <motion.div
                  key="err"
                  initial={{ opacity: 0, height: 0 }}
                  animate={{ opacity: 1, height: "auto" }}
                  exit={{ opacity: 0, height: 0 }}
                  className="mb-4 flex items-center gap-2 rounded-lg bg-destructive/10 border border-destructive/20 px-3 py-2.5 text-sm text-destructive"
                  role="alert"
                >
                  <AlertCircle className="h-4 w-4 shrink-0" />
                  {error}
                </motion.div>
              )}
            </AnimatePresence>

            {step === "phone" ? (
              <form onSubmit={send} className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="phone" className="text-sm">
                    Mobile number
                  </Label>
                  <div className="relative">
                    <Phone className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-accent/80" />
                    <Input
                      id="phone"
                      type="tel"
                      inputMode="tel"
                      autoComplete="tel"
                      placeholder="0712 345 678"
                      value={phone}
                      onChange={(e) => setPhone(e.target.value)}
                      className="h-12 pl-10 text-base font-mono"
                      required
                    />
                  </div>
                  <p className="text-xs text-muted-foreground">
                    We'll text a one-time code to {phoneValid ? <span className="font-mono text-foreground">{normalised}</span> : "this number"}.
                  </p>
                </div>
                <Button type="submit" className="w-full h-12 text-base" disabled={busy}>
                  {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
                  {busy ? "Sending code…" : "Send code"}
                </Button>
              </form>
            ) : (
              <form onSubmit={verify} className="space-y-4">
                <button type="button" onClick={() => { setStep("phone"); setOtp(""); setError(""); }} className="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
                  <ArrowLeft className="h-4 w-4" /> <span className="font-mono">{normalised}</span>
                </button>
                <div className="space-y-2">
                  <Label htmlFor="otp" className="text-sm">
                    One-time code
                  </Label>
                  <Input
                    ref={otpRef}
                    id="otp"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    pattern="[0-9]*"
                    maxLength={8}
                    placeholder="••••••"
                    value={otp}
                    onChange={(e) => setOtp(e.target.value.replace(/\D/g, ""))}
                    className="h-14 text-center text-2xl tracking-[0.5em] font-mono"
                    required
                  />
                  <p className="text-xs text-muted-foreground">
                    {sent?.expiresInSeconds ? `Code expires in ${Math.round(sent.expiresInSeconds / 60)} min. ` : ""}
                    Didn't get it?{" "}
                    <button type="button" className="text-primary hover:underline disabled:text-muted-foreground disabled:no-underline" onClick={() => send()} disabled={busy || cooldown > 0}>
                      {cooldown > 0 ? `Resend in ${cooldown}s` : "Resend"}
                    </button>
                  </p>
                </div>
                <Button type="submit" className="w-full h-12 text-base" disabled={busy}>
                  {busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <ShieldCheck className="h-4 w-4" />}
                  {busy ? "Verifying…" : "Sign in"}
                </Button>
              </form>
            )}

            {appConfig.demo && (
              <div className="mt-5 rounded-lg border border-warning/30 bg-warning/5 p-3">
                <p className="text-[11px] uppercase tracking-wider text-warning font-semibold mb-1">Demo mode</p>
                {sent?.otp ? (
                  <p className="text-sm">
                    Your code is <code className="font-mono font-semibold text-foreground">{sent.otp}</code> (echoed by the sandbox gateway — it's prefilled above).
                  </p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    No SMS is sent in demo. The gateway echoes the code in its response when <code className="font-mono">OTP_ECHO_ENABLED=true</code>; it will appear here and prefill the field. Buyer confirmation links use OTP <code className="font-mono">000000</code>.
                  </p>
                )}
              </div>
            )}
          </div>

          <p className="text-center text-[11px] text-muted-foreground mt-6">
            Every action is written to a tamper-evident ledger. © 2026 Nemo.
          </p>
        </motion.div>
      </main>
    </div>
  );
};

export default SellerLoginPage;
