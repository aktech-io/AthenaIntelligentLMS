import { useRef, useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Shield, AlertTriangle, Upload, FileText, Download, Trash2 } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { complianceService, type ComplianceAlert } from "@/services/complianceService";
import { mediaService, type MediaFile } from "@/services/mediaService";

const riskColor: Record<string, string> = {
  HIGH: "bg-destructive/10 text-destructive border-destructive/20",
  MEDIUM: "bg-warning/10 text-warning border-warning/20",
  LOW: "bg-info/10 text-info border-info/20",
};

const CompliancePage = () => {
  const queryClient = useQueryClient();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [docCustomerId, setDocCustomerId] = useState("CUST-001");
  const [uploading, setUploading] = useState(false);
  const [uploadError, setUploadError] = useState<string | null>(null);

  const { data: page, isLoading, isError } = useQuery({
    queryKey: ["compliance", "alerts"],
    queryFn: () => complianceService.listAlerts(0, 50),
    staleTime: 60_000,
    retry: false,
  });

  const { data: docs = [], refetch: refetchDocs } = useQuery({
    queryKey: ["media", "customer", docCustomerId],
    queryFn: () => mediaService.listByCustomer(docCustomerId),
    staleTime: 30_000,
    retry: false,
    enabled: !!docCustomerId,
  });

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setUploadError(null);
    try {
      await mediaService.upload(file, docCustomerId, undefined, "CUSTOMER_DOCUMENT");
      refetchDocs();
    } catch (err: unknown) {
      setUploadError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  };

  const handleDelete = async (mediaId: string) => {
    try {
      await mediaService.deleteFile(mediaId);
      refetchDocs();
    } catch {
      // silent — doc may already be deleted
    }
  };

  const alerts: ComplianceAlert[] = page?.content ?? [];

  return (
    <DashboardLayout
      title="KYC & Compliance"
      subtitle="Customer verification & regulatory checks"
    >
      <div className="space-y-4 animate-fade-in">
        {/* Summary */}
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <Card>
            <CardContent className="p-5 flex items-center gap-3">
              <div className="h-10 w-10 rounded-lg bg-primary/10 flex items-center justify-center">
                <Shield className="h-5 w-5 text-primary" />
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Total Alerts</p>
                {isLoading ? (
                  <Skeleton className="h-7 w-10 mt-1" />
                ) : (
                  <p className="text-xl font-bold">{page?.totalElements ?? alerts.length}</p>
                )}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5 flex items-center gap-3">
              <div className="h-10 w-10 rounded-lg bg-destructive/10 flex items-center justify-center">
                <AlertTriangle className="h-5 w-5 text-destructive" />
              </div>
              <div>
                <p className="text-xs text-muted-foreground">High Risk</p>
                {isLoading ? (
                  <Skeleton className="h-7 w-10 mt-1" />
                ) : (
                  <p className="text-xl font-bold">
                    {alerts.filter((a) => a.riskLevel === "HIGH").length}
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardContent className="p-5 flex items-center gap-3">
              <div className="h-10 w-10 rounded-lg bg-warning/10 flex items-center justify-center">
                <AlertTriangle className="h-5 w-5 text-warning" />
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Pending Review</p>
                {isLoading ? (
                  <Skeleton className="h-7 w-10 mt-1" />
                ) : (
                  <p className="text-xl font-bold">
                    {alerts.filter((a) => a.status === "PENDING" || a.status === "OPEN").length}
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium">Compliance Alerts</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            {isLoading ? (
              <div className="p-4 space-y-2">
                {Array.from({ length: 4 }).map((_, i) => (
                  <Skeleton key={i} className="h-10 w-full" />
                ))}
              </div>
            ) : isError ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <p className="text-sm font-medium">Unable to load compliance data</p>
                <p className="text-xs mt-1">Compliance service returned an error.</p>
              </div>
            ) : alerts.length === 0 ? (
              <div className="flex flex-col items-center justify-center h-48 text-muted-foreground">
                <Shield className="h-10 w-10 mb-3 opacity-30" />
                <p className="text-sm font-medium">No compliance alerts</p>
                <p className="text-xs mt-1">All compliance checks are clear.</p>
              </div>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">Alert ID</TableHead>
                    <TableHead className="text-xs">Entity</TableHead>
                    <TableHead className="text-xs">Type</TableHead>
                    <TableHead className="text-xs">Risk Level</TableHead>
                    <TableHead className="text-xs">Status</TableHead>
                    <TableHead className="text-xs">Date</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {alerts.map((alert) => (
                    <TableRow key={alert.id} className="table-row-hover">
                      <TableCell className="text-xs font-mono">{alert.id}</TableCell>
                      <TableCell className="text-xs">{alert.entityId ?? "—"}</TableCell>
                      <TableCell className="text-xs">{alert.alertType ?? alert.checkType ?? "—"}</TableCell>
                      <TableCell>
                        {alert.riskLevel ? (
                          <Badge variant="outline" className={`text-[11px] ${riskColor[alert.riskLevel] ?? ""}`}>
                            {alert.riskLevel}
                          </Badge>
                        ) : (
                          <span className="text-xs text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px]">{alert.status}</Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {alert.checkedAt ?? alert.createdAt ?? "—"}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Customer Documents */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between flex-wrap gap-2">
              <CardTitle className="text-sm font-medium flex items-center gap-2">
                <FileText className="h-4 w-4" /> Customer Documents
              </CardTitle>
              <div className="flex items-center gap-2">
                <Input
                  className="h-7 text-xs w-40"
                  placeholder="Customer ID"
                  value={docCustomerId}
                  onChange={(e) => setDocCustomerId(e.target.value)}
                />
                <Button
                  size="sm"
                  variant="outline"
                  className="text-xs gap-1.5 h-7"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={uploading}
                >
                  <Upload className="h-3.5 w-3.5" />
                  {uploading ? "Uploading…" : "Upload Document"}
                </Button>
                <input
                  ref={fileInputRef}
                  type="file"
                  className="hidden"
                  onChange={handleUpload}
                  accept=".pdf,.jpg,.jpeg,.png,.doc,.docx"
                />
              </div>
            </div>
            {uploadError && (
              <p className="text-xs text-destructive mt-1">{uploadError}</p>
            )}
          </CardHeader>
          <CardContent>
            {docs.length === 0 ? (
              <p className="text-xs text-muted-foreground py-3 text-center">
                No documents on file for {docCustomerId}. Upload a customer document above.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="text-xs">File Name</TableHead>
                    <TableHead className="text-xs">Category</TableHead>
                    <TableHead className="text-xs">Size</TableHead>
                    <TableHead className="text-xs">Uploaded</TableHead>
                    <TableHead className="text-xs">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {docs.map((doc: MediaFile) => (
                    <TableRow key={doc.id} className="table-row-hover">
                      <TableCell className="text-xs flex items-center gap-1.5">
                        <FileText className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                        {doc.originalFilename}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className="text-[11px]">{doc.category}</Badge>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {doc.fileSize ? `${Math.round(doc.fileSize / 1024)} KB` : "—"}
                      </TableCell>
                      <TableCell className="text-xs font-mono">
                        {new Date(doc.createdAt).toLocaleDateString()}
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <a
                            href={mediaService.downloadUrl(doc.id)}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                          >
                            <Download className="h-3.5 w-3.5" /> Download
                          </a>
                          <Button
                            size="icon"
                            variant="ghost"
                            className="h-6 w-6 text-destructive hover:text-destructive"
                            onClick={() => handleDelete(doc.id)}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
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
    </DashboardLayout>
  );
};

export default CompliancePage;
