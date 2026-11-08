import { NemoMark } from "@/components/NemoMark";
import { ThemeToggle } from "@/components/ThemeToggle";

/** Branded chrome for unauthenticated pages (buyer confirmation, public verify). */
export function PublicShell({ children, caption }: { children: React.ReactNode; caption?: string }) {
  return (
    <div className="min-h-screen bg-background text-foreground flex flex-col">
      <div className="h-1.5 nemo-stripes opacity-90" aria-hidden />
      <header className="px-4 sm:px-8 py-4 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <NemoMark className="h-9 w-9" />
          <div>
            <p className="font-heading text-base tracking-wide leading-none">Nemo</p>
            <p className="text-[11px] uppercase tracking-widest text-muted-foreground mt-0.5">{caption ?? "Invoice finance"}</p>
          </div>
        </div>
        <ThemeToggle />
      </header>
      <main className="flex-1 px-4 sm:px-8 pb-10 flex items-start justify-center">
        <div className="w-full max-w-xl mt-4 sm:mt-10">{children}</div>
      </main>
      <footer className="px-4 sm:px-8 py-4 text-[11px] text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 justify-between">
        <span>Nemo — Neobank in a Box</span>
        <span>Every action on this page is written to a tamper-evident ledger.</span>
      </footer>
    </div>
  );
}
