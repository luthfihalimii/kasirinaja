"use client";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

type ConfirmDialogProps = {
  open: boolean;
  title: string;
  description: string;
  onConfirm: () => void;
  onCancel: () => void;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "default" | "destructive";
};

export function ConfirmDialog({
  open,
  title,
  description,
  onConfirm,
  onCancel,
  confirmLabel = "Ya, Lanjutkan",
  cancelLabel = "Batal",
  variant = "default",
}: ConfirmDialogProps) {
  if (!open) {
    return null;
  }

  return (
    // Backdrop
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
      aria-describedby="confirm-dialog-description"
      onClick={onCancel}
    >
      {/* Dialog panel - stop propagation so clicks inside don't close it */}
      <div
        className="w-full max-w-sm"
        onClick={(event) => event.stopPropagation()}
      >
        <Card>
          <CardHeader>
            <CardTitle id="confirm-dialog-title">{title}</CardTitle>
            <CardDescription id="confirm-dialog-description">
              {description}
            </CardDescription>
          </CardHeader>
          <CardContent />
          <CardFooter className="justify-end gap-2">
            <Button variant="outline" onClick={onCancel}>
              {cancelLabel}
            </Button>
            <Button
              variant={variant === "destructive" ? "danger" : "default"}
              onClick={onConfirm}
            >
              {confirmLabel}
            </Button>
          </CardFooter>
        </Card>
      </div>
    </div>
  );
}
