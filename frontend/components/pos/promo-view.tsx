"use client";

import { useEffect, useState } from "react";

import { createPromo, fetchPromos, setPromoActive } from "@/lib/api";
import { clamp, parseNumber, parsePositiveInt } from "@/lib/pos-helpers";
import { PromoCreateSchema } from "@/lib/schemas";
import { toastError, toastSuccess } from "@/lib/toast";
import type { PromoRule } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";

type PromoViewProps = {
  authToken: string;
};

export function PromoView({ authToken }: PromoViewProps) {
  const [promos, setPromos] = useState<PromoRule[]>([]);
  const [promoName, setPromoName] = useState("");
  const [promoType, setPromoType] = useState<"cart_percent" | "flat_cart">("cart_percent");
  const [promoMinSubtotalInput, setPromoMinSubtotalInput] = useState("0");
  const [promoDiscountPercentInput, setPromoDiscountPercentInput] = useState("10");
  const [promoFlatDiscountInput, setPromoFlatDiscountInput] = useState("0");
  const [isSavingPromo, setIsSavingPromo] = useState(false);

  useEffect(() => {
    void loadPromos();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authToken]);

  async function loadPromos() {
    try {
      const data = await fetchPromos(authToken);
      setPromos(data);
    } catch (error) {
      console.error("[promo-view] failed to load promos:", error);
    }
  }

  async function handleCreatePromo() {
    const parsed = PromoCreateSchema.safeParse({
      name: promoName.trim(),
      type: promoType,
      min_subtotal_cents: parsePositiveInt(promoMinSubtotalInput),
      discount_percent:
        promoType === "cart_percent"
          ? clamp(parseNumber(promoDiscountPercentInput, 0), 0, 100)
          : 0,
      flat_discount_cents:
        promoType === "flat_cart" ? parsePositiveInt(promoFlatDiscountInput) : 0,
    });

    if (!parsed.success) {
      const firstError = parsed.error.issues[0];
      toastError(firstError?.message ?? "Input promo tidak valid.");
      return;
    }

    setIsSavingPromo(true);
    try {
      await createPromo(authToken, {
        name: parsed.data.name,
        type: parsed.data.type,
        min_subtotal_cents: parsed.data.min_subtotal_cents,
        discount_percent: parsed.data.discount_percent,
        flat_discount_cents: parsed.data.flat_discount_cents,
      });
      setPromoName("");
      setPromoMinSubtotalInput("0");
      setPromoDiscountPercentInput("10");
      setPromoFlatDiscountInput("0");
      await loadPromos();
      toastSuccess("Promo berhasil disimpan.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menyimpan promo";
      toastError(message);
    } finally {
      setIsSavingPromo(false);
    }
  }

  async function handleTogglePromo(promoID: string, active: boolean) {
    try {
      await setPromoActive(authToken, promoID, active);
      const latestPromos = await fetchPromos(authToken);
      setPromos(latestPromos);
      toastSuccess(active ? "Promo diaktifkan." : "Promo dinonaktifkan.");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal update promo";
      toastError(message);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Promo Engine</CardTitle>
        <CardDescription>Buat promo cart percentage atau flat cart discount.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <Input
          value={promoName}
          onChange={(event) => setPromoName(event.target.value)}
          placeholder="Nama promo"
        />
        <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
          <select
            className="h-10 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] px-3 text-sm text-[var(--c-text)] outline-none"
            value={promoType}
            onChange={(event) =>
              setPromoType(event.target.value as "cart_percent" | "flat_cart")
            }
          >
            <option value="cart_percent">cart_percent</option>
            <option value="flat_cart">flat_cart</option>
          </select>
          <Input
            inputMode="numeric"
            value={promoMinSubtotalInput}
            onChange={(event) => setPromoMinSubtotalInput(event.target.value)}
            placeholder="Min subtotal"
          />
        </div>
        {promoType === "cart_percent" ? (
          <Input
            inputMode="decimal"
            value={promoDiscountPercentInput}
            onChange={(event) => setPromoDiscountPercentInput(event.target.value)}
            placeholder="Discount %"
          />
        ) : (
          <Input
            inputMode="numeric"
            value={promoFlatDiscountInput}
            onChange={(event) => setPromoFlatDiscountInput(event.target.value)}
            placeholder="Flat discount cents"
          />
        )}
        <Button onClick={() => void handleCreatePromo()} disabled={isSavingPromo}>
          {isSavingPromo ? "Menyimpan..." : "Simpan Promo"}
        </Button>
        <div className="max-h-48 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
          {promos.length === 0 ? (
            <p>Belum ada promo.</p>
          ) : (
            promos.map((promo) => (
              <div key={promo.id} className="mb-2 flex items-center justify-between gap-2">
                <span>
                  {promo.name} ({promo.type})
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => void handleTogglePromo(promo.id, !promo.active)}
                >
                  {promo.active ? "Nonaktifkan" : "Aktifkan"}
                </Button>
              </div>
            ))
          )}
        </div>
      </CardContent>
    </Card>
  );
}
