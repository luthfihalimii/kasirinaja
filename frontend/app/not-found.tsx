import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-[var(--c-bg)] px-4 text-center">
      <h1 className="font-display text-4xl uppercase text-[var(--c-title)]">Halaman Tidak Ditemukan</h1>
      <p className="max-w-xl text-sm text-[var(--c-text-muted)]">
        Path yang kamu buka tidak tersedia di dashboard KasirinAja.
      </p>
      <Link
        href="/"
        className="rounded-xl border border-[var(--c-border)] bg-[var(--c-panel)] px-4 py-2 text-sm text-[var(--c-title)] transition-colors hover:bg-[var(--c-panel-soft)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--c-accent)]"
      >
        Kembali ke Terminal
      </Link>
    </div>
  );
}
