import { query, escapeHTML } from "./ui.js";
import { formatAmount } from "./presentation.js";

export function createWalletsView(workspace) {
  return {
    render() {
      const state = workspace.getState();
      query("#wallets-list").innerHTML = state.wallets.length
        ? `<div class="table-scroll"><table><thead><tr><th>WALLET</th><th>CURRENCY</th><th>BALANCE</th><th>RESERVED</th><th>AVAILABLE</th><th>SPENT 24H</th><th>SPENT 30D</th></tr></thead><tbody>${state.wallets
            .map((wallet) => {
              const groups = state.catalog.wallet_groups.filter(
                (group) =>
                  group.profile_id === state.profile_id &&
                  group.members.includes(wallet.id),
              );
              return `<tr data-wallet="${escapeHTML(wallet.id)}"><td><strong>${escapeHTML(wallet.name)}</strong><span class="cell-sub">${escapeHTML(groups.map((group) => group.name).join(", ") || "No group")}</span></td><td><span class="chip">${escapeHTML(wallet.currency)}</span></td><td class="amount-cell" data-balance="balance">${escapeHTML(formatAmount(wallet.balance))}</td><td class="amount-cell" data-balance="reserved">${escapeHTML(formatAmount(wallet.reserved))}</td><td class="amount-cell wallet-available" data-balance="available">${escapeHTML(formatAmount(wallet.available))}</td><td class="amount-cell" data-balance="spent-24h">${escapeHTML(formatAmount(wallet.spent_24h))}</td><td class="amount-cell" data-balance="spent-30d">${escapeHTML(formatAmount(wallet.spent_30d))}</td></tr>`;
            })
            .join("")}</tbody></table></div>`
        : '<div class="empty">No wallets in this profile.</div>';
    },
  };
}
