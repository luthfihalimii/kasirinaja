import { z } from "zod";

// Product creation schema
export const ProductCreateSchema = z.object({
  sku: z
    .string()
    .min(1, "SKU wajib diisi")
    .transform((val) => val.toUpperCase()),
  name: z.string().min(1, "Nama produk wajib diisi"),
  category: z.string().min(1, "Kategori wajib diisi"),
  price_cents: z
    .number({ error: "Harga harus berupa angka" })
    .int("Harga harus bilangan bulat")
    .positive("Harga harus lebih dari 0"),
  margin_rate: z
    .number({ error: "Margin harus berupa angka" })
    .min(0, "Margin minimal 0")
    .max(1, "Margin maksimal 1 (100%)"),
  initial_stock: z
    .number({ error: "Stok awal harus berupa angka" })
    .int("Stok harus bilangan bulat")
    .min(0, "Stok tidak boleh negatif"),
});

export type ProductCreateInput = z.infer<typeof ProductCreateSchema>;

// Cashier creation schema
export const CashierCreateSchema = z.object({
  username: z
    .string()
    .min(4, "Username minimal 4 karakter")
    .regex(/^\S+$/, "Username tidak boleh mengandung spasi"),
  password: z.string().min(6, "Password minimal 6 karakter"),
});

export type CashierCreateInput = z.infer<typeof CashierCreateSchema>;

// Promo creation schema
export const PromoCreateSchema = z.object({
  name: z.string().min(1, "Nama promo wajib diisi"),
  type: z.enum(["cart_percent", "flat_cart"], {
    error: "Tipe promo tidak valid",
  }),
  discount_percent: z
    .number({ error: "Persentase diskon harus berupa angka" })
    .min(0, "Diskon minimal 0%")
    .max(100, "Diskon maksimal 100%"),
  flat_discount_cents: z
    .number({ error: "Flat diskon harus berupa angka" })
    .int("Flat diskon harus bilangan bulat")
    .min(0, "Flat diskon tidak boleh negatif"),
  min_subtotal_cents: z
    .number({ error: "Min subtotal harus berupa angka" })
    .int("Min subtotal harus bilangan bulat")
    .min(0, "Min subtotal tidak boleh negatif"),
});

export type PromoCreateInput = z.infer<typeof PromoCreateSchema>;

// Supplier creation schema
export const SupplierCreateSchema = z.object({
  name: z.string().min(1, "Nama supplier wajib diisi"),
  phone: z.string().default(""),
});

export type SupplierCreateInput = z.infer<typeof SupplierCreateSchema>;

// Login schema
export const LoginSchema = z.object({
  username: z.string().min(1, "Username wajib diisi"),
  password: z.string().min(1, "Password wajib diisi"),
});

export type LoginInput = z.infer<typeof LoginSchema>;
