import { Building2, Phone, RefreshCw, LogOut } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { useAuth } from "@/contexts/AuthContext";
import { SellerShell } from "./components/SellerShell";

/**
 * Shown when the API answers 409 SELLER_PROFILE_MISSING: the phone login worked
 * but account-service has no business (seller) profile for this user yet.
 */
export function SellerOnboardingPage({ onRetry, retrying }: { onRetry?: () => void; retrying?: boolean }) {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  return (
    <SellerShell title="Almost there" subtitle="Your business profile needs to be set up before you can finance invoices.">
      <div data-nemo-card="" className="rounded-xl border bg-card p-6 space-y-5">
        <div className="flex items-start gap-4">
          <div className="h-12 w-12 rounded-xl bg-primary/10 flex items-center justify-center shrink-0">
            <Building2 className="h-6 w-6 text-primary" />
          </div>
          <div className="space-y-2">
            <p className="eyebrow">Business profile</p>
            <p className="text-base">
              You're signed in as <span className="font-mono">{user?.name ?? "this number"}</span>, but the bank hasn't created a business profile for you yet. Profiles are opened by your relationship officer after a short KYB check — it can't be self-served here.
            </p>
            <p className="text-sm text-muted-foreground">Once it's created, come back and tap <em>Check again</em>. Your sign-in stays valid.</p>
          </div>
        </div>

        <ol className="space-y-3 text-sm">
          {[
            ["Contact the bank", "Call or visit your branch and ask for Nemo Invoice Finance onboarding."],
            ["Share your details", "Business registration, KRA PIN, and the mobile number you signed in with."],
            ["We link your profile", "Your invoices and payouts are tied to this number."],
          ].map(([t, d], i) => (
            <li key={t} className="flex gap-3">
              <span className="h-6 w-6 rounded-full bg-accent/15 text-accent text-[11px] font-semibold flex items-center justify-center shrink-0">{i + 1}</span>
              <div>
                <p className="font-medium">{t}</p>
                <p className="text-muted-foreground">{d}</p>
              </div>
            </li>
          ))}
        </ol>

        <div className="flex flex-col sm:flex-row gap-2 pt-1">
          {onRetry && (
            <Button className="h-11" onClick={onRetry} disabled={retrying}>
              <RefreshCw className={retrying ? "h-4 w-4 animate-spin" : "h-4 w-4"} /> Check again
            </Button>
          )}
          <Button variant="outline" className="h-11" asChild>
            <a href="tel:+254700000000">
              <Phone className="h-4 w-4" /> Call the bank
            </a>
          </Button>
          <Button
            variant="ghost"
            className="h-11 sm:ml-auto text-muted-foreground"
            onClick={() => {
              logout();
              navigate("/login");
            }}
          >
            <LogOut className="h-4 w-4" /> Sign out
          </Button>
        </div>
      </div>
    </SellerShell>
  );
}

export default SellerOnboardingPage;
