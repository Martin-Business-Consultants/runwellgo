// The install's custom fields for a model (Client, Engagement, Todo), read
// once a session.
import { get } from "./api.js";

const cache = {};

export async function customFields(model) {
  cache[model] ||= get("/custom_fields?model=" + model).then((r) => r.fields).catch(() => []);
  return cache[model];
}
