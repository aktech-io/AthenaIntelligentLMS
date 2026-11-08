import { useEffect, useRef, useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
} from "@/components/ui/dialog";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  FolderOpen, Upload, Trash2, Download, HardDrive, FileText,
  Search, Loader2, RefreshCw, X,
} from "lucide-react";
import { mediaService, MediaFile, MediaStats } from "@/services/mediaService";

// ─── Helpers ───────────────────────────────────────────────────────────────────
const CATEGORIES = [
  "CUSTOMER_DOCUMENT", "LOAN_DOCUMENT", "COLLATERAL", "IDENTITY",
  "INCOME_PROOF", "CONTRACT", "REPORT", "OTHER",
];

const fmtBytes = (bytes: number) => {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let i = 0;
  let v = bytes;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(1)} ${units[i]}`;
};

const statusBadge = (status: string) => {
  const map: Record<string, string> = {
    ACTIVE: "bg-success/10 text-success border-success/20",
    ARCHIVED: "bg-muted text-muted-foreground",
    DELETED: "bg-destructive/10 text-destructive border-destructive/20",
  };
  return (
    <Badge variant="outline" className={`text-[11px] ${map[status] ?? ""}`}>{status}</Badge>
  );
};

// ─── Upload Dialog ─────────────────────────────────────────────────────────────
function UploadDialog({
  open, onClose, onDone,
}: { open: boolean; onClose: () => void; onDone: () => void }) {
  const fileRef = useRef<HTMLInputElement>(null);
  const [customerId, setCustomerId] = useState("");
  const [category, setCategory] = useState("CUSTOMER_DOCUMENT");
  const [description, setDescription] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setCustomerId(""); setCategory("CUSTOMER_DOCUMENT");
    setDescription(""); setFile(null); setError(null);
    if (fileRef.current) fileRef.current.value = "";
  };

  const handleClose = () => { reset(); onClose(); };

  const upload = async () => {
    if (!file) { setError("Please select a file."); return; }
    setUploading(true);
    setError(null);
    try {
      await mediaService.upload(file, customerId || undefined, undefined, category);
      reset();
      onDone();
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Upload failed.");
    } finally {
      setUploading(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => !v && handleClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="text-sm">Upload Document</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <Label className="text-xs">File</Label>
            <input
              ref={fileRef}
              type="file"
              className="text-xs w-full border rounded-md px-3 py-1.5 cursor-pointer file:mr-3 file:text-xs file:border-0 file:bg-primary file:text-primary-foreground file:px-2 file:py-1 file:rounded"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label className="text-xs">Customer ID <span className="text-muted-foreground">(optional)</span></Label>
              <Input className="h-8 text-xs" placeholder="CUST-001"
                value={customerId} onChange={(e) => setCustomerId(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs">Category</Label>
              <Select value={category} onValueChange={setCategory}>
                <SelectTrigger className="h-8 text-xs"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {CATEGORIES.map((c) => <SelectItem key={c} value={c} className="text-xs">{c}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs">Description <span className="text-muted-foreground">(optional)</span></Label>
            <Input className="h-8 text-xs" placeholder="Brief description…"
              value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          {error && <p className="text-xs text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" className="text-xs" onClick={handleClose} disabled={uploading}>Cancel</Button>
          <Button size="sm" className="text-xs gap-1.5" onClick={upload} disabled={uploading}>
            {uploading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}
            Upload
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ─── Page ──────────────────────────────────────────────────────────────────────
const DocumentsPage = () => {
  const [files, setFiles] = useState<MediaFile[]>([]);
  const [stats, setStats] = useState<MediaStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [uploadOpen, setUploadOpen] = useState(false);

  // Filters
  const [filterCustomer, setFilterCustomer] = useState("");
  const [filterCategory, setFilterCategory] = useState("ALL");
  const [filterStatus, setFilterStatus] = useState("ALL");

  const load = async () => {
    setLoading(true);
    try {
      const [allFiles, statsData] = await Promise.all([
        mediaService.listAll(),
        mediaService.getStats(),
      ]);
      setFiles(allFiles);
      setStats(statsData);
    } catch {
      setFiles([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const handleDelete = async (id: string) => {
    if (!confirm("Delete this file permanently?")) return;
    try {
      await mediaService.deleteFile(id);
      setFiles((prev) => prev.filter((f) => f.id !== id));
    } catch {
      alert("Failed to delete file.");
    }
  };

  // Apply filters client-side
  const filtered = files.filter((f) => {
    if (filterCustomer && !f.customerId?.toLowerCase().includes(filterCustomer.toLowerCase())) return false;
    if (filterCategory !== "ALL" && f.category !== filterCategory) return false;
    if (filterStatus !== "ALL" && f.status !== filterStatus) return false;
    return true;
  });

  const clearFilters = () => {
    setFilterCustomer("");
    setFilterCategory("ALL");
    setFilterStatus("ALL");
  };
  const hasFilters = filterCustomer || filterCategory !== "ALL" || filterStatus !== "ALL";

  return (
    <DashboardLayout
      title="Document Store"
      subtitle="Manage customer and loan documents stored in the media-service"
      breadcrumbs={[{ label: "Home", href: "/" }, { label: "Administration" }, { label: "Documents" }]}
    >
      <div className="space-y-5 animate-fade-in">

        {/* Stats row */}
        {stats && (
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[11px] text-muted-foreground">Total Files</span>
                  <FileText className="h-3.5 w-3.5 text-muted-foreground" />
                </div>
                <p className="text-xl font-heading">{stats.totalDocuments}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[11px] text-muted-foreground">Storage Used</span>
                  <HardDrive className="h-3.5 w-3.5 text-muted-foreground" />
                </div>
                <p className="text-xl font-heading">{fmtBytes(stats.usedSpace)}</p>
                <p className="text-[11px] text-muted-foreground mt-0.5">{stats.usedPercentage?.toFixed(1)}% of disk</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[11px] text-muted-foreground">Free Space</span>
                  <HardDrive className="h-3.5 w-3.5 text-muted-foreground" />
                </div>
                <p className="text-xl font-heading">{fmtBytes(stats.freeSpace)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-[11px] text-muted-foreground">By File Type</span>
                  <FolderOpen className="h-3.5 w-3.5 text-muted-foreground" />
                </div>
                <div className="space-y-0.5 mt-1">
                  {Object.entries(stats.documentsByType ?? {}).slice(0, 3).map(([type, count]) => (
                    <div key={type} className="flex justify-between text-[11px]">
                      <span className="text-muted-foreground truncate">{type}</span>
                      <span className="font-medium">{count}</span>
                    </div>
                  ))}
                  {Object.keys(stats.documentsByType ?? {}).length === 0 &&
                    <p className="text-[11px] text-muted-foreground">No files yet</p>}
                </div>
              </CardContent>
            </Card>
          </div>
        )}

        {/* File browser */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between gap-3 flex-wrap">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <FileText className="h-4 w-4" />
                Files
                {!loading && <span className="text-xs font-normal text-muted-foreground">{filtered.length} shown</span>}
              </CardTitle>
              <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" className="text-xs gap-1.5" onClick={load} disabled={loading}>
                  <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
                  Refresh
                </Button>
                <Button size="sm" className="text-xs gap-1.5" onClick={() => setUploadOpen(true)}>
                  <Upload className="h-3.5 w-3.5" />
                  Upload File
                </Button>
              </div>
            </div>

            {/* Filters */}
            <div className="flex items-center gap-2 flex-wrap pt-3">
              <div className="relative">
                <Search className="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-muted-foreground" />
                <Input className="h-7 text-xs pl-6 w-36" placeholder="Customer ID…"
                  value={filterCustomer} onChange={(e) => setFilterCustomer(e.target.value)} />
              </div>
              <Select value={filterCategory} onValueChange={setFilterCategory}>
                <SelectTrigger className="h-7 text-xs w-44"><SelectValue placeholder="All categories" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="ALL" className="text-xs">All categories</SelectItem>
                  {CATEGORIES.map((c) => <SelectItem key={c} value={c} className="text-xs">{c}</SelectItem>)}
                </SelectContent>
              </Select>
              <Select value={filterStatus} onValueChange={setFilterStatus}>
                <SelectTrigger className="h-7 text-xs w-32"><SelectValue placeholder="All statuses" /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="ALL" className="text-xs">All statuses</SelectItem>
                  <SelectItem value="ACTIVE" className="text-xs">ACTIVE</SelectItem>
                  <SelectItem value="ARCHIVED" className="text-xs">ARCHIVED</SelectItem>
                </SelectContent>
              </Select>
              {hasFilters && (
                <Button variant="ghost" size="sm" className="h-7 text-xs gap-1 text-muted-foreground" onClick={clearFilters}>
                  <X className="h-3 w-3" /> Clear
                </Button>
              )}
            </div>
          </CardHeader>

          <CardContent className="p-0">
            {loading ? (
              <p className="text-xs text-muted-foreground py-10 text-center">Loading files…</p>
            ) : filtered.length === 0 ? (
              <div className="py-12 text-center space-y-2">
                <FolderOpen className="h-8 w-8 mx-auto text-muted-foreground/50" />
                <p className="text-xs text-muted-foreground">
                  {hasFilters ? "No files match the current filters." : "No documents uploaded yet."}
                </p>
                {!hasFilters && (
                  <Button size="sm" variant="outline" className="text-xs mt-2 gap-1.5" onClick={() => setUploadOpen(true)}>
                    <Upload className="h-3.5 w-3.5" /> Upload first file
                  </Button>
                )}
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs pl-4">File Name</TableHead>
                    <TableHead className="text-xs">Customer</TableHead>
                    <TableHead className="text-xs">Category</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Size</TableHead>
                    <TableHead className="text-xs">Uploaded By</TableHead>
                    <TableHead className="text-xs">Date</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs text-right pr-4">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filtered.map((f) => (
                    <TableRow key={f.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono max-w-[180px] truncate pl-4">
                        {f.originalFilename}
                      </TableCell>
                      <TableCell className="text-xs">{f.customerId ?? <span className="text-muted-foreground">—</span>}</TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px]">{f.category}</Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">{f.mediaType}</TableCell>
                      <TableCell className="text-xs tabular-nums">{fmtBytes(f.fileSize ?? 0)}</TableCell>
                      <TableCell className="text-xs">{f.uploadedBy ?? "—"}</TableCell>
                      <TableCell className="text-xs whitespace-nowrap">
                        {f.createdAt ? new Date(f.createdAt).toLocaleDateString() : "—"}
                      </TableCell>
                      <TableCell>{statusBadge(f.status)}</TableCell>
                      <TableCell className="text-right pr-4">
                        <div className="flex items-center justify-end gap-1">
                          <a
                            href={mediaService.downloadUrl(f.id)}
                            download={f.originalFilename}
                            className="inline-flex items-center gap-1 px-2 py-1 text-[11px] rounded border border-border hover:bg-muted transition-colors"
                          >
                            <Download className="h-3 w-3" /> Download
                          </a>
                          <Button
                            variant="ghost" size="icon"
                            className="h-6 w-6 text-destructive hover:text-destructive hover:bg-destructive/10"
                            onClick={() => handleDelete(f.id)}
                          >
                            <Trash2 className="h-3 w-3" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      <UploadDialog open={uploadOpen} onClose={() => setUploadOpen(false)} onDone={load} />
    </DashboardLayout>
  );
};

export default DocumentsPage;
