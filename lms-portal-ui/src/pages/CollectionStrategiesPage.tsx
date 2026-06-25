import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter, DialogDescription,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Settings, Plus, Pencil, Trash2 } from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  collectionsService,
  type CollectionStrategy,
  type CreateStrategyRequest,
} from "@/services/collectionsService";
import { useToast } from "@/hooks/use-toast";

// ─── Action Type Labels ──────────────────────────────────────────────────────

const ACTION_TYPES = [
  "PHONE_CALL",
  "SMS",
  "EMAIL",
  "FIELD_VISIT",
  "LEGAL_NOTICE",
  "RESTRUCTURE_OFFER",
  "WRITE_OFF",
  "OTHER",
] as const;

const ACTION_TYPE_LABELS: Record<string, string> = {
  PHONE_CALL: "Phone Call",
  SMS: "SMS",
  EMAIL: "Email",
  FIELD_VISIT: "Field Visit",
  LEGAL_NOTICE: "Legal Notice",
  RESTRUCTURE_OFFER: "Restructure Offer",
  WRITE_OFF: "Write-Off",
  OTHER: "Other",
};

function actionLabel(type: string): string {
  return ACTION_TYPE_LABELS[type] ?? type;
}

// ─── Empty Form State ────────────────────────────────────────────────────────

interface StrategyForm {
  name: string;
  productType: string;
  dpdFrom: number;
  dpdTo: number;
  actionType: string;
  priority: number;
  isActive: boolean;
}

const emptyForm: StrategyForm = {
  name: "",
  productType: "",
  dpdFrom: 0,
  dpdTo: 30,
  actionType: "PHONE_CALL",
  priority: 1,
  isActive: true,
};

// ─── Main Page Component ─────────────────────────────────────────────────────

export default function CollectionStrategiesPage() {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  // Dialog state
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<StrategyForm>(emptyForm);

  // Delete confirmation
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deletingStrategy, setDeletingStrategy] = useState<CollectionStrategy | null>(null);

  // ─── Queries ─────────────────────────────────────────────────────────────

  const { data: strategies, isLoading } = useQuery({
    queryKey: ["collection-strategies"],
    queryFn: () => collectionsService.listStrategies(),
  });

  // ─── Mutations ───────────────────────────────────────────────────────────

  const createMutation = useMutation({
    mutationFn: (req: CreateStrategyRequest) => collectionsService.createStrategy(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-strategies"] });
      toast({ title: "Strategy created successfully" });
      closeDialog();
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, req }: { id: string; req: Partial<CollectionStrategy> }) =>
      collectionsService.updateStrategy(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-strategies"] });
      toast({ title: "Strategy updated successfully" });
      closeDialog();
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => collectionsService.deleteStrategy(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-strategies"] });
      toast({ title: "Strategy deleted" });
      setDeleteDialogOpen(false);
      setDeletingStrategy(null);
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, isActive }: { id: string; isActive: boolean }) =>
      collectionsService.updateStrategy(id, { isActive }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["collection-strategies"] });
    },
    onError: (err: Error) => toast({ title: err.message, variant: "destructive" }),
  });

  // ─── Helpers ─────────────────────────────────────────────────────────────

  function openCreate() {
    setEditingId(null);
    setForm(emptyForm);
    setDialogOpen(true);
  }

  function openEdit(s: CollectionStrategy) {
    setEditingId(s.id);
    setForm({
      name: s.name,
      productType: s.productType ?? "",
      dpdFrom: s.dpdFrom,
      dpdTo: s.dpdTo,
      actionType: s.actionType,
      priority: s.priority,
      isActive: s.isActive,
    });
    setDialogOpen(true);
  }

  function closeDialog() {
    setDialogOpen(false);
    setEditingId(null);
    setForm(emptyForm);
  }

  function handleSubmit() {
    const req: CreateStrategyRequest = {
      name: form.name.trim(),
      productType: form.productType.trim() || undefined,
      dpdFrom: form.dpdFrom,
      dpdTo: form.dpdTo,
      actionType: form.actionType,
      priority: form.priority,
      isActive: form.isActive,
    };
    if (!req.name) {
      toast({ title: "Strategy name is required", variant: "destructive" });
      return;
    }
    if (req.dpdFrom > req.dpdTo) {
      toast({ title: "DPD From must be less than or equal to DPD To", variant: "destructive" });
      return;
    }
    if (editingId) {
      updateMutation.mutate({ id: editingId, req });
    } else {
      createMutation.mutate(req);
    }
  }

  function confirmDelete(s: CollectionStrategy) {
    setDeletingStrategy(s);
    setDeleteDialogOpen(true);
  }

  // ─── Group strategies by product type ────────────────────────────────────

  const grouped = (strategies ?? []).reduce<Record<string, CollectionStrategy[]>>((acc, s) => {
    const key = s.productType || "All Products";
    if (!acc[key]) acc[key] = [];
    acc[key].push(s);
    return acc;
  }, {});

  // Sort groups: "All Products" first, then alphabetical
  const sortedGroupKeys = Object.keys(grouped).sort((a, b) => {
    if (a === "All Products") return -1;
    if (b === "All Products") return 1;
    return a.localeCompare(b);
  });

  // Sort strategies within each group by priority
  for (const key of sortedGroupKeys) {
    grouped[key].sort((a, b) => a.priority - b.priority);
  }

  // ─── Render ──────────────────────────────────────────────────────────────

  const isSaving = createMutation.isPending || updateMutation.isPending;

  return (
    <DashboardLayout>
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
              <Settings className="h-6 w-6 text-muted-foreground" />
              Collection Strategies
            </h1>
            <p className="text-muted-foreground mt-1">
              Configure automated collection actions by product type and DPD range
            </p>
          </div>
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4 mr-2" />
            Add Strategy
          </Button>
        </div>

        {/* Loading skeleton */}
        {isLoading && (
          <Card>
            <CardContent className="p-6 space-y-3">
              {Array.from({ length: 5 }).map((_, i) => (
                <Skeleton key={i} className="h-10 w-full" />
              ))}
            </CardContent>
          </Card>
        )}

        {/* Empty state */}
        {!isLoading && (strategies ?? []).length === 0 && (
          <Card>
            <CardContent className="p-12 text-center">
              <Settings className="h-12 w-12 mx-auto text-muted-foreground/50 mb-4" />
              <h3 className="text-lg font-medium mb-1">No strategies configured</h3>
              <p className="text-sm text-muted-foreground mb-4">
                Create your first collection strategy to automate actions based on days past due.
              </p>
              <Button onClick={openCreate}>
                <Plus className="h-4 w-4 mr-2" />
                Add Strategy
              </Button>
            </CardContent>
          </Card>
        )}

        {/* Strategy tables grouped by product type */}
        {!isLoading && sortedGroupKeys.map((groupKey) => (
          <Card key={groupKey}>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">{groupKey}</CardTitle>
              <CardDescription>
                {grouped[groupKey].length} strateg{grouped[groupKey].length === 1 ? "y" : "ies"}
              </CardDescription>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>DPD Range</TableHead>
                    <TableHead>Action Type</TableHead>
                    <TableHead className="text-center">Priority</TableHead>
                    <TableHead className="text-center">Active</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {grouped[groupKey].map((s) => (
                    <TableRow key={s.id}>
                      <TableCell className="font-medium">{s.name}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className="font-mono text-xs">
                          {s.dpdFrom} &ndash; {s.dpdTo} days
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">{actionLabel(s.actionType)}</Badge>
                      </TableCell>
                      <TableCell className="text-center">{s.priority}</TableCell>
                      <TableCell className="text-center">
                        <Switch
                          checked={s.isActive}
                          onCheckedChange={(checked) =>
                            toggleMutation.mutate({ id: s.id, isActive: checked })
                          }
                        />
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => openEdit(s)}
                            title="Edit strategy"
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => confirmDelete(s)}
                            title="Delete strategy"
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        ))}

        {/* ─── Create / Edit Dialog ─────────────────────────────────────────── */}
        <Dialog open={dialogOpen} onOpenChange={(o) => { if (!o) closeDialog(); }}>
          <DialogContent className="sm:max-w-lg">
            <DialogHeader>
              <DialogTitle>{editingId ? "Edit Strategy" : "Create Strategy"}</DialogTitle>
              <DialogDescription>
                {editingId
                  ? "Update the collection strategy configuration."
                  : "Define a new automated collection action triggered by DPD range."}
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="strategy-name">Name</Label>
                <Input
                  id="strategy-name"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="e.g. Early SMS Reminder"
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="strategy-product-type">Product Type (optional)</Label>
                <Input
                  id="strategy-product-type"
                  value={form.productType}
                  onChange={(e) => setForm({ ...form, productType: e.target.value })}
                  placeholder="Leave empty for all products"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="grid gap-2">
                  <Label htmlFor="strategy-dpd-from">DPD From</Label>
                  <Input
                    id="strategy-dpd-from"
                    type="number"
                    min={0}
                    value={form.dpdFrom}
                    onChange={(e) => setForm({ ...form, dpdFrom: parseInt(e.target.value) || 0 })}
                  />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="strategy-dpd-to">DPD To</Label>
                  <Input
                    id="strategy-dpd-to"
                    type="number"
                    min={0}
                    value={form.dpdTo}
                    onChange={(e) => setForm({ ...form, dpdTo: parseInt(e.target.value) || 0 })}
                  />
                </div>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="strategy-action-type">Action Type</Label>
                <Select
                  value={form.actionType}
                  onValueChange={(v) => setForm({ ...form, actionType: v })}
                >
                  <SelectTrigger id="strategy-action-type">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {ACTION_TYPES.map((at) => (
                      <SelectItem key={at} value={at}>
                        {actionLabel(at)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="strategy-priority">Priority</Label>
                <Input
                  id="strategy-priority"
                  type="number"
                  min={1}
                  value={form.priority}
                  onChange={(e) => setForm({ ...form, priority: parseInt(e.target.value) || 1 })}
                />
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="strategy-active"
                  checked={form.isActive}
                  onCheckedChange={(checked) =>
                    setForm({ ...form, isActive: checked === true })
                  }
                />
                <Label htmlFor="strategy-active" className="cursor-pointer">
                  Active
                </Label>
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={closeDialog} disabled={isSaving}>
                Cancel
              </Button>
              <Button onClick={handleSubmit} disabled={isSaving}>
                {isSaving ? "Saving..." : editingId ? "Update" : "Create"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        {/* ─── Delete Confirmation Dialog ───────────────────────────────────── */}
        <Dialog open={deleteDialogOpen} onOpenChange={(o) => { if (!o) { setDeleteDialogOpen(false); setDeletingStrategy(null); } }}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Delete Strategy</DialogTitle>
              <DialogDescription>
                Are you sure you want to delete the strategy{" "}
                <span className="font-semibold">{deletingStrategy?.name}</span>? This action cannot
                be undone.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => { setDeleteDialogOpen(false); setDeletingStrategy(null); }}
                disabled={deleteMutation.isPending}
              >
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => deletingStrategy && deleteMutation.mutate(deletingStrategy.id)}
                disabled={deleteMutation.isPending}
              >
                {deleteMutation.isPending ? "Deleting..." : "Delete"}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </DashboardLayout>
  );
}
