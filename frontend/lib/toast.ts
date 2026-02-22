// Thin wrapper around sonner's toast with typed convenience helpers.
import { toast as sonnerToast } from "sonner";

export { sonnerToast as toast };

export function toastSuccess(msg: string): void {
  sonnerToast.success(msg);
}

export function toastError(msg: string): void {
  sonnerToast.error(msg);
}

export function toastInfo(msg: string): void {
  sonnerToast.info(msg);
}
