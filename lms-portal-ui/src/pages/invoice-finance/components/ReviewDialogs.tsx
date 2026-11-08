import { useState, useEffect } from "react";
import { Loader2 } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";

interface BaseProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  pending?: boolean;
}

export function ApproveDialog({ open, onOpenChange, title, pending, onConfirm }: BaseProps & { onConfirm: (note: string) => void }) {
  const [note, setNote] = useState("");
  useEffect(() => {
    if (!open) setNote("");
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-heading">{title}</DialogTitle>
          <DialogDescription>
            Overrides a REFER decision (or provides the second approval for a large disbursement). This is written to the invoice ledger with your identity.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="approve-note">Note (optional)</Label>
          <Textarea id="approve-note" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Why this is acceptable…" rows={3} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            Cancel
          </Button>
          <Button onClick={() => onConfirm(note)} disabled={pending}>
            {pending && <Loader2 className="h-4 w-4 mr-1 animate-spin" />} Approve
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function RejectDialog({ open, onOpenChange, title, pending, onConfirm }: BaseProps & { onConfirm: (reason: string) => void }) {
  const [reason, setReason] = useState("");
  useEffect(() => {
    if (!open) setReason("");
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="font-heading">{title}</DialogTitle>
          <DialogDescription>The invoice will be declined and the seller notified. A reason is required.</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="reject-reason">Reason</Label>
          <Textarea id="reject-reason" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="e.g. Buyer concentration too high for this seller" rows={3} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => onConfirm(reason.trim())} disabled={pending || reason.trim().length < 3}>
            {pending && <Loader2 className="h-4 w-4 mr-1 animate-spin" />} Reject
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
