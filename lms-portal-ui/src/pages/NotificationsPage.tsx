import { useEffect, useState } from "react";
import { DashboardLayout } from "@/components/DashboardLayout";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  Bell, Mail, MessageSquare, CheckCircle2, XCircle, MinusCircle,
  ChevronLeft, ChevronRight, Send, Settings2, SaveIcon, Loader2,
} from "lucide-react";
import { notificationService, NotificationLog, NotificationConfig } from "@/services/notificationService";

// ─── Status badge helper ───────────────────────────────────────────────────────
const statusBadge = (status: string) => {
  if (status === "SENT")
    return <Badge variant="outline" className="text-[10px] bg-success/10 text-success border-success/20 gap-1"><CheckCircle2 className="h-3 w-3" />{status}</Badge>;
  if (status === "FAILED")
    return <Badge variant="outline" className="text-[10px] bg-destructive/10 text-destructive border-destructive/20 gap-1"><XCircle className="h-3 w-3" />{status}</Badge>;
  return <Badge variant="outline" className="text-[10px] text-muted-foreground gap-1"><MinusCircle className="h-3 w-3" />{status}</Badge>;
};

// ─── Email Config Tab ──────────────────────────────────────────────────────────
function EmailConfigTab() {
  const [cfg, setCfg] = useState<NotificationConfig>({
    type: "EMAIL", provider: "SMTP", host: "", port: 587,
    username: "", password: "", fromAddress: "", enabled: false,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null);

  useEffect(() => {
    notificationService.getConfig("EMAIL")
      .then((data) => { if (data) setCfg(data); })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const save = async () => {
    setSaving(true);
    setMsg(null);
    try {
      const saved = await notificationService.updateConfig({ ...cfg, type: "EMAIL" });
      setCfg(saved);
      setMsg({ text: "Email configuration saved.", ok: true });
    } catch {
      setMsg({ text: "Failed to save configuration.", ok: false });
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p className="text-xs text-muted-foreground py-6 text-center">Loading config…</p>;

  return (
    <div className="space-y-6 max-w-xl">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium">Email (SMTP)</p>
          <p className="text-xs text-muted-foreground mt-0.5">Configure the outbound SMTP server used to send email notifications.</p>
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="email-enabled" className="text-xs">Enabled</Label>
          <Switch id="email-enabled" checked={cfg.enabled} onCheckedChange={(v) => setCfg((c) => ({ ...c, enabled: v }))} />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label htmlFor="host" className="text-xs">SMTP Host</Label>
          <Input id="host" className="h-8 text-xs" placeholder="smtp.example.com"
            value={cfg.host ?? ""} onChange={(e) => setCfg((c) => ({ ...c, host: e.target.value }))} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="port" className="text-xs">Port</Label>
          <Input id="port" className="h-8 text-xs" type="number" placeholder="587"
            value={cfg.port ?? ""} onChange={(e) => setCfg((c) => ({ ...c, port: Number(e.target.value) }))} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="username" className="text-xs">Username</Label>
          <Input id="username" className="h-8 text-xs" placeholder="user@example.com"
            value={cfg.username ?? ""} onChange={(e) => setCfg((c) => ({ ...c, username: e.target.value }))} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="password" className="text-xs">Password</Label>
          <Input id="password" className="h-8 text-xs" type="password" placeholder="••••••••"
            value={cfg.password ?? ""} onChange={(e) => setCfg((c) => ({ ...c, password: e.target.value }))} />
        </div>
        <div className="col-span-2 space-y-1.5">
          <Label htmlFor="fromAddress" className="text-xs">From Address</Label>
          <Input id="fromAddress" className="h-8 text-xs" placeholder="noreply@athena.lms"
            value={cfg.fromAddress ?? ""} onChange={(e) => setCfg((c) => ({ ...c, fromAddress: e.target.value }))} />
        </div>
      </div>

      {msg && (
        <p className={`text-xs px-3 py-2 rounded-md border ${msg.ok ? "bg-success/10 text-success border-success/20" : "bg-destructive/10 text-destructive border-destructive/20"}`}>
          {msg.text}
        </p>
      )}

      <Button size="sm" className="text-xs gap-1.5" onClick={save} disabled={saving}>
        {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <SaveIcon className="h-3.5 w-3.5" />}
        Save Configuration
      </Button>
    </div>
  );
}

// ─── SMS Config Tab ────────────────────────────────────────────────────────────
function SmsConfigTab() {
  const [cfg, setCfg] = useState<NotificationConfig>({
    type: "SMS", provider: "AFRICAS_TALKING", apiKey: "", apiSecret: "", senderId: "", enabled: false,
  });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null);

  useEffect(() => {
    notificationService.getConfig("SMS")
      .then((data) => { if (data) setCfg(data); })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const save = async () => {
    setSaving(true);
    setMsg(null);
    try {
      const saved = await notificationService.updateConfig({ ...cfg, type: "SMS" });
      setCfg(saved);
      setMsg({ text: "SMS configuration saved.", ok: true });
    } catch {
      setMsg({ text: "Failed to save configuration.", ok: false });
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <p className="text-xs text-muted-foreground py-6 text-center">Loading config…</p>;

  return (
    <div className="space-y-6 max-w-xl">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium">SMS — Africa's Talking</p>
          <p className="text-xs text-muted-foreground mt-0.5">Configure Africa's Talking API for outbound SMS notifications.</p>
        </div>
        <div className="flex items-center gap-2">
          <Label htmlFor="sms-enabled" className="text-xs">Enabled</Label>
          <Switch id="sms-enabled" checked={cfg.enabled} onCheckedChange={(v) => setCfg((c) => ({ ...c, enabled: v }))} />
        </div>
      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label htmlFor="apiKey" className="text-xs">API Key</Label>
          <Input id="apiKey" className="h-8 text-xs" placeholder="atsk_xxxxxxx"
            value={cfg.apiKey ?? ""} onChange={(e) => setCfg((c) => ({ ...c, apiKey: e.target.value }))} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="apiSecret" className="text-xs">API Secret</Label>
          <Input id="apiSecret" className="h-8 text-xs" type="password" placeholder="••••••••"
            value={cfg.apiSecret ?? ""} onChange={(e) => setCfg((c) => ({ ...c, apiSecret: e.target.value }))} />
        </div>
        <div className="col-span-2 space-y-1.5">
          <Label htmlFor="senderId" className="text-xs">Sender ID</Label>
          <Input id="senderId" className="h-8 text-xs" placeholder="ATHENA"
            value={cfg.senderId ?? ""} onChange={(e) => setCfg((c) => ({ ...c, senderId: e.target.value }))} />
        </div>
      </div>

      {msg && (
        <p className={`text-xs px-3 py-2 rounded-md border ${msg.ok ? "bg-success/10 text-success border-success/20" : "bg-destructive/10 text-destructive border-destructive/20"}`}>
          {msg.text}
        </p>
      )}

      <Button size="sm" className="text-xs gap-1.5" onClick={save} disabled={saving}>
        {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <SaveIcon className="h-3.5 w-3.5" />}
        Save Configuration
      </Button>
    </div>
  );
}

// ─── Send Test Tab ─────────────────────────────────────────────────────────────
function SendTestTab() {
  const [form, setForm] = useState({ recipient: "", subject: "", message: "" });
  const [sending, setSending] = useState(false);
  const [msg, setMsg] = useState<{ text: string; ok: boolean } | null>(null);

  const send = async () => {
    if (!form.recipient || !form.message) {
      setMsg({ text: "Recipient and message are required.", ok: false });
      return;
    }
    setSending(true);
    setMsg(null);
    try {
      const result = await notificationService.sendManual({ ...form, type: "EMAIL" });
      setMsg({ text: result ?? "Notification sent.", ok: true });
      setForm({ recipient: "", subject: "", message: "" });
    } catch {
      setMsg({ text: "Failed to send notification.", ok: false });
    } finally {
      setSending(false);
    }
  };

  return (
    <div className="space-y-6 max-w-xl">
      <div>
        <p className="text-sm font-medium">Send Test Notification</p>
        <p className="text-xs text-muted-foreground mt-0.5">
          Send a manual email directly via the notification-service. Useful for verifying SMTP configuration.
        </p>
      </div>

      <div className="space-y-4">
        <div className="space-y-1.5">
          <Label htmlFor="recipient" className="text-xs">Recipient Email</Label>
          <Input id="recipient" className="h-8 text-xs" type="email" placeholder="test@example.com"
            value={form.recipient} onChange={(e) => setForm((f) => ({ ...f, recipient: e.target.value }))} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="subject" className="text-xs">Subject</Label>
          <Input id="subject" className="h-8 text-xs" placeholder="Test notification from Nemo"
            value={form.subject} onChange={(e) => setForm((f) => ({ ...f, subject: e.target.value }))} />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="message" className="text-xs">Message</Label>
          <Textarea id="message" className="text-xs min-h-[100px]" placeholder="Message body…"
            value={form.message} onChange={(e) => setForm((f) => ({ ...f, message: e.target.value }))} />
        </div>
      </div>

      {msg && (
        <p className={`text-xs px-3 py-2 rounded-md border ${msg.ok ? "bg-success/10 text-success border-success/20" : "bg-destructive/10 text-destructive border-destructive/20"}`}>
          {msg.text}
        </p>
      )}

      <Button size="sm" className="text-xs gap-1.5" onClick={send} disabled={sending}>
        {sending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Send className="h-3.5 w-3.5" />}
        Send Email
      </Button>
    </div>
  );
}

// ─── Logs Tab ──────────────────────────────────────────────────────────────────
function LogsTab() {
  const [logs, setLogs] = useState<NotificationLog[]>([]);
  const [page, setPage] = useState(0);
  const [totalPages, setTotalPages] = useState(0);
  const [totalElements, setTotalElements] = useState(0);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    notificationService.getLogs(page, 20)
      .then((res) => {
        setLogs(res.content ?? []);
        setTotalPages(res.totalPages ?? 0);
        setTotalElements(res.totalElements ?? 0);
      })
      .catch(() => setLogs([]))
      .finally(() => setLoading(false));
  }, [page]);

  const sentCount = logs.filter((l) => l.status === "SENT").length;
  const failedCount = logs.filter((l) => l.status === "FAILED").length;

  return (
    <div className="space-y-4">
      {/* Summary row */}
      <div className="grid grid-cols-3 gap-3">
        <div className="border rounded-lg p-3">
          <p className="text-[10px] text-muted-foreground">Total Notifications</p>
          <p className="text-xl font-heading mt-0.5">{totalElements}</p>
        </div>
        <div className="border rounded-lg p-3">
          <p className="text-[10px] text-muted-foreground">Sent (this page)</p>
          <p className="text-xl font-heading text-success mt-0.5">{sentCount}</p>
        </div>
        <div className="border rounded-lg p-3">
          <p className="text-[10px] text-muted-foreground">Failed / Skipped</p>
          <p className="text-xl font-heading text-destructive mt-0.5">{failedCount}</p>
        </div>
      </div>

      {/* Table */}
      {loading ? (
        <p className="text-xs text-muted-foreground py-6 text-center">Loading logs…</p>
      ) : logs.length === 0 ? (
        <p className="text-xs text-muted-foreground py-8 text-center">
          No notification logs yet. Logs appear when LMS lifecycle events trigger notifications
          (loan submitted, disbursed, repayment, KYC).
        </p>
      ) : (
        <>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-xs">When</TableHead>
                <TableHead className="text-xs">Service</TableHead>
                <TableHead className="text-xs">Type</TableHead>
                <TableHead className="text-xs">Recipient</TableHead>
                <TableHead className="text-xs">Subject</TableHead>
                <TableHead className="text-xs">Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((log) => (
                <TableRow key={log.id} className="table-row-hover">
                  <TableCell className="text-xs font-mono whitespace-nowrap">
                    {new Date(log.sentAt).toLocaleString()}
                  </TableCell>
                  <TableCell className="text-xs">{log.serviceName}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-1.5 text-xs">
                      {log.type === "EMAIL" ? <Mail className="h-3.5 w-3.5" /> : <MessageSquare className="h-3.5 w-3.5" />}
                      {log.type}
                    </div>
                  </TableCell>
                  <TableCell className="text-xs max-w-[160px] truncate">{log.recipient}</TableCell>
                  <TableCell className="text-xs max-w-[200px] truncate text-muted-foreground">
                    {log.subject ?? "—"}
                  </TableCell>
                  <TableCell>{statusBadge(log.status)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>

          {totalPages > 1 && (
            <div className="flex items-center justify-between pt-3 border-t border-border">
              <p className="text-xs text-muted-foreground">Page {page + 1} of {totalPages}</p>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" className="text-xs" onClick={() => setPage((p) => p - 1)} disabled={page === 0}>
                  <ChevronLeft className="h-3.5 w-3.5 mr-1" /> Prev
                </Button>
                <Button size="sm" variant="outline" className="text-xs" onClick={() => setPage((p) => p + 1)} disabled={page >= totalPages - 1}>
                  Next <ChevronRight className="h-3.5 w-3.5 ml-1" />
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ─── Page ──────────────────────────────────────────────────────────────────────
const NotificationsPage = () => (
  <DashboardLayout
    title="Notifications"
    subtitle="Configure channels, send test messages, and view the dispatch audit trail"
    breadcrumbs={[{ label: "Home", href: "/" }, { label: "Administration" }, { label: "Notifications" }]}
  >
    <div className="animate-fade-in">
      <Tabs defaultValue="logs">
        <TabsList className="mb-6">
          <TabsTrigger value="logs" className="text-xs gap-1.5"><Bell className="h-3.5 w-3.5" />Logs</TabsTrigger>
          <TabsTrigger value="email" className="text-xs gap-1.5"><Mail className="h-3.5 w-3.5" />Email Config</TabsTrigger>
          <TabsTrigger value="sms" className="text-xs gap-1.5"><MessageSquare className="h-3.5 w-3.5" />SMS Config</TabsTrigger>
          <TabsTrigger value="send" className="text-xs gap-1.5"><Send className="h-3.5 w-3.5" />Send Test</TabsTrigger>
        </TabsList>

        <TabsContent value="logs">
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <CardTitle className="text-sm font-medium">Notification Log</CardTitle>
                <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
                  <Settings2 className="h-3.5 w-3.5" />
                  Live from notification-service
                </div>
              </div>
            </CardHeader>
            <CardContent><LogsTab /></CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="email">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">Email Configuration</CardTitle>
              <CardDescription className="text-xs">Settings are persisted in the notification-service database.</CardDescription>
            </CardHeader>
            <CardContent><EmailConfigTab /></CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="sms">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">SMS Configuration</CardTitle>
              <CardDescription className="text-xs">Africa's Talking integration for outbound SMS messages.</CardDescription>
            </CardHeader>
            <CardContent><SmsConfigTab /></CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="send">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm font-medium">Send Test Notification</CardTitle>
              <CardDescription className="text-xs">Verify channel configuration by sending a real message now.</CardDescription>
            </CardHeader>
            <CardContent><SendTestTab /></CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  </DashboardLayout>
);

export default NotificationsPage;
