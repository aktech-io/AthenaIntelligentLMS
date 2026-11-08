import { useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  ShieldAlert, Plus, Trash2, Users, Globe, UserX, Newspaper, Scan,
} from "lucide-react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { fraudService, type WatchlistEntry } from "@/services/fraudService";
import { useToast } from "@/hooks/use-toast";

const LIST_TYPE_CONFIG: Record<string, { label: string; color: string; icon: React.ElementType }> = {
  PEP: { label: "PEP", color: "bg-warning/15 text-warning border-warning/30", icon: Users },
  SANCTIONS: { label: "Sanctions", color: "bg-destructive/15 text-destructive border-destructive/30", icon: Globe },
  INTERNAL_BLACKLIST: { label: "Internal Blacklist", color: "bg-red-600/15 text-red-700 border-red-600/30", icon: UserX },
  ADVERSE_MEDIA: { label: "Adverse Media", color: "bg-info/15 text-info border-info/30", icon: Newspaper },
};

const WatchlistPage = () => {
  const { toast } = useToast();
  const queryClient = useQueryClient();
  const [showInactive, setShowInactive] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [formData, setFormData] = useState({
    listType: "INTERNAL_BLACKLIST",
    entryType: "INDIVIDUAL",
    name: "",
    nationalId: "",
    phone: "",
    reason: "",
    source: "",
  });

  const { data: entriesPage, isLoading } = useQuery({
    queryKey: ["fraud", "watchlist", showInactive],
    queryFn: () => fraudService.listWatchlistEntries(0, 100, !showInactive),
    staleTime: 30_000,
    retry: false,
  });

  const createMutation = useMutation({
    mutationFn: () => fraudService.createWatchlistEntry({
      listType: formData.listType,
      entryType: formData.entryType,
      name: formData.name || undefined,
      nationalId: formData.nationalId || undefined,
      phone: formData.phone || undefined,
      reason: formData.reason || undefined,
      source: formData.source || undefined,
    }),
    onSuccess: () => {
      toast({ title: "Entry added", description: "Watchlist entry created" });
      queryClient.invalidateQueries({ queryKey: ["fraud", "watchlist"] });
      setCreateOpen(false);
      setFormData({ listType: "INTERNAL_BLACKLIST", entryType: "INDIVIDUAL", name: "", nationalId: "", phone: "", reason: "", source: "" });
    },
    onError: (e: Error) => toast({ title: "Failed", description: e.message, variant: "destructive" }),
  });

  const deactivateMutation = useMutation({
    mutationFn: (id: string) => fraudService.deactivateWatchlistEntry(id),
    onSuccess: () => {
      toast({ title: "Deactivated", description: "Entry removed from active watchlist" });
      queryClient.invalidateQueries({ queryKey: ["fraud", "watchlist"] });
    },
    onError: (e: Error) => toast({ title: "Failed", description: e.message, variant: "destructive" }),
  });

  const entries = entriesPage?.content ?? [];

  const countByType = (type: string) => entries.filter(e => e.listType === type).length;

  return (
    <DashboardLayout
      title="Watchlist Management"
      subtitle="PEP, sanctions, internal blacklist, and adverse media screening lists"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Compliance" }, { label: "Watchlist" }]}
    >
      <div className="space-y-6 animate-fade-in">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <Badge className="bg-success/10 text-success border-success/20 gap-1.5 px-3 py-1">
              <span className="h-1.5 w-1.5 rounded-full bg-success inline-block animate-pulse" />
              Screening Active
            </Badge>
            <span className="text-xs text-muted-foreground">
              {entries.length} active entries | Real-time matching on customer onboarding & transactions
            </span>
          </div>
          <div className="flex gap-2">
            <Button variant="outline" size="sm" className="h-8 text-xs"
              onClick={() => setShowInactive(!showInactive)}>
              {showInactive ? "Show Active" : "Show All"}
            </Button>
            <Button variant="outline" size="sm" className="h-8 text-xs" onClick={() => {
              fraudService.triggerBatchScreening()
                .then((r) => toast({ title: "Screening complete", description: `${r.matchesFound} matches found, ${r.alertsCreated} alerts created` }))
                .catch((e: Error) => toast({ title: "Screening failed", description: e.message, variant: "destructive" }));
            }}>
              <Scan className="h-3.5 w-3.5 mr-1" /> Screen All Customers
            </Button>
            <Button size="sm" className="h-8 text-xs" onClick={() => setCreateOpen(true)}>
              <Plus className="h-3.5 w-3.5 mr-1" /> Add Entry
            </Button>
          </div>
        </div>

        {/* Type KPIs */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
          {Object.entries(LIST_TYPE_CONFIG).map(([type, cfg]) => (
            <Card key={type}>
              <CardContent className="p-5">
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs text-muted-foreground">{cfg.label}</span>
                  <cfg.icon className="h-4 w-4 text-muted-foreground" />
                </div>
                <p className="text-2xl font-heading">{countByType(type)}</p>
              </CardContent>
            </Card>
          ))}
        </div>

        {/* Entries Table */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium">
              {showInactive ? "All" : "Active"} Watchlist Entries
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
              </div>
            ) : entries.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <ShieldAlert className="h-8 w-8 mb-2 text-muted-foreground/50" />
                <p className="text-sm font-medium">No watchlist entries</p>
                <p className="text-xs mt-1">Add PEP, sanctions, or internal blacklist entries</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">List Type</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Name</TableHead>
                    <TableHead className="text-xs">National ID</TableHead>
                    <TableHead className="text-xs">Phone</TableHead>
                    <TableHead className="text-xs">Reason</TableHead>
                    <TableHead className="text-xs">Source</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Added</TableHead>
                    <TableHead className="text-xs"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((e) => {
                    const cfg = LIST_TYPE_CONFIG[e.listType] ?? { label: e.listType, color: "bg-muted text-muted-foreground" };
                    return (
                      <TableRow key={e.id} className="table-row-hover">
                        <TableCell>
                          <Badge variant="outline" className={`text-[11px] ${cfg.color}`}>{cfg.label}</Badge>
                        </TableCell>
                        <TableCell className="text-xs">{e.entryType}</TableCell>
                        <TableCell className="text-xs font-medium">{e.name || "—"}</TableCell>
                        <TableCell className="text-xs font-mono">{e.nationalId || "—"}</TableCell>
                        <TableCell className="text-xs">{e.phone || "—"}</TableCell>
                        <TableCell className="text-xs text-muted-foreground max-w-[200px] truncate">{e.reason || "—"}</TableCell>
                        <TableCell className="text-xs">{e.source || "—"}</TableCell>
                        <TableCell>
                          <Badge variant="outline" className={`text-[11px] ${e.active ? "bg-success/15 text-success border-success/30" : "bg-muted text-muted-foreground"}`}>
                            {e.active ? "Active" : "Inactive"}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs whitespace-nowrap">{e.createdAt?.split("T")[0]}</TableCell>
                        <TableCell>
                          {e.active && (
                            <Button variant="ghost" size="sm" className="h-7 text-xs text-destructive hover:text-destructive"
                              onClick={() => { if (confirm("Deactivate this entry?")) deactivateMutation.mutate(e.id); }}
                              disabled={deactivateMutation.isPending}>
                              <Trash2 className="h-3 w-3" />
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Create Dialog */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Add Watchlist Entry</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-muted-foreground">List Type</label>
                <Select value={formData.listType} onValueChange={(v) => setFormData(p => ({ ...p, listType: v }))}>
                  <SelectTrigger className="h-9 text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="PEP">PEP</SelectItem>
                    <SelectItem value="SANCTIONS">Sanctions</SelectItem>
                    <SelectItem value="INTERNAL_BLACKLIST">Internal Blacklist</SelectItem>
                    <SelectItem value="ADVERSE_MEDIA">Adverse Media</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Entry Type</label>
                <Select value={formData.entryType} onValueChange={(v) => setFormData(p => ({ ...p, entryType: v }))}>
                  <SelectTrigger className="h-9 text-sm"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="INDIVIDUAL">Individual</SelectItem>
                    <SelectItem value="ENTITY">Entity</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">Name</label>
              <Input className="h-9 text-sm" placeholder="Full name or entity name" value={formData.name}
                onChange={(e) => setFormData(p => ({ ...p, name: e.target.value }))} />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="text-xs text-muted-foreground">National ID</label>
                <Input className="h-9 text-sm" placeholder="ID number" value={formData.nationalId}
                  onChange={(e) => setFormData(p => ({ ...p, nationalId: e.target.value }))} />
              </div>
              <div>
                <label className="text-xs text-muted-foreground">Phone</label>
                <Input className="h-9 text-sm" placeholder="+254..." value={formData.phone}
                  onChange={(e) => setFormData(p => ({ ...p, phone: e.target.value }))} />
              </div>
            </div>
            <div>
              <label className="text-xs text-muted-foreground">Reason</label>
              <Textarea className="text-sm" placeholder="Why is this entry on the watchlist?" value={formData.reason}
                onChange={(e) => setFormData(p => ({ ...p, reason: e.target.value }))} />
            </div>
            <div>
              <label className="text-xs text-muted-foreground">Source</label>
              <Input className="h-9 text-sm" placeholder="e.g. OFAC, UN Security Council, Internal Investigation" value={formData.source}
                onChange={(e) => setFormData(p => ({ ...p, source: e.target.value }))} />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button onClick={() => createMutation.mutate()} disabled={createMutation.isPending}>
              {createMutation.isPending ? "Adding..." : "Add to Watchlist"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default WatchlistPage;
