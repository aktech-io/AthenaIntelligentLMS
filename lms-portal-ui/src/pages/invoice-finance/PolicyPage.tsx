import { useEffect, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Scale, Save, RotateCcw, Loader2, Lock } from "lucide-react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useToast } from "@/hooks/use-toast";
import { invoiceService, shortHash, type Policy } from "@/services/invoiceService";
import { formatDateTime } from "@/lib/format";
import { LoadingState, ErrorState } from "./components/States";
import { CopyButton } from "./components/CopyButton";
import { useInvoiceRole } from "./components/useInvoiceRole";

const PolicyPage = () => {
  const { isAdmin } = useInvoiceRole();
  const { toast } = useToast();
  const qc = useQueryClient();
  const q = useQuery<Policy>({ queryKey: ["if-policy"], queryFn: () => invoiceService.getPolicy() });
  const [draft, setDraft] = useState("");
  useEffect(() => {
    if (q.data) setDraft(q.data.yaml ?? "");
  }, [q.data]);

  const save = useMutation({
    mutationFn: () => invoiceService.putPolicy(draft),
    onSuccess: (p) => {
      toast({ title: "Policy saved", description: `Version ${p.version} · ${shortHash(p.hash)}` });
      qc.invalidateQueries({ queryKey: ["if-policy"] });
    },
    onError: (e: Error) => toast({ title: "Policy rejected", description: e.message, variant: "destructive" }),
  });

  const dirty = (q.data?.yaml ?? "") !== draft;
  const lines = draft.split("\n").length;

  return (
    <DashboardLayout
      title="Advance Policy"
      subtitle="invoice.advance — rules evaluated by common/decision for every scored invoice"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Invoice Finance", href: "/invoice-finance" }, { label: "Policy" }]}
    >
      <div className="space-y-4 animate-fade-in">
        {q.isLoading ? (
          <LoadingState label="Loading policy…" />
        ) : q.isError ? (
          <ErrorState error={q.error} onRetry={() => q.refetch()} />
        ) : q.data ? (
          <Card>
            <CardHeader className="pb-3 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
              <div className="flex flex-wrap items-center gap-2">
                <CardTitle className="text-sm font-heading flex items-center gap-2">
                  <Scale className="h-4 w-4 text-primary" /> invoice.advance
                </CardTitle>
                <Badge variant="outline" className="font-mono text-[11px]">
                  v{q.data.version}
                </Badge>
                <span className="inline-flex items-center text-[11px] font-mono text-muted-foreground">
                  {shortHash(q.data.hash, 12)}
                  <CopyButton value={q.data.hash} />
                </span>
                {q.data.updatedAt && <span className="text-xs text-muted-foreground">updated {formatDateTime(q.data.updatedAt)}</span>}
                {dirty && (
                  <Badge className="bg-warning/15 text-warning border-warning/30" variant="outline">
                    unsaved
                  </Badge>
                )}
              </div>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" disabled={!dirty || save.isPending} onClick={() => setDraft(q.data!.yaml)}>
                  <RotateCcw className="h-3.5 w-3.5 mr-1" /> Revert
                </Button>
                <Button size="sm" disabled={!isAdmin || !dirty || save.isPending} onClick={() => save.mutate()} title={isAdmin ? "Validate and publish a new version" : "ADMIN role required"}>
                  {save.isPending ? <Loader2 className="h-3.5 w-3.5 mr-1 animate-spin" /> : isAdmin ? <Save className="h-3.5 w-3.5 mr-1" /> : <Lock className="h-3.5 w-3.5 mr-1" />}
                  Save as v{Number(q.data.version) + 1 || "next"}
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <div className="rounded-lg border bg-[hsl(var(--nemo-abyss))] text-[hsl(var(--nemo-stripe))] overflow-hidden">
                <div className="grid grid-cols-[3rem_1fr]">
                  <pre className="select-none text-right pr-3 py-3 text-[12px] leading-5 font-mono text-[hsl(var(--nemo-glass))]/60 bg-black/20" aria-hidden>
                    {Array.from({ length: lines }, (_, i) => i + 1).join("\n")}
                  </pre>
                  <textarea
                    value={draft}
                    onChange={(e) => setDraft(e.target.value)}
                    readOnly={!isAdmin}
                    spellCheck={false}
                    rows={Math.max(16, lines)}
                    className="w-full resize-y bg-transparent py-3 pr-4 pl-3 text-[12px] leading-5 font-mono outline-none placeholder:text-muted-foreground min-h-[24rem]"
                    onKeyDown={(e) => {
                      if (e.key === "Tab") {
                        e.preventDefault();
                        const t = e.currentTarget;
                        const s = t.selectionStart;
                        const v = t.value.slice(0, s) + "  " + t.value.slice(t.selectionEnd);
                        setDraft(v);
                        requestAnimationFrame(() => t.setSelectionRange(s + 2, s + 2));
                      }
                    }}
                  />
                </div>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">
                Saving validates the YAML server-side, bumps the version, recomputes the hash and writes a <code className="font-mono">policy.changed</code> ledger event. Existing decisions keep the hash they were evaluated with.
              </p>
            </CardContent>
          </Card>
        ) : null}
      </div>
    </DashboardLayout>
  );
};

export default PolicyPage;
