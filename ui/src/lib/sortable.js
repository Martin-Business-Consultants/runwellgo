// Drag-to-reorder for a list held in state, with SortableJS:
//   <div {@attach sortable(() => items)}>
//     {#each items as item (item)}<div data-sortable-item><DragHandle …/>…</div>{/each}
//   </div>
// Rows are dragged by their [data-drag-handle]. On a drop the row is put
// back where Svelte drew it and the list itself is reordered, so Svelte
// stays the one moving the DOM. Lists can nest; each moves only its own rows.
import Sortable from "sortablejs";

export function move(list, from, to) {
  if (to < 0 || to >= list.length || from === to) return;
  const [x] = list.splice(from, 1);
  list.splice(to, 0, x);
}

export function sortable(getList) {
  return (el) => {
    const s = Sortable.create(el, {
      draggable: ">[data-sortable-item]",
      handle: "[data-drag-handle]",
      animation: 150,
      ghostClass: "opacity-40",
      onEnd({ item, oldDraggableIndex: from, newDraggableIndex: to }) {
        if (from === to) return;
        const others = [...el.children].filter((c) => c !== item && c.matches("[data-sortable-item]"));
        el.insertBefore(item, others[from] ?? others[others.length - 1]?.nextSibling ?? null);
        move(getList(), from, to);
      },
    });
    return () => s.destroy();
  };
}
