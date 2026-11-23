import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

import { intlLocale } from "@/i18n";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Helper for onInteractOutside close decisions in sheets/dialogs (Sheet/Dialog).
//
// Background: Radix popovers inside a sheet (Select dropdowns, DropdownMenu, Popover, etc.)
// portal outside the sheet. With a popover open, clicking the overlay or outside the sheet
// to dismiss it makes this one pointerdown get handled by both the Select and the Sheet
// DismissableLayers; Select closes first as a discrete event React flushes synchronously, so
// by the time Sheet's handler runs the popover's data-state has long flipped to closed — detecting "right now" whether a popover is open is inherently unreliable (verified empirically).
//
// Correct approach: Radix listens for pointerdown in the bubble phase; we record in the
// capture phase (earlier) whether any popover is open right now, and onInteractOutside reads that recorded value to decide whether to allow closing.
function isRadixOverlayOpenNow(): boolean {
  if (typeof document === "undefined") return false;
  return !!document.querySelector(
    [
      "[data-slot='select-trigger'][data-state='open']",
      "[data-slot='select-content'][data-state='open']",
      "[role='listbox'][data-state='open']",
      "[data-radix-popper-content-wrapper]",
      "[aria-expanded='true'][data-state='open']",
    ].join(","),
  );
}

let overlayOpenAtLastPointerDown = false;
if (typeof document !== "undefined") {
  document.addEventListener(
    "pointerdown",
    () => {
      overlayOpenAtLastPointerDown = isRadixOverlayOpenNow();
    },
    true, // capture: record before Radix's bubble-phase pointerdown handlers
  );
}

// radixOverlayWasOpenAtPointerDown returns whether a Radix popover was open at the most
// recent pointerdown. Sheets/dialogs use it: overlay click with a popover open → dismiss only the popover, keep themselves open.
export function radixOverlayWasOpenAtPointerDown(): boolean {
  return overlayOpenAtLastPointerDown;
}

// copyText writes text to the clipboard and returns whether it succeeded.
// Background: navigator.clipboard is only available in secure contexts (HTTPS /
// localhost); it is undefined when accessed via IP + HTTP, so we fall back to execCommand("copy").
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // continue with the fallback
    }
  }
  try {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    textarea.style.top = "0";
    document.body.appendChild(textarea);
    textarea.focus();
    textarea.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(textarea);
    return ok;
  } catch {
    return false;
  }
}

export const getInitials = (str: string): string => {
  if (typeof str !== "string" || !str.trim()) return "?";

  return (
    str
      .trim()
      .split(/\s+/)
      .filter(Boolean)
      .map((word) => word[0])
      .join("")
      .toUpperCase() || "?"
  );
};

export function formatCurrency(
  amount: number,
  opts?: {
    currency?: string;
    locale?: string;
    minimumFractionDigits?: number;
    maximumFractionDigits?: number;
    noDecimals?: boolean;
  },
) {
  const { currency = "USD", locale = intlLocale, minimumFractionDigits, maximumFractionDigits, noDecimals } = opts ?? {};

  const formatOptions: Intl.NumberFormatOptions = {
    style: "currency",
    currency,
    minimumFractionDigits: noDecimals ? 0 : minimumFractionDigits,
    maximumFractionDigits: noDecimals ? 0 : maximumFractionDigits,
  };

  return new Intl.NumberFormat(locale, formatOptions).format(amount);
}
