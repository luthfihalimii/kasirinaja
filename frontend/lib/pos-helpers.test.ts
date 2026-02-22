import { describe, expect, it } from "vitest";

import { clamp, parsePositiveInt, paymentLabel, reasonText } from "@/lib/pos-helpers";

describe("pos-helpers", () => {
  it("clamps values inside min and max", () => {
    expect(clamp(5, 1, 10)).toBe(5);
    expect(clamp(-2, 1, 10)).toBe(1);
    expect(clamp(20, 1, 10)).toBe(10);
  });

  it("parses positive integer from mixed input", () => {
    expect(parsePositiveInt("12.500")).toBe(12500);
    expect(parsePositiveInt("abc")).toBe(0);
    expect(parsePositiveInt("-10")).toBe(10);
  });

  it("maps payment method and recommendation reason labels", () => {
    expect(paymentLabel("qris")).toBe("QRIS");
    expect(reasonText("high_margin_boost")).toBe("Margin tinggi");
    expect(reasonText("unknown")).toBe("Relevan untuk keranjang");
  });
});
