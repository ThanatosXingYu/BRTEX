// Mock switch. A public variable injected at build time (only the NEXT_PUBLIC_ prefix is readable in the browser).
// On Vercel, setting NEXT_PUBLIC_MOCK=1 runs the whole site on mock with no backend needed.
export const MOCK = process.env.NEXT_PUBLIC_MOCK === "1";
