import type { CheckoutResponse, PaymentMethod } from "@/lib/types";
import { formatCurrency } from "@/lib/utils";

export type ReceiptCartDetail = {
  sku: string;
  qty: number;
  name: string;
  price_cents: number;
  subtotal_cents: number;
};

export type ReceiptSnapshot = {
  checkout: CheckoutResponse;
  cartDetails: ReceiptCartDetail[];
  cashierName: string;
  discountCents: number;
  taxRatePercent: number;
};

export function generateId(prefix: string): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `${prefix}-${crypto.randomUUID()}`;
  }
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 1000000)}`;
}

export function reasonText(code: string): string {
  switch (code) {
    case "often_bought_together":
      return "Sering dibeli bersama";
    case "high_margin_boost":
      return "Margin tinggi";
    case "healthy_stock":
      return "Stok aman";
    case "time_slot_match":
      return "Cocok jam transaksi";
    default:
      return "Relevan untuk keranjang";
  }
}

export function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

export function parsePositiveInt(input: string): number {
  const digits = input.replace(/[^0-9]/g, "");
  if (!digits) {
    return 0;
  }
  const parsed = Number.parseInt(digits, 10);
  if (!Number.isFinite(parsed) || parsed < 0) {
    return 0;
  }
  return parsed;
}

export function parseNumber(input: string, fallback = 0): number {
  const parsed = Number(input.replace(",", "."));
  if (!Number.isFinite(parsed)) {
    return fallback;
  }
  return parsed;
}

export function paymentLabel(method: PaymentMethod): string {
  switch (method) {
    case "cash":
      return "Tunai";
    case "card":
      return "Kartu";
    case "qris":
      return "QRIS";
    case "ewallet":
      return "E-Wallet";
    case "split":
      return "Split";
    default:
      return method;
  }
}

export function buildReceiptHTML(snapshot: ReceiptSnapshot, storeID: string, terminalID: string): string {
  const transaction = snapshot.checkout;
  const recordedPaymentCents =
    transaction.payment_method === "cash"
      ? transaction.cash_received_cents
      : transaction.total_cents;
  const changeCents =
    transaction.payment_method === "cash" ? transaction.change_cents : 0;
  const lineRows = snapshot.cartDetails
    .map(
      (line) =>
        `<tr>
          <td>${escapeHTML(line.name)}<br/><small>${escapeHTML(line.sku)}</small></td>
          <td style="text-align:center;">${line.qty}</td>
          <td style="text-align:right;">${formatCurrency(line.subtotal_cents)}</td>
        </tr>`,
    )
    .join("");

  return `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <title>Receipt ${escapeHTML(transaction.transaction_id)}</title>
  <style>
    body { font-family: ui-monospace, Menlo, monospace; margin: 0; padding: 12px; }
    h1, p { margin: 0; }
    table { width: 100%; border-collapse: collapse; margin-top: 8px; }
    td { font-size: 12px; padding: 4px 0; border-bottom: 1px dashed #bbb; vertical-align: top; }
    .meta { font-size: 12px; margin-top: 6px; }
    .summary { margin-top: 10px; font-size: 12px; }
    .summary p { display: flex; justify-content: space-between; margin: 2px 0; }
    .total { font-weight: 700; border-top: 1px solid #222; padding-top: 6px; margin-top: 6px; }
  </style>
</head>
<body>
  <h1>KasirinAja</h1>
  <p class="meta">Store: ${escapeHTML(storeID)} / Terminal: ${escapeHTML(terminalID)}</p>
  <p class="meta">Kasir: ${escapeHTML(snapshot.cashierName)}</p>
  <p class="meta">Tx: ${escapeHTML(transaction.transaction_id)}</p>
  <p class="meta">Waktu: ${escapeHTML(new Date(transaction.created_at).toLocaleString("id-ID"))}</p>

  <table>
    <tbody>
      ${lineRows}
    </tbody>
  </table>

  <div class="summary">
    <p><span>Subtotal</span><span>${formatCurrency(transaction.subtotal_cents)}</span></p>
    <p><span>Diskon</span><span>${formatCurrency(transaction.discount_cents)}</span></p>
    <p><span>Pajak (${snapshot.taxRatePercent.toFixed(2)}%)</span><span>${formatCurrency(transaction.tax_cents)}</span></p>
    <p class="total"><span>Total</span><span>${formatCurrency(transaction.total_cents)}</span></p>
    <p><span>Bayar (${escapeHTML(paymentLabel(transaction.payment_method))})</span><span>${formatCurrency(recordedPaymentCents)}</span></p>
    <p><span>Kembalian</span><span>${formatCurrency(changeCents)}</span></p>
  </div>
</body>
</html>`;
}

function escapeHTML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}
