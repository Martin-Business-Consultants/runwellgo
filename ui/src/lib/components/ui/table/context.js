import { getContext, setContext } from "svelte";

// A table straight inside a frame (or a framed card) is drawn as cards:
// its head on the frame's tint, its body a white panel. Inside a panel it's
// a plain table again.
const KEY = Symbol("table-variant");
export const setTableVariant = (variant) => setContext(KEY, variant);
export const tableVariant = () => getContext(KEY) || "default";

// Whether this is already inside a frame, a card or a panel: an empty
// state standing on its own brings a frame of its own.
export const onSurface = () => getContext(KEY) !== undefined;
