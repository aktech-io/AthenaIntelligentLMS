import { useEffect, useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { ShieldCheck, Save } from "lucide-react";
import { useToast } from "@/hooks/use-toast";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  approvalService,
  type ControlConfig,
} from "@/services/approvalService";

const operationLabels: Record<string, string> = {
  ACCOUNT_CREDIT: "Account Credit (Deposit)",
  ACCOUNT_DEBIT: "Account Debit (Withdrawal)",
  TRANSFER: "Transfer",
  ACCOUNT_CLOSE: "Account Closure",
};

const loanOperationLabels: Record<string, string> = {
  LOAN_APPROVE: "Loan Approval",
  LOAN_DISBURSE: "Loan Disbursement",
};

const LOAN_OPERATIONS = ["LOAN_APPROVE", "LOAN_DISBURSE"];

interface RowState {
  enabled: boolean;
  threshold: string;
}

const ControlSettingsPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  const { data: configs, isLoading } = useQuery({
    queryKey: ["control-config"],
    queryFn: () => approvalService.listControlConfig(),
    retry: false,
  });

  const [draft, setDraft] = useState<Record<string, RowState>>({});

  useEffect(() => {
    if (configs) {
      const next: Record<string, RowState> = {};
      configs.forEach((c) => {
        next[c.operation] = {
          enabled: c.enabled,
          threshold: String(c.thresholdAmount ?? 0),
        };
      });
      setDraft(next);
    }
  }, [configs]);

  const saveMutation = useMutation({
    mutationFn: (vars: { operation: string; enabled: boolean; threshold: number }) =>
      approvalService.updateControlConfig(vars.operation, vars.enabled, vars.threshold),
    onSuccess: (data, vars) => {
      toast({
        title: "Saved",
        description: `Dual-control settings for ${operationLabels[vars.operation] ?? vars.operation} updated.`,
      });
      queryClient.setQueryData(["control-config"], data);
    },
    onError: (err: Error) => {
      toast({ title: "Save Failed", description: err.message, variant: "destructive" });
    },
  });

  const { data: loanConfigs, isLoading: loanLoading } = useQuery({
    queryKey: ["loan-control-config"],
    queryFn: () => approvalService.listLoanControlConfig(),
    retry: false,
  });

  const [loanDraft, setLoanDraft] = useState<Record<string, RowState>>({});

  useEffect(() => {
    if (loanConfigs) {
      const next: Record<string, RowState> = {};
      loanConfigs.forEach((c) => {
        next[c.operation] = {
          enabled: c.enabled,
          threshold: String(c.thresholdAmount ?? 0),
        };
      });
      setLoanDraft(next);
    }
  }, [loanConfigs]);

  const saveLoanMutation = useMutation({
    mutationFn: (vars: { operation: string; enabled: boolean; threshold: number }) =>
      approvalService.updateLoanControlConfig(vars.operation, vars.enabled, vars.threshold),
    onSuccess: (data, vars) => {
      toast({
        title: "Saved",
        description: `Dual-control settings for ${loanOperationLabels[vars.operation] ?? vars.operation} updated.`,
      });
      queryClient.setQueryData(["loan-control-config"], data);
    },
    onError: (err: Error) => {
      toast({ title: "Save Failed", description: err.message, variant: "destructive" });
    },
  });

  const rows: ControlConfig[] = configs ?? [];

  // Ensure both LOAN operations always render, even if the backend returns none yet.
  const loanRows: ControlConfig[] = LOAN_OPERATIONS.map((operation) => {
    const existing = (loanConfigs ?? []).find((c) => c.operation === operation);
    return (
      existing ?? {
        tenantId: "",
        operation,
        enabled: false,
        thresholdAmount: 0,
      }
    );
  });

  return (
    <DashboardLayout
      title="Dual-Control Settings"
      subtitle="Configure maker-checker thresholds for sensitive operations"
      breadcrumbs={[
        { label: "Home", href: "/" },
        { label: "Dual-Control Settings" },
      ]}
    >
      <div className="space-y-4">
        <Card className="border-blue-200 bg-blue-50">
          <CardContent className="p-4 flex items-start gap-3">
            <ShieldCheck className="h-5 w-5 text-blue-700 shrink-0 mt-0.5" />
            <div>
              <p className="text-xs font-semibold text-blue-800 font-sans">
                Maker-Checker (Dual Control)
              </p>
              <p className="text-xs text-blue-700 font-sans mt-1">
                When an operation is enabled and a transaction meets or exceeds its threshold,
                it is held as a pending approval and must be authorised by a second user (the
                checker) who is different from the user who initiated it (the maker).
              </p>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Controlled Operations</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-14 w-full" />
                ))}
              </div>
            ) : rows.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-32 text-muted-foreground">
                <p className="text-sm font-medium">No control configurations</p>
              </div>
            ) : (
              <div className="divide-y">
                {rows.map((c) => {
                  const state = draft[c.operation] ?? {
                    enabled: c.enabled,
                    threshold: String(c.thresholdAmount ?? 0),
                  };
                  const setState = (patch: Partial<RowState>) =>
                    setDraft((prev) => ({
                      ...prev,
                      [c.operation]: { ...state, ...patch },
                    }));
                  return (
                    <div
                      key={c.operation}
                      className="flex flex-col sm:flex-row sm:items-center gap-3 p-4"
                    >
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium font-sans">
                          {operationLabels[c.operation] ?? c.operation}
                        </p>
                        <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                          {c.operation}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Switch
                          id={`enabled-${c.operation}`}
                          checked={state.enabled}
                          onCheckedChange={(checked) => setState({ enabled: checked })}
                        />
                        <Label
                          htmlFor={`enabled-${c.operation}`}
                          className="text-xs font-sans cursor-pointer"
                        >
                          {state.enabled ? "Enabled" : "Disabled"}
                        </Label>
                      </div>
                      <div className="flex items-center gap-2">
                        <Label className="text-xs font-sans text-muted-foreground">
                          Threshold (KES)
                        </Label>
                        <Input
                          type="number"
                          min="0"
                          step="0.01"
                          className="w-[140px] h-8 text-sm font-mono"
                          value={state.threshold}
                          onChange={(e) => setState({ threshold: e.target.value })}
                        />
                      </div>
                      <Button
                        size="sm"
                        className="text-xs h-8"
                        onClick={() =>
                          saveMutation.mutate({
                            operation: c.operation,
                            enabled: state.enabled,
                            threshold: parseFloat(state.threshold) || 0,
                          })
                        }
                        disabled={saveMutation.isPending}
                      >
                        <Save className="h-3.5 w-3.5 mr-1" /> Save
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Loan Controls</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {loanLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 2 }).map((_, i) => (
                  <Skeleton key={i} className="h-14 w-full" />
                ))}
              </div>
            ) : (
              <div className="divide-y">
                {loanRows.map((c) => {
                  const state = loanDraft[c.operation] ?? {
                    enabled: c.enabled,
                    threshold: String(c.thresholdAmount ?? 0),
                  };
                  const setState = (patch: Partial<RowState>) =>
                    setLoanDraft((prev) => ({
                      ...prev,
                      [c.operation]: { ...state, ...patch },
                    }));
                  return (
                    <div
                      key={c.operation}
                      className="flex flex-col sm:flex-row sm:items-center gap-3 p-4"
                    >
                      <div className="flex-1 min-w-0">
                        <p className="text-sm font-medium font-sans">
                          {loanOperationLabels[c.operation] ?? c.operation}
                        </p>
                        <p className="text-[10px] uppercase tracking-wider text-muted-foreground font-sans">
                          {c.operation}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Switch
                          id={`enabled-${c.operation}`}
                          checked={state.enabled}
                          onCheckedChange={(checked) => setState({ enabled: checked })}
                        />
                        <Label
                          htmlFor={`enabled-${c.operation}`}
                          className="text-xs font-sans cursor-pointer"
                        >
                          {state.enabled ? "Enabled" : "Disabled"}
                        </Label>
                      </div>
                      <div className="flex flex-col gap-0.5">
                        <div className="flex items-center gap-2">
                          <Label className="text-xs font-sans text-muted-foreground">
                            Threshold (KES)
                          </Label>
                          <Input
                            type="number"
                            min="0"
                            step="0.01"
                            className="w-[140px] h-8 text-sm font-mono"
                            value={state.threshold}
                            onChange={(e) => setState({ threshold: e.target.value })}
                          />
                        </div>
                        <p className="text-[10px] text-muted-foreground font-sans">
                          0 = always require dual approval
                        </p>
                      </div>
                      <Button
                        size="sm"
                        className="text-xs h-8"
                        onClick={() =>
                          saveLoanMutation.mutate({
                            operation: c.operation,
                            enabled: state.enabled,
                            threshold: parseFloat(state.threshold) || 0,
                          })
                        }
                        disabled={saveLoanMutation.isPending}
                      >
                        <Save className="h-3.5 w-3.5 mr-1" /> Save
                      </Button>
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </DashboardLayout>
  );
};

export default ControlSettingsPage;
