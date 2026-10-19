// What switching a plugin (on, off, granted, removed) changes in the app:
// the person's permissions, the sidebar, record tabs and settings pages,
// and the screens it may draw.
import { session } from "$lib/session.svelte.js";
import { nav } from "$lib/nav.svelte.js";
import { extensions } from "./extensions.svelte.js";
import { forgetPluginScreens } from "./PluginScreen.svelte";

export async function afterSwitch() {
  forgetPluginScreens();
  await session.load();
  await Promise.all([extensions.load().catch(() => {}), nav.refresh()]);
}
