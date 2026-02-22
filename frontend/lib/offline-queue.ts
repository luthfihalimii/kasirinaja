import type { OfflineTransaction } from "@/lib/types";

const DB_NAME = "kasirinaja_offline";
const DB_VERSION = 1;
const STORE_NAME = "checkout_outbox";

type OfflineCheckoutRecord = OfflineTransaction & {
  queued_at: string;
};

function isBrowser(): boolean {
  return typeof window !== "undefined" && "indexedDB" in window;
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (!isBrowser()) {
      reject(new Error("indexedDB is not available"));
      return;
    }

    const request = window.indexedDB.open(DB_NAME, DB_VERSION);

    request.onupgradeneeded = () => {
      const db = request.result;
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME, { keyPath: "client_transaction_id" });
      }
    };

    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("Failed to open indexedDB"));
  });
}

export async function enqueueOfflineCheckout(
  tx: OfflineTransaction,
): Promise<void> {
  const db = await openDB();

  await new Promise<void>((resolve, reject) => {
    const transaction = db.transaction(STORE_NAME, "readwrite");
    const store = transaction.objectStore(STORE_NAME);
    const request = store.put({ ...tx, queued_at: new Date().toISOString() });

    request.onsuccess = () => resolve();
    request.onerror = () => reject(request.error ?? new Error("Failed to queue checkout"));
  });

  db.close();
}

export async function listOfflineCheckouts(): Promise<OfflineTransaction[]> {
  const db = await openDB();

  const records = await new Promise<OfflineCheckoutRecord[]>((resolve, reject) => {
    const transaction = db.transaction(STORE_NAME, "readonly");
    const store = transaction.objectStore(STORE_NAME);
    const request = store.getAll();

    request.onsuccess = () => {
      const result = (request.result as OfflineCheckoutRecord[]).sort((a, b) =>
        a.queued_at.localeCompare(b.queued_at),
      );
      resolve(result);
    };

    request.onerror = () =>
      reject(request.error ?? new Error("Failed to read offline queue"));
  });

  db.close();
  return records.map((entry) => ({
    client_transaction_id: entry.client_transaction_id,
    checkout: entry.checkout,
  }));
}

export async function removeOfflineCheckouts(
  clientTransactionIDs: string[],
): Promise<void> {
  if (clientTransactionIDs.length === 0) {
    return;
  }

  const db = await openDB();

  await new Promise<void>((resolve, reject) => {
    const transaction = db.transaction(STORE_NAME, "readwrite");
    const store = transaction.objectStore(STORE_NAME);

    for (const id of clientTransactionIDs) {
      store.delete(id);
    }

    transaction.oncomplete = () => resolve();
    transaction.onerror = () =>
      reject(transaction.error ?? new Error("Failed to delete synced records"));
  });

  db.close();
}

export async function countOfflineCheckouts(): Promise<number> {
  const db = await openDB();

  const count = await new Promise<number>((resolve, reject) => {
    const transaction = db.transaction(STORE_NAME, "readonly");
    const store = transaction.objectStore(STORE_NAME);
    const request = store.count();

    request.onsuccess = () => resolve(request.result);
    request.onerror = () =>
      reject(request.error ?? new Error("Failed to count offline records"));
  });

  db.close();
  return count;
}
