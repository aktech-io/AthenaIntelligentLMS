import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription,
} from "@/components/ui/dialog";
import {
  Accordion, AccordionContent, AccordionItem, AccordionTrigger,
} from "@/components/ui/accordion";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Settings2, ShieldCheck, Zap, BookOpen, AlertTriangle, Info } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fraudService, type FraudRule, type AlertSeverity } from "@/services/fraudService";
import { useToast } from "@/hooks/use-toast";

// ─── Rule Documentation ─────────────────────────────────────────────────────
// Built-in reference for every fraud detection rule — what it detects,
// how it works, and what each parameter controls.

interface ParamDoc {
  key: string;
  label: string;
  description: string;
  type: "number" | "percentage" | "days" | "hours" | "minutes" | "count";
  defaultValue: number;
  unit?: string;
}

interface RuleDoc {
  summary: string;
  howItWorks: string;
  example: string;
  params: ParamDoc[];
  regulatoryBasis?: string;
}

const RULE_DOCS: Record<string, RuleDoc> = {
  LARGE_SINGLE_TXN: {
    summary: "Flags any single transaction that exceeds the Cash Transaction Report (CTR) reporting threshold.",
    howItWorks: "Compares the transaction amount against the configured threshold. Any transaction above this limit triggers an immediate alert for compliance review.",
    example: "A customer deposits KES 2,500,000 in a single cash transaction. Since this exceeds the 1,000,000 threshold, the system raises a HIGH severity alert.",
    params: [
      { key: "threshold", label: "Amount Threshold", description: "Transaction amount above which an alert is triggered (in KES)", type: "number", defaultValue: 1000000, unit: "KES" },
    ],
    regulatoryBasis: "Central Bank of Kenya AML/CFT regulations require reporting of cash transactions above KES 1,000,000.",
  },
  STRUCTURING: {
    summary: "Detects 'smurfing' — multiple transactions deliberately kept below the reporting threshold that aggregate above it.",
    howItWorks: "Within the configured time window, counts all transactions from the same customer that are individually below the per-transaction ceiling. If 3+ such transactions sum to more than the threshold, it indicates potential structuring to evade CTR reporting.",
    example: "A customer makes 5 deposits of KES 300,000 each within 24 hours (total KES 1,500,000). Each is under the 999,999 ceiling, but the aggregate exceeds 1,000,000.",
    params: [
      { key: "threshold", label: "Aggregate Threshold", description: "Total amount across transactions that triggers the alert", type: "number", defaultValue: 1000000, unit: "KES" },
      { key: "windowHours", label: "Time Window", description: "Hours to look back for aggregating transactions", type: "hours", defaultValue: 24 },
      { key: "minTransactions", label: "Minimum Transactions", description: "Minimum number of transactions required to consider it structuring", type: "count", defaultValue: 3 },
      { key: "perTxnCeiling", label: "Per-Transaction Ceiling", description: "Maximum single transaction amount — transactions above this are not considered structuring", type: "number", defaultValue: 999999, unit: "KES" },
    ],
    regulatoryBasis: "Structuring is a federal offense under most AML frameworks. It is specifically addressed in FATF Recommendation 20.",
  },
  ROUND_AMOUNT_PATTERN: {
    summary: "Detects unusual frequency of round-number transactions, which can indicate layering or structuring.",
    howItWorks: "Checks if the transaction amount is a round number (divisible by the threshold). Tracks how many round-amount transactions a customer has made in the time window. Triggers when the count exceeds the minimum.",
    example: "A customer makes 6 transactions of exactly KES 50,000, KES 100,000, KES 200,000, etc. within 24 hours — all perfectly round numbers.",
    params: [
      { key: "minRoundTxns", label: "Minimum Round Transactions", description: "How many round-number transactions trigger the alert", type: "count", defaultValue: 5 },
      { key: "windowHours", label: "Time Window", description: "Hours to look back for counting round transactions", type: "hours", defaultValue: 24 },
      { key: "roundThreshold", label: "Round Number Divisor", description: "Amounts divisible by this are considered 'round' (e.g., 10000 means 10K, 20K, etc.)", type: "number", defaultValue: 10000, unit: "KES" },
    ],
  },
  HIGH_VELOCITY_1H: {
    summary: "Flags customers with an unusually high number of transactions in a 1-hour window.",
    howItWorks: "Counts all transactions from the same customer within the past hour. If the count exceeds the maximum, the rule triggers. This catches automated fraud, card testing, and rapid account draining.",
    example: "A customer makes 15 transactions in 45 minutes, triggering the 10-transaction-per-hour threshold.",
    params: [
      { key: "maxTransactions", label: "Max Transactions", description: "Maximum transactions allowed in the window before triggering", type: "count", defaultValue: 10 },
      { key: "windowMinutes", label: "Window (minutes)", description: "Time window for counting transactions", type: "minutes", defaultValue: 60 },
    ],
  },
  HIGH_VELOCITY_24H: {
    summary: "Flags customers with excessive transactions over a 24-hour period.",
    howItWorks: "Same as the 1-hour rule but over a full day. Catches sustained unusual activity that the 1-hour rule might miss if transactions are spread across hours.",
    example: "A customer makes 60 transactions spread throughout the day (2-3 per hour) — normal hourly velocity but abnormal daily volume.",
    params: [
      { key: "maxTransactions", label: "Max Transactions", description: "Maximum transactions allowed in 24 hours", type: "count", defaultValue: 50 },
      { key: "windowMinutes", label: "Window (minutes)", description: "Time window in minutes (1440 = 24 hours)", type: "minutes", defaultValue: 1440 },
    ],
  },
  RAPID_FUND_MOVEMENT: {
    summary: "Detects the 'pass-through' pattern where funds are received and immediately transferred out.",
    howItWorks: "When a transfer is completed, checks if the customer had another transfer within the configured window. Multiple transfers in a short period from the same account suggest the account is being used as a conduit for money laundering.",
    example: "An account receives KES 500,000 and transfers out KES 490,000 within 10 minutes — classic pass-through behavior.",
    params: [
      { key: "windowMinutes", label: "Window (minutes)", description: "Time window to detect rapid in-and-out fund movement", type: "minutes", defaultValue: 15 },
    ],
    regulatoryBasis: "Pass-through accounts are a key indicator of money laundering (layering phase) under FATF guidelines.",
  },
  APPLICATION_STACKING: {
    summary: "Detects customers submitting many loan applications in a short period.",
    howItWorks: "Counts loan applications from the same customer within the window. Exceeding the maximum suggests either fraud (using multiple identities or products) or desperation borrowing.",
    example: "A customer submits 7 loan applications across different products within 30 days.",
    params: [
      { key: "maxApplications", label: "Max Applications", description: "Maximum allowed applications in the window", type: "count", defaultValue: 5 },
      { key: "windowDays", label: "Window (days)", description: "Days to look back for counting applications", type: "days", defaultValue: 30 },
    ],
  },
  EARLY_PAYOFF_SUSPICIOUS: {
    summary: "Flags loans paid off suspiciously quickly after disbursement, which may indicate money laundering.",
    howItWorks: "When a loan is closed, checks how many days elapsed since disbursement. Early payoff with legitimate funds is normal, but very early payoff can indicate the loan was used to legitimize illicit funds.",
    example: "A KES 1,000,000 loan is fully repaid 5 days after disbursement — far faster than the expected term.",
    params: [
      { key: "minDaysForAlert", label: "Minimum Days", description: "Loans closed within this many days of disbursement trigger an alert", type: "days", defaultValue: 30 },
    ],
    regulatoryBasis: "Early loan repayment is a recognized AML typology for trade-based money laundering.",
  },
  LOAN_CYCLING: {
    summary: "Detects rapid loan close-and-reapply patterns that may indicate layering.",
    howItWorks: "When a new loan application is submitted, checks if the customer had a recent loan application within the window. Multiple applications in quick succession after closures suggests cycling funds through the loan system.",
    example: "Customer closes a loan on Monday and applies for a new one on Wednesday — within the 7-day window.",
    params: [
      { key: "windowDays", label: "Window (days)", description: "Days after a previous application within which a new application triggers the alert", type: "days", defaultValue: 7 },
    ],
  },
  DORMANT_REACTIVATION: {
    summary: "Flags sudden activity on accounts that have been inactive for an extended period.",
    howItWorks: "When a credit or unfreezing event occurs, checks when the account was last active. Long-dormant accounts suddenly receiving funds can indicate account takeover or use of 'sleeping' mule accounts.",
    example: "An account dormant for 8 months suddenly receives a KES 500,000 credit.",
    params: [
      { key: "dormantDays", label: "Dormant Period (days)", description: "Number of inactive days after which reactivation triggers an alert", type: "days", defaultValue: 180 },
    ],
  },
  KYC_BYPASS_ATTEMPT: {
    summary: "Flags transactions on accounts where KYC verification is pending or failed.",
    howItWorks: "Checks the KYC status of the customer when a transaction occurs. If KYC is not PASSED or APPROVED, the transaction may violate compliance requirements. No configurable parameters — any non-verified transaction triggers the rule.",
    example: "A customer with PENDING KYC status initiates a KES 100,000 transfer.",
    params: [],
    regulatoryBasis: "KYC requirements are mandated by the Proceeds of Crime and Anti-Money Laundering Act (POCAMLA).",
  },
  OVERDRAFT_RAPID_DRAW: {
    summary: "Flags immediate full drawdown of newly approved overdraft facilities.",
    howItWorks: "When an overdraft is drawn, calculates what percentage of the limit was used. Drawing 90%+ of the facility immediately after approval can indicate the facility was obtained fraudulently.",
    example: "A KES 500,000 overdraft is approved and KES 480,000 (96%) is drawn within the first hour.",
    params: [
      { key: "drawdownThresholdPercent", label: "Drawdown Threshold (%)", description: "Percentage of overdraft limit that triggers an alert when drawn at once", type: "percentage", defaultValue: 90 },
      { key: "windowMinutes", label: "Window (minutes)", description: "Time window after facility approval", type: "minutes", defaultValue: 60 },
    ],
  },
  BNPL_ABUSE: {
    summary: "Detects rapid sequential Buy-Now-Pay-Later approvals with minimal deposits.",
    howItWorks: "Counts BNPL approvals for a customer within the window. Multiple rapid BNPL approvals can indicate the customer is over-leveraging or committing first-party fraud.",
    example: "A customer gets 4 BNPL approvals in 5 days across different merchants.",
    params: [
      { key: "maxApprovals", label: "Max Approvals", description: "Maximum BNPL approvals allowed in the window", type: "count", defaultValue: 3 },
      { key: "windowDays", label: "Window (days)", description: "Days to look back for counting approvals", type: "days", defaultValue: 7 },
      { key: "minDepositPercent", label: "Min Deposit (%)", description: "Minimum deposit percentage required (currently informational)", type: "percentage", defaultValue: 5 },
    ],
  },
  PAYMENT_REVERSAL_ABUSE: {
    summary: "Flags customers with an abnormally high ratio of reversed to completed payments.",
    howItWorks: "When a payment reversal occurs, checks the customer's reversal-to-total-payment ratio. A high reversal rate can indicate chargeback fraud, return abuse, or exploitation of payment processing errors.",
    example: "A customer has 8 reversals out of 15 total payments (53% reversal rate) — well above the 30% threshold.",
    params: [
      { key: "maxReversalPercent", label: "Max Reversal Rate (%)", description: "Reversal percentage above which the alert triggers", type: "percentage", defaultValue: 30 },
      { key: "minPayments", label: "Min Payments", description: "Minimum total payments required before evaluating the ratio (avoids false positives on new accounts)", type: "count", defaultValue: 5 },
      { key: "windowDays", label: "Window (days)", description: "Days to look back for payment history", type: "days", defaultValue: 30 },
    ],
  },
  OVERPAYMENT: {
    summary: "Flags payments that exceed the total outstanding loan balance.",
    howItWorks: "Compares the payment amount against the loan's outstanding balance. Overpaying a loan and then requesting a refund is a recognized money laundering technique.",
    example: "A loan with KES 200,000 outstanding receives a payment of KES 350,000 (175% of balance).",
    params: [
      { key: "overpaymentThresholdPercent", label: "Overpayment Threshold (%)", description: "Payment as percentage of outstanding balance that triggers an alert", type: "percentage", defaultValue: 110 },
    ],
    regulatoryBasis: "Loan overpayment followed by refund requests is a FATF-recognized typology for money laundering through lending institutions.",
  },
  SUSPICIOUS_WRITEOFF: {
    summary: "Flags write-offs on loans that had recent payment activity — potential collusion.",
    howItWorks: "When a loan is written off, checks if it was recently disbursed (as a proxy for recent activity). A loan with recent payments being written off may indicate insider fraud or collusion between staff and borrowers.",
    example: "A loan disbursed 15 days ago is written off despite the borrower making a payment last week.",
    params: [
      { key: "recentPaymentDays", label: "Recent Activity Window (days)", description: "Write-offs within this many days of disbursement trigger an alert", type: "days", defaultValue: 30 },
    ],
  },
  WATCHLIST_MATCH: {
    summary: "Flags customers who match entries on PEP, sanctions, or internal blacklists.",
    howItWorks: "When a customer is created, updated, or submits a loan application, their name, national ID, and phone are checked against all active watchlist entries (PEP, OFAC/UN sanctions, internal blacklists, adverse media). Any match triggers a CRITICAL alert.",
    example: "A new loan applicant's national ID matches an entry on the internal blacklist.",
    params: [],
    regulatoryBasis: "Watchlist screening is required by UN Security Council resolutions, OFAC regulations, and local PEP screening requirements under POCAMLA.",
  },
  PROMISE_TO_PAY_GAMING: {
    summary: "Detects customers repeatedly making unfulfilled payment promises to delay collections.",
    howItWorks: "Tracks days-past-due (DPD) updates for a customer. Multiple DPD events without actual payments suggest the customer is gaming the collections process by making promises they don't keep.",
    example: "A customer has made 5 payment promises in 90 days but fulfilled none — each promise resets the collections escalation.",
    params: [
      { key: "maxUnfulfilledPromises", label: "Max Unfulfilled Promises", description: "Number of broken promises before triggering an alert", type: "count", defaultValue: 3 },
      { key: "windowDays", label: "Window (days)", description: "Days to look back for counting promises", type: "days", defaultValue: 90 },
    ],
  },
};

// ─── Styles ──────────────────────────────────────────────────────────────────

const severityColor: Record<AlertSeverity, string> = {
  CRITICAL: "bg-red-500/15 text-red-600 border-red-500/30",
  HIGH: "bg-destructive/15 text-destructive border-destructive/30",
  MEDIUM: "bg-warning/15 text-warning border-warning/30",
  LOW: "bg-info/15 text-info border-info/30",
};

const categoryColor: Record<string, string> = {
  TRANSACTION: "bg-blue-500/10 text-blue-600 border-blue-500/20",
  AML: "bg-red-500/10 text-red-600 border-red-500/20",
  VELOCITY: "bg-orange-500/10 text-orange-600 border-orange-500/20",
  APPLICATION: "bg-purple-500/10 text-purple-600 border-purple-500/20",
  COMPLIANCE: "bg-pink-500/10 text-pink-600 border-pink-500/20",
  ACCOUNT: "bg-teal-500/10 text-teal-600 border-teal-500/20",
  OVERDRAFT: "bg-cyan-500/10 text-cyan-600 border-cyan-500/20",
  COLLECTIONS: "bg-amber-500/10 text-amber-600 border-amber-500/20",
  INTERNAL: "bg-gray-500/10 text-gray-600 border-gray-500/20",
};

const unitLabel = (type: ParamDoc["type"], unit?: string) => {
  if (unit) return unit;
  switch (type) {
    case "percentage": return "%";
    case "days": return "days";
    case "hours": return "hours";
    case "minutes": return "min";
    case "count": return "";
    default: return "";
  }
};

// ─── Component ───────────────────────────────────────────────────────────────

const FraudRulesPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [editRule, setEditRule] = useState<FraudRule | null>(null);
  const [editSeverity, setEditSeverity] = useState("");
  const [editParams, setEditParams] = useState<Record<string, string>>({});
  const [showReference, setShowReference] = useState(false);

  const { data: rules, isLoading } = useQuery({
    queryKey: ["fraud", "rules"],
    queryFn: () => fraudService.listRules(),
    staleTime: 60_000,
    retry: false,
  });

  const toggleMutation = useMutation({
    mutationFn: (params: { id: string; enabled: boolean }) =>
      fraudService.updateRule(params.id, { enabled: params.enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud", "rules"] });
      toast({ title: "Rule updated" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const updateMutation = useMutation({
    mutationFn: () => {
      if (!editRule) throw new Error("No rule selected");
      const doc = RULE_DOCS[editRule.ruleCode];
      const params: Record<string, unknown> = { ...(editRule.parameters ?? {}) };
      if (doc?.params) {
        for (const p of doc.params) {
          const val = editParams[p.key];
          if (val !== undefined && val !== "") {
            params[p.key] = Number(val);
          }
        }
      }
      return fraudService.updateRule(editRule.id, {
        severity: editSeverity || undefined,
        parameters: params,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["fraud", "rules"] });
      setEditRule(null);
      toast({ title: "Rule configuration saved" });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const openEditDialog = (rule: FraudRule) => {
    setEditRule(rule);
    setEditSeverity(rule.severity);
    const paramValues: Record<string, string> = {};
    const doc = RULE_DOCS[rule.ruleCode];
    if (doc?.params) {
      for (const p of doc.params) {
        const current = rule.parameters?.[p.key];
        paramValues[p.key] = current !== undefined ? String(current) : String(p.defaultValue);
      }
    }
    setEditParams(paramValues);
  };

  const ruleDoc = editRule ? RULE_DOCS[editRule.ruleCode] : null;

  // Group rules by category for the reference guide
  const rulesByCategory: Record<string, typeof RULE_DOCS[string] & { code: string }[]> = {};
  for (const [code, doc] of Object.entries(RULE_DOCS)) {
    const rule = rules?.find(r => r.ruleCode === code);
    const cat = rule?.category ?? "OTHER";
    if (!rulesByCategory[cat]) rulesByCategory[cat] = [];
    rulesByCategory[cat].push({ ...doc, code });
  }

  return (
    <DashboardLayout
      title="Fraud Detection Rules"
      subtitle="Configure and manage fraud detection rules and thresholds"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Rules" }]}
    >
      <div className="space-y-6 animate-fade-in">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Badge className="bg-success/10 text-success border-success/20 gap-1.5 px-3 py-1">
              <ShieldCheck className="h-3 w-3" />
              {rules?.filter((r) => r.enabled).length ?? 0} Active Rules
            </Badge>
            <span className="text-xs text-muted-foreground">
              {rules?.length ?? 0} total rules across {new Set(rules?.map((r) => r.category)).size ?? 0} categories
            </span>
          </div>
          <Button variant="outline" size="sm" className="gap-1.5 text-xs" onClick={() => setShowReference(!showReference)}>
            <BookOpen className="h-3.5 w-3.5" />
            {showReference ? "Hide" : "Show"} Rule Reference Guide
          </Button>
        </div>

        {/* ── Rule Reference Guide ─────────────────────────────────────── */}
        {showReference && (
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm flex items-center gap-2">
                <BookOpen className="h-4 w-4" />
                Fraud Detection Rule Reference
              </CardTitle>
              <CardDescription className="text-xs">
                Complete documentation for all {Object.keys(RULE_DOCS).length} detection rules. Click any rule to see how it works, what it detects, and how to configure its parameters.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Accordion type="multiple" className="w-full">
                {Object.entries(rulesByCategory).map(([category, catRules]) => (
                  <AccordionItem key={category} value={category}>
                    <AccordionTrigger className="text-xs font-medium py-2">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className={`text-[10px] ${categoryColor[category] ?? ""}`}>
                          {category}
                        </Badge>
                        <span>{catRules.length} rules</span>
                      </div>
                    </AccordionTrigger>
                    <AccordionContent>
                      <div className="space-y-4 pl-1">
                        {catRules.map((doc) => {
                          const matchingRule = rules?.find(r => r.ruleCode === doc.code);
                          return (
                            <div key={doc.code} className="border rounded-lg p-3 space-y-2">
                              <div className="flex items-center justify-between">
                                <div className="flex items-center gap-2">
                                  <code className="text-xs font-mono font-bold">{doc.code}</code>
                                  {matchingRule && (
                                    <Badge variant="outline" className={`text-[9px] ${severityColor[matchingRule.severity]}`}>
                                      {matchingRule.severity}
                                    </Badge>
                                  )}
                                  {matchingRule && (
                                    <span className={`text-[10px] ${matchingRule.enabled ? "text-green-600" : "text-muted-foreground"}`}>
                                      {matchingRule.enabled ? "Active" : "Disabled"}
                                    </span>
                                  )}
                                </div>
                              </div>
                              <p className="text-xs font-medium">{doc.summary}</p>
                              <p className="text-[11px] text-muted-foreground">{doc.howItWorks}</p>
                              <div className="bg-muted/50 rounded p-2">
                                <p className="text-[10px] text-muted-foreground"><strong>Example:</strong> {doc.example}</p>
                              </div>
                              {doc.params.length > 0 && (
                                <div className="space-y-1">
                                  <p className="text-[10px] font-medium text-muted-foreground uppercase tracking-wider">Parameters</p>
                                  {doc.params.map((p) => {
                                    const currentVal = matchingRule?.parameters?.[p.key];
                                    return (
                                      <div key={p.key} className="flex items-center justify-between text-[11px] py-0.5">
                                        <span className="text-muted-foreground">
                                          <code className="font-mono text-[10px]">{p.key}</code> — {p.description}
                                        </span>
                                        <span className="font-mono font-medium ml-2 whitespace-nowrap">
                                          {currentVal !== undefined ? String(currentVal) : String(p.defaultValue)}
                                          {unitLabel(p.type, p.unit) ? ` ${unitLabel(p.type, p.unit)}` : ""}
                                        </span>
                                      </div>
                                    );
                                  })}
                                </div>
                              )}
                              {doc.regulatoryBasis && (
                                <p className="text-[10px] text-blue-600 flex items-start gap-1">
                                  <Info className="h-3 w-3 mt-0.5 shrink-0" />
                                  {doc.regulatoryBasis}
                                </p>
                              )}
                            </div>
                          );
                        })}
                      </div>
                    </AccordionContent>
                  </AccordionItem>
                ))}
              </Accordion>
            </CardContent>
          </Card>
        )}

        {/* ── Rules Table ──────────────────────────────────────────────── */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Settings2 className="h-4 w-4" />
              Rule Configuration
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">{Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}</div>
            ) : (
              <div className="overflow-x-auto">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="text-xs w-[60px]">Active</TableHead>
                      <TableHead className="text-xs">Code</TableHead>
                      <TableHead className="text-xs">Name</TableHead>
                      <TableHead className="text-xs">Category</TableHead>
                      <TableHead className="text-xs">Severity</TableHead>
                      <TableHead className="text-xs">Key Thresholds</TableHead>
                      <TableHead className="text-xs max-w-[250px]">Description</TableHead>
                      <TableHead className="text-xs text-right">Config</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {(rules ?? []).map((rule) => {
                      const doc = RULE_DOCS[rule.ruleCode];
                      const thresholdSummary = doc?.params.slice(0, 2).map(p => {
                        const val = rule.parameters?.[p.key] ?? p.defaultValue;
                        const u = unitLabel(p.type, p.unit);
                        return `${p.label}: ${val}${u ? ` ${u}` : ""}`;
                      }).join(" · ") || "—";
                      return (
                        <TableRow key={rule.id} className="table-row-hover">
                          <TableCell>
                            <Switch
                              checked={rule.enabled}
                              onCheckedChange={(checked) =>
                                toggleMutation.mutate({ id: rule.id, enabled: checked })
                              }
                            />
                          </TableCell>
                          <TableCell className="text-xs font-mono font-medium">{rule.ruleCode}</TableCell>
                          <TableCell className="text-xs">{rule.ruleName}</TableCell>
                          <TableCell>
                            <Badge variant="outline" className={`text-[10px] ${categoryColor[rule.category] ?? ""}`}>
                              {rule.category}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className={`text-[10px] ${severityColor[rule.severity]}`}>
                              {rule.severity}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-[10px] text-muted-foreground max-w-[200px] truncate font-mono" title={thresholdSummary}>
                            {thresholdSummary}
                          </TableCell>
                          <TableCell className="text-xs text-muted-foreground max-w-[250px] truncate" title={doc?.summary ?? rule.description ?? ""}>
                            {doc?.summary ?? rule.description ?? "—"}
                          </TableCell>
                          <TableCell className="text-right">
                            <Button variant="ghost" size="sm" className="h-7 px-2 text-xs gap-1"
                              onClick={() => openEditDialog(rule)}>
                              <Zap className="h-3 w-3" />
                              Edit
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* ── Edit Rule Dialog ──────────────────────────────────────────── */}
      <Dialog open={!!editRule} onOpenChange={() => setEditRule(null)}>
        <DialogContent className="sm:max-w-xl max-h-[85vh] overflow-y-auto">
          {editRule && (
            <>
              <DialogHeader>
                <DialogTitle className="text-sm flex items-center gap-2">
                  <span className="font-mono">{editRule.ruleCode}</span>
                  <Badge variant="outline" className={`text-[10px] ${categoryColor[editRule.category]}`}>
                    {editRule.category}
                  </Badge>
                  <Badge variant="outline" className={`text-[10px] ${severityColor[editRule.severity]}`}>
                    {editRule.severity}
                  </Badge>
                </DialogTitle>
                <DialogDescription className="text-xs">
                  {editRule.ruleName}
                </DialogDescription>
              </DialogHeader>

              <div className="space-y-4">
                {/* Rule documentation */}
                {ruleDoc && (
                  <div className="bg-muted/30 rounded-lg p-3 space-y-2 border">
                    <p className="text-xs">{ruleDoc.summary}</p>
                    <p className="text-[11px] text-muted-foreground">{ruleDoc.howItWorks}</p>
                    <div className="bg-background rounded p-2 border">
                      <p className="text-[10px] text-muted-foreground"><strong>Example:</strong> {ruleDoc.example}</p>
                    </div>
                    {ruleDoc.regulatoryBasis && (
                      <p className="text-[10px] text-blue-600 flex items-start gap-1">
                        <AlertTriangle className="h-3 w-3 mt-0.5 shrink-0" />
                        <strong>Regulatory:</strong> {ruleDoc.regulatoryBasis}
                      </p>
                    )}
                  </div>
                )}

                <Separator />

                {/* Severity */}
                <div>
                  <Label className="text-xs">Alert Severity</Label>
                  <p className="text-[10px] text-muted-foreground mb-1">Determines how urgently this alert is treated in triage</p>
                  <Select value={editSeverity} onValueChange={setEditSeverity}>
                    <SelectTrigger className="mt-1 text-xs h-9"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="LOW">Low — Informational, review in batch</SelectItem>
                      <SelectItem value="MEDIUM">Medium — Review within 24 hours</SelectItem>
                      <SelectItem value="HIGH">High — Review within 4 hours</SelectItem>
                      <SelectItem value="CRITICAL">Critical — Immediate review required</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {/* Parameters — individual fields instead of raw JSON */}
                {ruleDoc && ruleDoc.params.length > 0 && (
                  <div className="space-y-3">
                    <Label className="text-xs">Rule Parameters</Label>
                    {ruleDoc.params.map((p) => (
                      <div key={p.key}>
                        <div className="flex items-center justify-between">
                          <Label className="text-[11px] font-medium">{p.label}</Label>
                          <span className="text-[10px] text-muted-foreground font-mono">
                            default: {p.defaultValue}{unitLabel(p.type, p.unit) ? ` ${unitLabel(p.type, p.unit)}` : ""}
                          </span>
                        </div>
                        <p className="text-[10px] text-muted-foreground mb-1">{p.description}</p>
                        <div className="flex items-center gap-2">
                          <Input
                            type="number"
                            value={editParams[p.key] ?? ""}
                            onChange={(e) => setEditParams({ ...editParams, [p.key]: e.target.value })}
                            className="text-xs font-mono h-8 w-40"
                          />
                          {unitLabel(p.type, p.unit) && (
                            <span className="text-[10px] text-muted-foreground">{unitLabel(p.type, p.unit)}</span>
                          )}
                        </div>
                      </div>
                    ))}
                  </div>
                )}

                {ruleDoc && ruleDoc.params.length === 0 && (
                  <p className="text-[11px] text-muted-foreground italic">
                    This rule has no configurable parameters — it triggers based on contextual conditions (e.g., KYC status, watchlist matches).
                  </p>
                )}

                {/* Monitored events */}
                <div>
                  <Label className="text-xs">Monitored Events</Label>
                  <div className="flex flex-wrap gap-1 mt-1">
                    {editRule.eventTypes?.split(",").map((evt) => (
                      <Badge key={evt} variant="outline" className="text-[9px] font-mono">
                        {evt.trim()}
                      </Badge>
                    ))}
                  </div>
                </div>
              </div>

              <DialogFooter>
                <Button variant="outline" size="sm" onClick={() => setEditRule(null)}>Cancel</Button>
                <Button size="sm" disabled={updateMutation.isPending} onClick={() => updateMutation.mutate()}>
                  {updateMutation.isPending ? "Saving..." : "Save Changes"}
                </Button>
              </DialogFooter>
            </>
          )}
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default FraudRulesPage;
