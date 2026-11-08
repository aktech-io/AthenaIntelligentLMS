import { useState } from "react";
import { useParams } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { CheckCircle2, AlertTriangle, Loader2, BadgeCheck, Ban, Link2Off } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { InputOTP, InputOTPGroup, InputOTPSlot } from "@/components/ui/input-otp";
import { cn } from "@/lib/utils";
import { invoiceService, type ConfirmAction, type ConfirmSummary } from "@/services/invoiceService";
import { formatMoney, formatDate } from "@/lib/format";
import { PublicShell } from "./PublicShell";

const ConfirmPage = () => {
  const { token = "" } = useParams();
  const q = useQuery<ConfirmSummary>({
    queryKey: ["public-confirm", token],
    queryFn: () => invoiceService.publicConfirmSummary(token),
    enabled: !!token,
    retry: false,
  });
  const [action, setAction] = useState<ConfirmAction>("CONFIRMED");
  const [otp, setOtp] = useState("");
  const [reason, setReason] = useState("");

  const submit = useMutation({
    mutationFn: () =>
      invoiceService.publicConfirm(token, {
        action,
        otp: otp || undefined,
        disputeReason: action === "DISPUTED" ? reason.trim() : undefined,
      }),
  });

  const s = q.data;
  const otpRequired = s?.otpRequired !== false;
  const canSubmit = !submit.isPending && (!otpRequired || otp.length === 6) && (action !== "DISPUTED" || reason.trim().length >= 3);

  const done = submit.data?.action ?? s?.action ?? null;

  return (
    <PublicShell caption="Buyer confirmation">
      {q.isLoading ? (
        <Card>
          <CardContent className="py-14 flex items-center justify-center text-muted-foreground text-sm">
            <Loader2 className="h-5 w-5 animate-spin mr-2" /> Loading invoice…
          </CardContent>
        </Card>
      ) : q.isError ? (
        <Card>
          <CardContent className="py-12 text-center">
            <Link2Off className="h-10 w-10 mx-auto text-destructive mb-3" />
            <h1 className="text-lg font-heading">This link isn't valid</h1>
            <p className="text-sm text-muted-foreground mt-1">{(q.error as Error).message}. Confirmation links expire after 5 days — ask the seller to resend.</p>
          </CardContent>
        </Card>
      ) : s ? (
        <div className="space-y-4">
          <Card>
            <CardHeader className="pb-3">
              <p className="text-xs uppercase tracking-widest text-muted-foreground">Please confirm this invoice</p>
              <CardTitle className="font-heading text-xl">
                {s.sellerName ?? "Your supplier"} is requesting finance against invoice {s.invoiceNumber ? `#${s.invoiceNumber}` : ""}
              </CardTitle>
              <p className="text-sm text-muted-foreground">
                Addressed to <span className="text-foreground">{s.buyerName ?? "you"}</span>. Confirming tells Nemo the goods or services were received and the amount is payable on the due date. It does not change who you pay or when.
              </p>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm">
                <div>
                  <p className="text-xs text-muted-foreground">Amount</p>
                  <p className="font-mono tabular-nums text-lg font-semibold">{formatMoney(s.total, s.currency, { decimals: 2 })}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Due date</p>
                  <p className="font-medium">{formatDate(s.dueDate)}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Issued</p>
                  <p>{formatDate(s.issueDate)}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Supplier</p>
                  <p>{s.sellerName ?? "—"}</p>
                </div>
              </div>
              {s.lineItems && s.lineItems.length > 0 && (
                <div className="mt-4 overflow-x-auto">
                  <table className="w-full text-xs">
                    <tbody className="tabular-nums">
                      {s.lineItems.map((li, i) => (
                        <tr key={i} className="border-t">
                          <td className="py-1.5 pr-2">{li.description}</td>
                          <td className="py-1.5 text-right text-muted-foreground whitespace-nowrap">
                            {li.qty} × {formatMoney(li.unitPrice, s.currency, { decimals: 2 })}
                          </td>
                          <td className="py-1.5 pl-3 text-right font-mono whitespace-nowrap">{formatMoney(li.amount, s.currency, { decimals: 2 })}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </CardContent>
          </Card>

          {done ? (
            <Card className={cn("border", done === "CONFIRMED" ? "border-accent/50" : done === "DISPUTED" ? "border-destructive/50" : "border-warning/50")}>
              <CardContent className="py-10 text-center">
                {done === "CONFIRMED" ? (
                  <CheckCircle2 className="h-12 w-12 mx-auto text-accent mb-3" />
                ) : done === "DISPUTED" ? (
                  <Ban className="h-12 w-12 mx-auto text-destructive mb-3" />
                ) : (
                  <BadgeCheck className="h-12 w-12 mx-auto text-warning mb-3" />
                )}
                <h2 className="text-lg font-heading">
                  {done === "CONFIRMED" ? "Thank you — invoice confirmed" : done === "DISPUTED" ? "Dispute recorded" : "Marked as already paid"}
                </h2>
                <p className="text-sm text-muted-foreground mt-1 max-w-sm mx-auto">
                  {submit.data?.message ??
                    (done === "CONFIRMED"
                      ? `Please pay ${s.sellerName ?? "the supplier"} as usual by ${formatDate(s.dueDate)}. You will receive payment instructions if they change.`
                      : done === "DISPUTED"
                        ? "The supplier has been notified and no finance will be advanced against this invoice."
                        : "The supplier has been notified. No further action is needed.")}
                </p>
                {s.actedAt && !submit.data && <p className="text-xs text-muted-foreground mt-3">Recorded {formatDate(s.actedAt)}</p>}
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-sm font-heading">Your response</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-2">
                  {(
                    [
                      { v: "CONFIRMED", label: "Confirm", hint: "Received and payable", icon: CheckCircle2, cls: "data-[on=true]:border-accent data-[on=true]:bg-accent/10" },
                      { v: "DISPUTED", label: "Dispute", hint: "Something is wrong", icon: AlertTriangle, cls: "data-[on=true]:border-destructive data-[on=true]:bg-destructive/10" },
                      { v: "ALREADY_PAID", label: "Already paid", hint: "We settled this", icon: BadgeCheck, cls: "data-[on=true]:border-warning data-[on=true]:bg-warning/10" },
                    ] as const
                  ).map((o) => (
                    <button
                      key={o.v}
                      type="button"
                      data-on={action === o.v}
                      onClick={() => setAction(o.v)}
                      className={cn("rounded-lg border p-3 text-left transition-colors hover:bg-muted/50", o.cls)}
                    >
                      <o.icon className="h-4 w-4 mb-1.5" />
                      <p className="text-sm font-medium">{o.label}</p>
                      <p className="text-[11px] text-muted-foreground">{o.hint}</p>
                    </button>
                  ))}
                </div>

                {action === "DISPUTED" && (
                  <div className="space-y-1.5">
                    <Label htmlFor="dispute">What's wrong?</Label>
                    <Textarea id="dispute" rows={3} value={reason} onChange={(e) => setReason(e.target.value)} placeholder="e.g. Goods not delivered, amount differs from PO, duplicate invoice" />
                  </div>
                )}

                {otpRequired && (
                  <div className="space-y-1.5">
                    <Label>One-time code</Label>
                    <InputOTP maxLength={6} value={otp} onChange={setOtp}>
                      <InputOTPGroup>
                        {[0, 1, 2, 3, 4, 5].map((i) => (
                          <InputOTPSlot key={i} index={i} className="font-mono" />
                        ))}
                      </InputOTPGroup>
                    </InputOTP>
                    <p className="text-[11px] text-muted-foreground">Sent by SMS to the number on file for {s.buyerName ?? "your company"}.</p>
                  </div>
                )}

                {submit.isError && (
                  <p className="text-sm text-destructive flex items-center gap-1.5">
                    <AlertTriangle className="h-4 w-4" /> {(submit.error as Error).message}
                  </p>
                )}

                <Button
                  className={cn("w-full", action === "DISPUTED" && "bg-destructive text-destructive-foreground hover:bg-destructive/90")}
                  disabled={!canSubmit}
                  onClick={() => submit.mutate()}
                >
                  {submit.isPending && <Loader2 className="h-4 w-4 mr-1 animate-spin" />}
                  {action === "CONFIRMED" ? "Confirm invoice" : action === "DISPUTED" ? "Submit dispute" : "Mark as already paid"}
                </Button>
                {s.expiresAt && <p className="text-[11px] text-center text-muted-foreground">This link expires {formatDate(s.expiresAt)}.</p>}
              </CardContent>
            </Card>
          )}
        </div>
      ) : null}
    </PublicShell>
  );
};

export default ConfirmPage;
