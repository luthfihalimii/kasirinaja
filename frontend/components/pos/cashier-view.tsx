"use client";

import { useEffect, useState } from "react";

import { createCashier, fetchCashiers } from "@/lib/api";
import { CashierCreateSchema } from "@/lib/schemas";
import { toastError, toastSuccess } from "@/lib/toast";
import type { CashierUser } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";

type CashierViewProps = {
  authToken: string;
};

export function CashierView({ authToken }: CashierViewProps) {
  const [cashierUsers, setCashierUsers] = useState<CashierUser[]>([]);
  const [newCashierUsername, setNewCashierUsername] = useState("");
  const [newCashierPassword, setNewCashierPassword] = useState("");
  const [isSavingCashier, setIsSavingCashier] = useState(false);

  useEffect(() => {
    void loadCashiers();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [authToken]);

  async function loadCashiers() {
    try {
      const users = await fetchCashiers(authToken);
      setCashierUsers(users);
    } catch (error) {
      console.error("[cashier-view] failed to load cashiers:", error);
    }
  }

  async function handleCreateCashier() {
    const parsed = CashierCreateSchema.safeParse({
      username: newCashierUsername.trim().toLowerCase(),
      password: newCashierPassword,
    });

    if (!parsed.success) {
      const firstError = parsed.error.issues[0];
      toastError(firstError?.message ?? "Input tidak valid.");
      return;
    }

    setIsSavingCashier(true);
    try {
      const cashier = await createCashier(authToken, {
        username: parsed.data.username,
        password: parsed.data.password,
      });
      setNewCashierUsername("");
      setNewCashierPassword("");
      await loadCashiers();
      toastSuccess(`Kasir ${cashier.username} berhasil dibuat.`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Gagal menambah kasir";
      toastError(message);
    } finally {
      setIsSavingCashier(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Manajemen Kasir</CardTitle>
        <CardDescription>Tambah akun kasir baru untuk login terminal.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid grid-cols-1 gap-2 md:grid-cols-3">
          <Input
            value={newCashierUsername}
            onChange={(event) => setNewCashierUsername(event.target.value)}
            placeholder="username kasir"
          />
          <Input
            type="password"
            value={newCashierPassword}
            onChange={(event) => setNewCashierPassword(event.target.value)}
            placeholder="password (min 6)"
          />
          <Button onClick={() => void handleCreateCashier()} disabled={isSavingCashier}>
            {isSavingCashier ? "Menyimpan..." : "Tambah Kasir"}
          </Button>
        </div>
        <div className="max-h-56 overflow-auto rounded-lg border border-[var(--c-border)] bg-[var(--c-panel-soft)] p-2 text-xs text-[var(--c-text-muted)]">
          {cashierUsers.length === 0 ? (
            <p>Belum ada user kasir tambahan.</p>
          ) : (
            cashierUsers.map((user) => (
              <p key={user.username}>
                {user.username} | aktif: {user.active ? "ya" : "tidak"} | dibuat:{" "}
                {new Date(user.created_at).toLocaleString("id-ID")}
              </p>
            ))
          )}
        </div>
      </CardContent>
    </Card>
  );
}
