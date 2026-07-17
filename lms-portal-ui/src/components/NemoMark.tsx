// Nemo brand mark: three clownfish stripes on the abyss tile, matching the
// concept deck's .stripes motif and the mobile app's launcher icon.
export function NemoMark({ className = "h-9 w-9" }: { className?: string }) {
  return (
    <svg viewBox="0 0 64 64" className={className} role="img" aria-label="Nemo">
      <rect width="64" height="64" rx="14" fill="#06222B" />
      <rect x="14" y="18" width="8" height="28" rx="3" fill="#FF6A3D" />
      <rect x="28" y="14" width="8" height="36" rx="3" fill="#F2F7F5" />
      <rect x="42" y="18" width="8" height="28" rx="3" fill="#FF6A3D" opacity="0.55" />
    </svg>
  );
}
