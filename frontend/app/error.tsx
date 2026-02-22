"use client";

import { useEffect } from "react";

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-[var(--c-bg)] px-4 text-center">
      <h1 className="font-display text-4xl uppercase text-[var(--c-title)]">Terjadi Gangguan</h1>
      <p className="max-w-xl text-sm text-[var(--c-text-muted)]">
        Sistem POS gagal memuat halaman ini. Coba muat ulang atau kembali beberapa saat lagi.
      </p>
      <button
        type="button"
        onClick={reset}
        className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel)] px-4 py-2 text-sm text-[var(--c-title)] transition-colors hover:bg-[var(--c-panel-soft)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--c-accent)]"
      >
        Coba Lagi
      </button>
    </div>
  );
}
