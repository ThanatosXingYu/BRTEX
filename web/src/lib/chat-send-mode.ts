"use client";

import * as React from "react";

import { t } from "@/i18n";
import { getLocalStorageValue, setLocalStorageValue } from "@/lib/local-storage.client";

// Send/newline key bindings for the chat input. Pure client-side preference: localStorage
// only, never stored in the database nor synced per account, so a new browser needs a
// re-set. See issue #39 — 0.3.2 turned Ctrl+Enter send into Enter send; this brings the old binding back as an option.
export type ChatSendMode = "enter" | "ctrl-enter";

export const CHAT_SEND_MODE_KEY = "artex_chat_send_mode";
export const DEFAULT_CHAT_SEND_MODE: ChatSendMode = "enter";

export const CHAT_SEND_MODE_OPTIONS: { value: ChatSendMode; label: string }[] = [
  { value: "enter", label: t("Enter to send, Shift+Enter for newline") },
  { value: "ctrl-enter", label: t("Ctrl+Enter to send, Enter for newline") },
];

function parseMode(raw: string | null): ChatSendMode {
  return raw === "ctrl-enter" || raw === "enter" ? raw : DEFAULT_CHAT_SEND_MODE;
}

// Subscribers within the same tab. The localStorage storage event only fires in "other"
// tabs, so after changing the setting on this page we must emit to same-tab inputs, otherwise a refresh is needed for it to take effect.
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  window.addEventListener("storage", listener);
  return () => {
    listeners.delete(listener);
    window.removeEventListener("storage", listener);
  };
}

// Returns a string literal; Object.is compares by value, so useSyncExternalStore cannot loop.
function getSnapshot(): ChatSendMode {
  return parseMode(getLocalStorageValue(CHAT_SEND_MODE_KEY));
}

// No localStorage on the server: render the default first, then getSnapshot corrects after hydration.
function getServerSnapshot(): ChatSendMode {
  return DEFAULT_CHAT_SEND_MODE;
}

export function useChatSendMode(): ChatSendMode {
  return React.useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

export function setChatSendMode(mode: ChatSendMode) {
  setLocalStorageValue(CHAT_SEND_MODE_KEY, mode);
  for (const listener of listeners) listener();
}

// shouldSubmitOnKey decides whether a key press should send.
// isComposing / keyCode 229 means an IME (e.g. Chinese) is still composing; it must pass through, or Enter picking a candidate would send by mistake.
// enter mode excludes only Shift, matching 0.3.2 verbatim — users who keep the default feel no difference.
// ctrl-enter mode accepts both Ctrl and Cmd (macOS).
export function shouldSubmitOnKey(e: React.KeyboardEvent, mode: ChatSendMode): boolean {
  if (e.key !== "Enter") return false;
  if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return false;
  if (mode === "ctrl-enter") return e.ctrlKey || e.metaKey;
  return !e.shiftKey;
}
