// Settings › Appearance, applied: the install's colors (each scheme mapped
// onto shadcn's tokens: --primary, --ring, --sidebar-primary…), corners
// (--radius), font (--app-font), text size (the root font size, which every
// rem scales from; a person's own beats the install's), the theme everyone
// starts on, the logo and favicon, and custom CSS last (left out on client
// pages unless it's meant for them, and anywhere with ?theme=off). Loaded
// once at startup from GET /api/appearance, open to everyone, and again
// whenever Settings › Appearance saves.
import { setMode } from "mode-watcher";

// Each scheme's accent, light and dark, and the text that sits on it.
const SCHEMES = {
  fizzy: { light: "#5d63cd", dark: "#8b90f0" },
  forest: { light: "oklch(0.55 0.162 147)", dark: "oklch(0.7 0.16 147)" },
  plum: { light: "oklch(0.58 0.21 310)", dark: "oklch(0.7 0.18 310)" },
  ocean: { light: "oklch(0.555 0.122 210)", dark: "oklch(0.7 0.12 210)" },
  graphite: { light: "oklch(0.38 0.012 275)", dark: "oklch(0.88 0.005 275)", darkText: "oklch(0.205 0 0)" },
};
const RADII = { sharp: "0.25rem", fizzy: "0.625rem", round: "1rem" };
// Local font stacks only, nothing downloaded; System is the app's own.
const FONTS = {
  humanist: 'Seravek, "Gill Sans Nova", Ubuntu, Calibri, "DejaVu Sans", source-sans-pro, sans-serif',
  rounded: 'ui-rounded, "Hiragino Maru Gothic ProN", Quicksand, Comfortaa, Manjari, "Arial Rounded MT", "Arial Rounded MT Bold", Calibri, source-sans-pro, sans-serif',
  serif: '"Iowan Old Style", "Palatino Linotype", "URW Palladio L", P052, Georgia, serif',
  mono: 'ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, "DejaVu Sans Mono", monospace',
};
// The root font size each text size sets (Default is the browser's 16px).
export const TEXT_SIZES = { small: "14px", default: "16px", large: "18px", larger: "20px" };
export const SCHEME_SWATCHES = Object.fromEntries(Object.entries(SCHEMES).map(([k, v]) => [k, v.light]));
export const FONT_STACKS = { system: "var(--app-font)", ...FONTS };
export const RADIUS_VALUES = RADII;

const THEME_KEY = "rw-install-theme";

function styleTag(id) {
  let el = document.getElementById(id);
  if (!el) {
    el = document.createElement("style");
    el.id = id;
    document.head.appendChild(el);
  }
  return el;
}

class Appearance {
  theme = $state("system");
  scheme = $state("fizzy");
  radius = $state("fizzy");
  font = $state("system");
  textSize = $state("default");
  customCSS = $state("");
  everywhere = $state(false);
  logoUrl = $state(null);
  faviconUrl = $state(null);
  // A person's own text size (Settings › Your account), when they chose one.
  own = $state(null);
  #cssOn = true;

  async load() {
    try {
      const { get } = await import("./api.js");
      this.set(await get("/appearance", { quiet: true }));
    } catch {
      this.apply();
    }
  }

  // set takes a /api/appearance (or Settings › Appearance) answer.
  set(a) {
    if (!a) return;
    this.theme = a.theme || "system";
    this.scheme = a.scheme || "fizzy";
    this.radius = a.radius || "fizzy";
    this.font = a.font || "system";
    this.textSize = a.text_size || "default";
    this.customCSS = a.custom_css || "";
    this.everywhere = !!a.custom_css_everywhere;
    if ("logo_url" in a) this.logoUrl = a.logo_url || null;
    if ("favicon_url" in a) this.faviconUrl = a.favicon_url || null;
    this.apply();
    this.#applyTheme();
  }

  // personal sets the person's own text size (null follows the install's).
  personal(size) {
    this.own = size || null;
    this.#applySize();
  }

  // useCustomCSS turns custom CSS on or off for the screen on view.
  useCustomCSS(on) {
    this.#cssOn = on;
    this.#applyCSS();
  }

  apply() {
    const s = SCHEMES[this.scheme] || SCHEMES.fizzy;
    // Light text on the light theme's accent; in the dark theme every accent
    // is light, so the text on it is the dark ground's.
    const onLight = "#ffffff";
    const onDark = s.darkText || "#12141c";
    const block = (accent, text) =>
      `--primary: ${accent}; --primary-foreground: ${text}; --ring: ${accent}; --sidebar-primary: ${accent}; --sidebar-primary-foreground: ${text}; --sidebar-ring: ${accent}; --chart-1: ${accent};`;
    const font = FONTS[this.font] ? `--app-font: ${FONTS[this.font]};` : "";
    const radius = `--radius: ${RADII[this.radius] || RADII.fizzy};`;
    // Indigo on Soft corners is the app as it ships: nothing to change.
    const plain = this.scheme === "fizzy" && this.radius === "fizzy" && !font;
    styleTag("rw-appearance").textContent = plain ? "" : `:root { ${block(s.light, onLight)} ${radius} ${font} }\n.dark { ${block(s.dark, onDark)} }`;
    this.#applySize();
    this.#applyCSS();
    this.#applyFavicon();
  }

  #applySize() {
    const size = TEXT_SIZES[this.own] || TEXT_SIZES[this.textSize] || TEXT_SIZES.default;
    document.documentElement.style.fontSize = size === TEXT_SIZES.default ? "" : size;
  }

  #applyCSS() {
    const off = new URLSearchParams(location.search).get("theme") === "off";
    styleTag("rw-custom-css").textContent = this.#cssOn && !off ? this.customCSS : "";
  }

  #applyFavicon() {
    let link = document.querySelector("link[rel~='icon']");
    if (!this.faviconUrl) {
      if (link?.dataset.appearance) link.remove();
      return;
    }
    if (!link) {
      link = document.createElement("link");
      link.rel = "icon";
      document.head.appendChild(link);
    }
    link.dataset.appearance = "1";
    link.href = this.faviconUrl;
  }

  // The install's theme is where everyone starts: a new one (or picking it
  // again in Settings › Appearance, which resets everyone's) replaces the
  // sun and moon's choice once.
  #applyTheme(force = false) {
    let seen = null;
    try { seen = localStorage.getItem(THEME_KEY); } catch { /* private mode */ }
    if (!force && seen === this.theme) return;
    try { localStorage.setItem(THEME_KEY, this.theme); } catch { /* private mode */ }
    setMode(this.theme);
  }

  // resetTheme applies the install's theme now (Settings › Appearance saved it).
  resetTheme() { this.#applyTheme(true); }
}

export const appearance = new Appearance();
