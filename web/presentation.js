import { escapeHTML } from "./ui.js";
export const lookupName = (collection, id) =>
  collection.find((item) => item.id === id)?.name || id;
export const formatAmount = (value) => {
  const [whole, fraction] = String(value).split(".");
  return (
    whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",") +
    (fraction ? `.${fraction}` : "")
  );
};
export const formatTimestamp = (value) =>
  new Intl.DateTimeFormat("en-GB", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "UTC",
    hour12: false,
  }).format(new Date(value));
export function renderStatusBadge(status) {
  const labels = {
    PENDING_SIGNATURE: "Awaiting signature",
    SENT: "Sent",
    CONFIRMED: "Confirmed",
    FAILED: "Failed",
    REJECTED: "Rejected",
    REJECTED_BY_POLICY: "Policy rejected",
    NO_POLICY_FOUND: "No matching policy",
    CANCELLED: "Cancelled",
  };
  const color =
    status === "CONFIRMED"
      ? "green"
      : status === "SENT" || status === "PENDING_SIGNATURE"
        ? "blue"
        : status === "FAILED" || status.includes("REJECT")
          ? "red"
          : "";
  return `<span class="badge ${color}">${escapeHTML(labels[status] || status)}</span>`;
}
export function renderPolicyConditions(policy, catalog) {
  const parts = [];
  if (policy.initiator_id)
    parts.push(`By ${lookupName(catalog.people, policy.initiator_id)}`);
  parts.push(
    policy.sender_type === "any"
      ? "Any wallet"
      : lookupName(
          policy.sender_type === "wallet"
            ? catalog.wallets
            : catalog.wallet_groups,
          policy.sender_id,
        ),
  );
  parts.push(
    policy.receiver_type === "any"
      ? "Any receiver"
      : lookupName(
          policy.receiver_type === "recipient"
            ? catalog.recipients
            : catalog.recipient_groups,
          policy.receiver_id,
        ),
  );
  parts.push(policy.currency);
  return (
    parts
      .map((part) => `<span class="chip">${escapeHTML(part)}</span>`)
      .join("") +
    `<span class="chip amount">${escapeHTML(describeAmountPolicy(policy))}</span>`
  );
}
export function describeAmountPolicy(policy) {
  const rule = policy.amount_policy;
  if (!rule) return "Any amount";
  const limit = `≤ ${formatAmount(rule.amount)} ${policy.currency}`;
  return rule.amount_type === "interval"
    ? `${limit} / ${rule.interval} per wallet`
    : `${limit} per transfer`;
}
