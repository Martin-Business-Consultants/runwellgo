// Chat (plugins/chat): team chat in place of Slack, at /chat, with its
// settings under Settings › You. Shown only while the plugin is on.
import { addSlot } from "$lib/plugin-slots.js";
import { addSettingsPage } from "$lib/settings-nav.js";
import { isOn } from "../../routes/plugins/extensions.svelte.js";
import ChatPage from "./ChatPage.svelte";
import ChatSettings from "./ChatSettings.svelte";
import ChatWatcher from "./ChatWatcher.svelte";

export const key = "chat";
export const routes = [["/chat", ChatPage], ["/settings/chat", ChatSettings]];

export function setup() {
  addSlot("header", { key, component: ChatWatcher, order: 90 });
  addSettingsPage({ href: "/settings/chat", label: "Chat", group: "You", order: 30, when: () => isOn(key) });
}
