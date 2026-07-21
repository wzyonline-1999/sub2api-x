import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { createPinia } from "pinia";
import type { SubscriptionPlan } from "@/types/payment";
import SubscriptionPlanCard from "../SubscriptionPlanCard.vue";

const translations = vi.hoisted<Record<string, string>>(() => ({
  "payment.days": "days",
  "payment.perMonth": "month",
  "payment.perYear": "year",
  "payment.renewNow": "Renew now",
  "payment.subscribeNow": "Subscribe now",
  "payment.planCard.dailyLimit": "Daily limit",
  "payment.planCard.models": "Models",
  "payment.planCard.peakRate": "Peak rate",
  "payment.planCard.quota": "Quota",
  "payment.planCard.rate": "Rate",
  "payment.planCard.unlimited": "Unlimited",
  "payment.planCard.weeklyLimit": "Weekly limit",
  "payment.planCard.monthlyLimit": "Monthly limit",
}));

vi.mock("vue-i18n", async (importOriginal) => {
  const actual = await importOriginal<typeof import("vue-i18n")>();
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => translations[key] ?? key,
    }),
  };
});

const mountPlanCard = (groupPlatform: string, overrides: Partial<SubscriptionPlan> = {}) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: {
        id: 1,
        group_id: 10,
        group_platform: groupPlatform,
        name: "Pro",
        price: 10,
        amount: 1000,
        features: [],
        rate_multiplier: 1,
        validity_days: 30,
        validity_unit: "day",
        supported_model_scopes: ["claude", "gemini_text", "gemini_image"],
        is_active: true,
        ...overrides,
      },
    },
    global: { plugins: [createPinia()] },
  });

describe("SubscriptionPlanCard", () => {
  it("does not show Antigravity model scopes for OpenAI plans", () => {
    const text = mountPlanCard("openai").text();

    expect(text).not.toContain("Claude");
    expect(text).not.toContain("Gemini");
    expect(text).not.toContain("Imagen");
  });

  it("shows model scopes for Antigravity plans", () => {
    const text = mountPlanCard("antigravity").text();

    expect(text).toContain("Claude");
    expect(text).toContain("Gemini");
    expect(text).toContain("Imagen");
  });

  it("uses the configured currency symbol while preserving USD for legacy plans", () => {
    const cnyPlan = mountPlanCard("openai", { currency: "CNY", original_price: 20 }).text();

    expect(cnyPlan).toContain("¥10CNY");
    expect(cnyPlan).toContain("¥20CNY");
    expect(mountPlanCard("openai", { currency: "USD" }).text()).toContain("$10USD");
    expect(mountPlanCard("openai", { currency: "" }).text()).toContain("$10");
  });
});
