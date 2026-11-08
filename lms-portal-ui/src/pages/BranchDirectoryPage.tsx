import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Loader2,
  Building2,
  Plus,
  Pencil,
  Trash2,
  MapPin,
  CheckCircle2,
} from "lucide-react";
import {
  orgService,
  Branch,
  CreateBranchRequest,
} from "@/services/orgService";

const BRANCH_TYPES = [
  { value: "HEAD_OFFICE", label: "Head Office" },
  { value: "BRANCH", label: "Branch" },
  { value: "AGENCY", label: "Agency" },
  { value: "SATELLITE", label: "Satellite" },
];

const STATUSES = [
  { value: "ACTIVE", label: "Active" },
  { value: "INACTIVE", label: "Inactive" },
];

const emptyForm: CreateBranchRequest = {
  name: "",
  code: "",
  type: "BRANCH",
  address: "",
  city: "",
  county: "",
  country: "KEN",
  phone: "",
  email: "",
  managerId: "",
  status: "ACTIVE",
  parentId: null,
};

const typeBadge = (type: string) => {
  switch (type) {
    case "HEAD_OFFICE":
      return (
        <Badge variant="default" className="text-[11px]">
          Head Office
        </Badge>
      );
    case "AGENCY":
      return (
        <Badge
          variant="outline"
          className="text-[11px] bg-amber-50 text-amber-700 border-amber-200"
        >
          Agency
        </Badge>
      );
    case "SATELLITE":
      return (
        <Badge
          variant="outline"
          className="text-[11px] bg-purple-50 text-purple-700 border-purple-200"
        >
          Satellite
        </Badge>
      );
    default:
      return (
        <Badge
          variant="outline"
          className="text-[11px] bg-blue-50 text-blue-700 border-blue-200"
        >
          Branch
        </Badge>
      );
  }
};

const statusBadge = (status: string) => {
  if (status === "ACTIVE")
    return (
      <Badge
        variant="outline"
        className="text-[11px] bg-success/10 text-success border-success/20 gap-1"
      >
        <CheckCircle2 className="h-3 w-3" />
        Active
      </Badge>
    );
  return (
    <Badge
      variant="outline"
      className="text-[11px] text-muted-foreground gap-1"
    >
      Inactive
    </Badge>
  );
};

const BranchDirectoryPage = () => {
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editingBranch, setEditingBranch] = useState<Branch | null>(null);
  const [deletingBranch, setDeletingBranch] = useState<Branch | null>(null);
  const [form, setForm] = useState<CreateBranchRequest>({ ...emptyForm });

  const {
    data: branches,
    isLoading,
    isError,
  } = useQuery({
    queryKey: ["branches"],
    queryFn: () => orgService.listBranches(),
  });

  const createMutation = useMutation({
    mutationFn: (data: CreateBranchRequest) => orgService.createBranch(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["branches"] });
      closeDialog();
    },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: CreateBranchRequest }) =>
      orgService.updateBranch(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["branches"] });
      closeDialog();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => orgService.deleteBranch(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["branches"] });
      setDeleteDialogOpen(false);
      setDeletingBranch(null);
    },
  });

  const openCreate = () => {
    setEditingBranch(null);
    setForm({ ...emptyForm });
    setDialogOpen(true);
  };

  const openEdit = (branch: Branch) => {
    setEditingBranch(branch);
    setForm({
      name: branch.name,
      code: branch.code,
      type: branch.type,
      address: branch.address,
      city: branch.city,
      county: branch.county,
      country: branch.country,
      phone: branch.phone,
      email: branch.email,
      managerId: branch.managerId,
      status: branch.status,
      parentId: branch.parentId,
    });
    setDialogOpen(true);
  };

  const openDelete = (branch: Branch) => {
    setDeletingBranch(branch);
    setDeleteDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setEditingBranch(null);
    setForm({ ...emptyForm });
  };

  const handleSubmit = () => {
    if (!form.name || !form.code) return;
    if (editingBranch) {
      updateMutation.mutate({ id: editingBranch.id, data: form });
    } else {
      createMutation.mutate(form);
    }
  };

  const isSaving = createMutation.isPending || updateMutation.isPending;

  // Summary counts
  const totalBranches = branches?.length ?? 0;
  const activeBranches =
    branches?.filter((b) => b.status === "ACTIVE").length ?? 0;
  const headOffices =
    branches?.filter((b) => b.type === "HEAD_OFFICE").length ?? 0;

  return (
    <DashboardLayout
      title="Branch Directory"
      subtitle="Manage all registered branches for this organization"
    >
      {/* Summary Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-primary/10 rounded-lg">
                <Building2 className="h-5 w-5 text-primary" />
              </div>
              <div>
                <p className="text-2xl font-bold">{totalBranches}</p>
                <p className="text-xs text-muted-foreground">Total Branches</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-success/10 rounded-lg">
                <CheckCircle2 className="h-5 w-5 text-success" />
              </div>
              <div>
                <p className="text-2xl font-bold">{activeBranches}</p>
                <p className="text-xs text-muted-foreground">Active</p>
              </div>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="pt-6">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-blue-500/10 rounded-lg">
                <MapPin className="h-5 w-5 text-blue-500" />
              </div>
              <div>
                <p className="text-2xl font-bold">{headOffices}</p>
                <p className="text-xs text-muted-foreground">Head Offices</p>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {isLoading && (
        <div className="flex items-center justify-center h-64 text-muted-foreground">
          <Loader2 className="h-6 w-6 animate-spin mr-2" />
          <span>Loading branch directory...</span>
        </div>
      )}

      {isError && (
        <div className="flex items-center justify-center h-64 text-destructive">
          <p>Failed to load branch directory. Please try again.</p>
        </div>
      )}

      {branches && (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div className="flex items-center gap-2">
              <Building2 className="h-5 w-5 text-muted-foreground" />
              <CardTitle className="text-base">Branches</CardTitle>
            </div>
            <Button size="sm" onClick={openCreate}>
              <Plus className="h-4 w-4 mr-1" />
              Add Branch
            </Button>
          </CardHeader>
          <CardContent className="p-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Code</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>City</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {branches.length === 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={6}
                      className="text-center text-muted-foreground py-8"
                    >
                      No branches found. Click "Add Branch" to create one.
                    </TableCell>
                  </TableRow>
                )}
                {branches.map((branch) => (
                  <TableRow key={branch.id}>
                    <TableCell>
                      <div>
                        <p className="font-medium text-sm">{branch.name}</p>
                        {branch.address && (
                          <p className="text-xs text-muted-foreground">
                            {branch.address}
                          </p>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {branch.code}
                    </TableCell>
                    <TableCell>{typeBadge(branch.type)}</TableCell>
                    <TableCell className="text-sm">
                      {branch.city || "---"}
                      {branch.county && branch.county !== branch.city
                        ? `, ${branch.county}`
                        : ""}
                    </TableCell>
                    <TableCell>{statusBadge(branch.status)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => openEdit(branch)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 text-destructive hover:text-destructive"
                          onClick={() => openDelete(branch)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      {/* Create / Edit Dialog */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {editingBranch ? "Edit Branch" : "Add New Branch"}
            </DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label htmlFor="name" className="text-xs">
                  Branch Name *
                </Label>
                <Input
                  id="name"
                  className="h-8 text-xs"
                  placeholder="e.g. Westlands Branch"
                  value={form.name}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, name: e.target.value }))
                  }
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="code" className="text-xs">
                  Branch Code *
                </Label>
                <Input
                  id="code"
                  className="h-8 text-xs"
                  placeholder="e.g. WL-001"
                  value={form.code}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, code: e.target.value }))
                  }
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label className="text-xs">Type</Label>
                <Select
                  value={form.type}
                  onValueChange={(v) => setForm((f) => ({ ...f, type: v }))}
                >
                  <SelectTrigger className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {BRANCH_TYPES.map((t) => (
                      <SelectItem key={t.value} value={t.value}>
                        {t.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label className="text-xs">Status</Label>
                <Select
                  value={form.status}
                  onValueChange={(v) => setForm((f) => ({ ...f, status: v }))}
                >
                  <SelectTrigger className="h-8 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {STATUSES.map((s) => (
                      <SelectItem key={s.value} value={s.value}>
                        {s.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="address" className="text-xs">
                Address
              </Label>
              <Input
                id="address"
                className="h-8 text-xs"
                placeholder="Street address"
                value={form.address ?? ""}
                onChange={(e) =>
                  setForm((f) => ({ ...f, address: e.target.value }))
                }
              />
            </div>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-1.5">
                <Label htmlFor="city" className="text-xs">
                  City
                </Label>
                <Input
                  id="city"
                  className="h-8 text-xs"
                  placeholder="Nairobi"
                  value={form.city ?? ""}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, city: e.target.value }))
                  }
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="county" className="text-xs">
                  County
                </Label>
                <Input
                  id="county"
                  className="h-8 text-xs"
                  placeholder="Nairobi"
                  value={form.county ?? ""}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, county: e.target.value }))
                  }
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="country" className="text-xs">
                  Country
                </Label>
                <Input
                  id="country"
                  className="h-8 text-xs"
                  placeholder="KEN"
                  value={form.country ?? "KEN"}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, country: e.target.value }))
                  }
                />
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label htmlFor="phone" className="text-xs">
                  Phone
                </Label>
                <Input
                  id="phone"
                  className="h-8 text-xs"
                  placeholder="+254..."
                  value={form.phone ?? ""}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, phone: e.target.value }))
                  }
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="email" className="text-xs">
                  Email
                </Label>
                <Input
                  id="email"
                  className="h-8 text-xs"
                  placeholder="branch@company.com"
                  value={form.email ?? ""}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, email: e.target.value }))
                  }
                />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" size="sm" onClick={closeDialog}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={isSaving || !form.name || !form.code}
            >
              {isSaving && <Loader2 className="h-4 w-4 animate-spin mr-1" />}
              {editingBranch ? "Save Changes" : "Create Branch"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Delete Branch</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            Are you sure you want to delete{" "}
            <strong>{deletingBranch?.name}</strong> ({deletingBranch?.code})?
            This action cannot be undone.
          </p>
          <DialogFooter>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDeleteDialogOpen(false)}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              size="sm"
              onClick={() =>
                deletingBranch && deleteMutation.mutate(deletingBranch.id)
              }
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending && (
                <Loader2 className="h-4 w-4 animate-spin mr-1" />
              )}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </DashboardLayout>
  );
};

export default BranchDirectoryPage;
