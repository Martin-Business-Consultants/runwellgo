// What the agreement screens share: the contacts to send to (approvers
// first) and money typed in dollars, kept as cents.

// contactOptions: [{value, label}], approvers first, then by name.
export function contactOptions(contacts, { emailOnly = false } = {}) {
  return contacts
    .filter((c) => !emailOnly || c.email)
    .sort((a, b) => (a.can_approve === b.can_approve ? a.name.localeCompare(b.name) : a.can_approve ? -1 : 1))
    .map((c) => ({ value: String(c.id), label: [c.name, c.can_approve ? "can approve" : null].filter(Boolean).join(" · ") }));
}

// defaultApprover: the first who can approve and has an email.
export const defaultApprover = (contacts) => {
  const c = contacts.find((c) => c.can_approve && c.email);
  return c ? String(c.id) : "";
};

// dollars(150000) → "1500.00"; blank for nothing.
export const dollars = (cents) => (cents ? (cents / 100).toFixed(2) : "");

// toCents("1,500.50") → 150050; 0 for blank or nonsense.
export function toCents(text) {
  const n = parseFloat(String(text ?? "").replace(/[,$\s]/g, ""));
  return isNaN(n) ? 0 : Math.round(n * 100);
}
