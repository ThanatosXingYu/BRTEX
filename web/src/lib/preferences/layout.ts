import { t } from "@/i18n";

// Sidebar Variant
export const SIDEBAR_VARIANT_OPTIONS = [
  { label: t("Sidebar"), value: "sidebar" },
  { label: t("Inset"), value: "inset" },
  { label: t("Floating"), value: "floating" },
] as const;
export const SIDEBAR_VARIANT_VALUES = SIDEBAR_VARIANT_OPTIONS.map((v) => v.value);
export type SidebarVariant = (typeof SIDEBAR_VARIANT_VALUES)[number];

// Sidebar Collapsible
export const SIDEBAR_COLLAPSIBLE_OPTIONS = [
  { label: t("Icon"), value: "icon" },
  { label: t("OffCanvas"), value: "offcanvas" },
] as const;
export const SIDEBAR_COLLAPSIBLE_VALUES = SIDEBAR_COLLAPSIBLE_OPTIONS.map((v) => v.value);
export type SidebarCollapsible = (typeof SIDEBAR_COLLAPSIBLE_VALUES)[number];

// Content Layout
export const CONTENT_LAYOUT_OPTIONS = [
  { label: t("Centered"), value: "centered" },
  { label: t("Full Width"), value: "full-width" },
] as const;
export const CONTENT_LAYOUT_VALUES = CONTENT_LAYOUT_OPTIONS.map((v) => v.value);
export type ContentLayout = (typeof CONTENT_LAYOUT_VALUES)[number];

// Navbar Style
export const NAVBAR_STYLE_OPTIONS = [
  { label: t("Sticky"), value: "sticky" },
  { label: t("Scroll"), value: "scroll" },
] as const;
export const NAVBAR_STYLE_VALUES = NAVBAR_STYLE_OPTIONS.map((v) => v.value);
export type NavbarStyle = (typeof NAVBAR_STYLE_VALUES)[number];
