import { useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { LogOut, Phone, Building2, Hash, ShieldCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ThemeToggle } from "@/components/ThemeToggle";
import { useAuth } from "@/contexts/AuthContext";
import { appConfig } from "@/lib/appConfig";
import { invoiceService, isSellerProfileMissing, type SellerSummary } from "@/services/invoiceService";
import { sellerAuthService, decodeJwt } from "@/services/sellerAuthService";
import { SellerShell } from "./components/SellerShell";

const SellerProfilePage = () => {
  const { user, token, logout } = useAuth();
  const navigate = useNavigate();
  const claims = decodeJwt(token ?? localStorage.getItem("athena_jwt")) ?? {};
  const summary = useQuery<SellerSummary>({ queryKey: ["seller-summary"], queryFn: () => invoiceService.mineSummary(), retry: false });

  const phone = (claims.phoneNumber as string) || (claims.phone as string) || sellerAuthService.lastPhone() || "—";
  const business = (claims.businessName as string) || (claims.name as string) || user?.name || "—";
  const customerId = summary.data?.sellerCustomerId || (claims.customerId as string) || (claims.sub as string) || "—";
  const profileMissing = isSellerProfileMissing(summary.error);

  const signOut = () => {
    sellerAuthService.clear();
    logout();
    navigate("/login", { replace: true });
  };

  const Row = ({ icon: Icon, label, value, mono }: { icon: React.ComponentType<{ className?: string }>; label: string; value: string; mono?: boolean }) => (
    <div className="flex items-center gap-3 py-3">
      <span className="h-9 w-9 rounded-lg bg-accent/10 flex items-center justify-center shrink-0">
        <Icon className="h-4 w-4 text-accent" />
      </span>
      <div className="min-w-0">
        <p className="text-[11px] uppercase tracking-wider text-muted-foreground">{label}</p>
        <p className={mono ? "font-mono text-sm truncate" : "text-sm truncate"}>{value}</p>
      </div>
    </div>
  );

  return (
    <SellerShell title="Profile">
      <div className="space-y-4 animate-fade-in">
        <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5">
          <div className="flex items-center gap-4">
            <div className="h-14 w-14 rounded-full bg-gradient-to-br from-primary to-[hsl(var(--nemo-coral-deep))] flex items-center justify-center text-base font-bold text-white shadow-glow">
              {user?.initials || "S"}
            </div>
            <div className="min-w-0">
              <p className="font-heading text-lg font-semibold truncate">{user?.name ?? "Seller"}</p>
              <p className="text-sm text-muted-foreground">Seller account</p>
            </div>
          </div>
          <div className="divide-y mt-3">
            <Row icon={Building2} label="Business" value={business} />
            <Row icon={Phone} label="Mobile" value={phone} mono />
            <Row icon={Hash} label="Customer ID" value={profileMissing ? "Not yet created by the bank" : customerId} mono />
            <Row icon={ShieldCheck} label="Signed in via" value="Phone + one-time code" />
          </div>
        </div>

        <div data-nemo-card="" className="rounded-xl border bg-card p-4 sm:p-5 flex items-center justify-between">
          <div>
            <p className="text-sm font-medium">Appearance</p>
            <p className="text-xs text-muted-foreground">Switch between deep-water dark and light.</p>
          </div>
          <ThemeToggle className="border rounded-lg" />
        </div>

        <Button variant="outline" className="w-full h-12 text-destructive border-destructive/30 hover:bg-destructive/10 hover:text-destructive" onClick={signOut}>
          <LogOut className="h-4 w-4" /> Log out
        </Button>

        <p className="text-[11px] text-muted-foreground text-center">
          Nemo Invoice Finance {appConfig.demo ? "· demo environment" : ""} · every action is written to a tamper-evident ledger.
        </p>
      </div>
    </SellerShell>
  );
};

export default SellerProfilePage;
