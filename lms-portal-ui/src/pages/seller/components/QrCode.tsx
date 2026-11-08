import { useEffect, useState } from "react";
import QRCode from "qrcode";
import { cn } from "@/lib/utils";

/**
 * QR code rendered as inline SVG via the `qrcode` package (pinned 1.5.4).
 * Always drawn on a white tile so scanners work in dark mode too.
 */
export function QrCode({ value, size = 160, className, label }: { value: string; size?: number; className?: string; label?: string }) {
  const [svg, setSvg] = useState<string>("");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setErr(null);
    QRCode.toString(value, { type: "svg", errorCorrectionLevel: "M", margin: 1, color: { dark: "#06222B", light: "#FFFFFF" } })
      .then((s) => {
        if (!cancelled) setSvg(s);
      })
      .catch((e: Error) => {
        if (!cancelled) setErr(e.message);
      });
    return () => {
      cancelled = true;
    };
  }, [value]);

  if (err) return <p className="text-xs text-destructive">QR unavailable: {err}</p>;

  return (
    <figure className={cn("inline-flex flex-col items-center gap-2", className)}>
      <div
        className="rounded-lg bg-white p-2 shadow-card [&_svg]:block [&_svg]:h-full [&_svg]:w-full"
        style={{ width: size, height: size }}
        role="img"
        aria-label={label ?? "QR code"}
        dangerouslySetInnerHTML={{ __html: svg }}
      />
      {label && <figcaption className="text-xs text-muted-foreground text-center max-w-[200px]">{label}</figcaption>}
    </figure>
  );
}
