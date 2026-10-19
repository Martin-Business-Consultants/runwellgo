// The rich text schema: the editor's Tiptap extensions, shared by
// TiptapEditor and scripts/richtext.mjs, so a document the script makes is
// one the editor opens as it is (and internal/fields/richtext.go accepts:
// its nodes and marks are these). tools "email" keeps to what email shows.
import StarterKit from "@tiptap/starter-kit";
import { TaskItem, TaskList } from "@tiptap/extension-list";

export function richTextExtensions(tools = "full") {
  const email = tools === "email";
  return [
    StarterKit.configure({ underline: false, link: { openOnClick: false, autolink: true }, ...(email ? { codeBlock: false, code: false } : {}) }),
    ...(email ? [] : [TaskList, TaskItem.configure({ nested: true })]),
  ];
}
