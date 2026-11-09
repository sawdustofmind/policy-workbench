import { api } from "./api.js";
import {
  query,
  escapeHTML,
  populateSelect,
  showError,
  toast,
  confirmAction,
} from "./ui.js";
import { renderPolicyConditions } from "./presentation.js";
export function createPoliciesView(workspace) {
  let state;
  let profile;
  const refresh = workspace.refresh;
  const profileEntries = (collection) =>
    collection.filter((item) => item.profile_id === profile);
  function synchronize() {
    state = workspace.getState();
    profile = workspace.getProfileId();
  }
  let orderBusy = false;
  function renderPolicyList() {
    query("#policies-list").innerHTML = state.policies.length
      ? state.policies
          .map(
            (policy, index) => `
    <article class="policy-row">
      <span class="order-number">${index + 1}</span>
      <div class="policy-summary"><div class="policy-title"><span></span>${escapeHTML(policy.name)}</div><div class="condition-chips">${renderPolicyConditions(policy, state.catalog)}</div></div>
      <div class="policy-action"><span class="badge ${policy.action === "ALLOW" ? "green" : "red"}">${policy.action === "ALLOW" ? "↗ Allow" : "× Reject"}</span><small>${policy.action === "ALLOW" ? "Transfer is allowed" : "Transfer is rejected"}</small></div>
      <div class="row-tools"><button class="icon-button" data-move="${index}" data-direction="-1" aria-label="Move ${escapeHTML(policy.name)} up" title="Move up" ${index === 0 ? "disabled" : ""}>↑</button><button class="icon-button" data-move="${index}" data-direction="1" aria-label="Move ${escapeHTML(policy.name)} down" title="Move down" ${index === state.policies.length - 1 ? "disabled" : ""}>↓</button><button class="icon-button delete" data-delete="${escapeHTML(policy.id)}" aria-label="Delete ${escapeHTML(policy.name)}" title="Delete policy">⌫</button></div>
    </article>`,
          )
          .join("")
      : '<div class="empty">No policies yet.<br>Create a policy to decide how transfers are handled.</div>';
  }
  function openCreatePolicyDialog() {
    const form = query("#policy-form");
    form.reset();
    query("#policy-error").hidden = true;
    populateSelect(form.elements.initiator_id, [
      ["", "Any member"],
      ...profileEntries(state.catalog.people).map((member) => [
        member.id,
        member.name,
      ]),
    ]);
    form.elements.initiator_id.value = "";
    updateCounterpartyFields();
    updateAmountFields();
    query("#policy-dialog").showModal();
    form.elements.name.focus();
  }
  function updateCounterpartyFields() {
    const fields = query("#policy-form").elements;
    for (const side of ["sender", "receiver"]) {
      const type = fields[`${side}_type`].value;
      const label = query(`#${side}-id-label`);
      label.hidden = type === "any";
      const list =
        side === "sender"
          ? type === "wallet"
            ? state.catalog.wallets
            : state.catalog.wallet_groups
          : type === "recipient"
            ? state.catalog.recipients
            : state.catalog.recipient_groups;
      populateSelect(
        fields[`${side}_id`],
        type === "any"
          ? []
          : profileEntries(list).map((item) => [item.id, item.name]),
      );
      label.firstChild.textContent =
        side === "sender"
          ? type === "wallet"
            ? "Wallet"
            : "Wallet group"
          : type === "recipient"
            ? "Recipient"
            : "Recipient group";
    }
  }
  const amountHints = {
    "": "Matches any amount.",
    count: "Inclusive upper bound for each transfer on its own.",
    interval:
      "Inclusive upper bound for the source wallet's pending, sent and confirmed payments in the rolling interval, including this one.",
  };
  function updateAmountFields() {
    const fields = query("#policy-form").elements;
    const type = fields.amount_type.value;
    query("#amount-fields").hidden = type === "";
    query("#interval-label").hidden = type !== "interval";
    fields.amount.required = type !== "";
    query("#amount-hint").textContent = amountHints[type];
  }
  function readAmountPolicy(fields) {
    const type = fields.amount_type.value;
    if (type === "") return null;
    const rule = { amount: fields.amount.value.trim(), amount_type: type };
    if (type === "interval") rule.interval = fields.interval.value;
    return rule;
  }
  query("#new-policy").addEventListener("click", openCreatePolicyDialog);
  query("#policy-form").addEventListener("change", (event) => {
    if (["sender_type", "receiver_type"].includes(event.target.name))
      updateCounterpartyFields();
    if (event.target.name === "amount_type") updateAmountFields();
  });
  query("#policy-form").addEventListener("submit", async (event) => {
    event.preventDefault();
    const button = event.submitter;
    button.disabled = true;
    query("#policy-error").hidden = true;
    const fields = event.target.elements;
    const policy = {
      profile_id: profile,
      name: fields.name.value,
      initiator_id: fields.initiator_id.value,
      sender_type: fields.sender_type.value,
      sender_id:
        fields.sender_type.value === "any" ? "" : fields.sender_id.value,
      receiver_type: fields.receiver_type.value,
      receiver_id:
        fields.receiver_type.value === "any" ? "" : fields.receiver_id.value,
      amount_policy: readAmountPolicy(fields),
      currency: fields.currency.value,
      action: fields.action.value,
    };
    try {
      await api.createPolicy(policy);
      query("#policy-dialog").close();
      await refresh();
      toast("Policy created at the bottom of the list");
    } catch (error) {
      showError("#policy-error", error);
    } finally {
      button.disabled = false;
    }
  });
  query("#policies-list").addEventListener("click", async (event) => {
    const button = event.target.closest("button");
    if (!button || button.disabled) return;
    if (button.dataset.delete) {
      const id = button.dataset.delete;
      const policy = state.policies.find((item) => item.id === id);
      const originalProfile = profile;
      confirmAction(
        "Delete policy?",
        `Delete “${policy.name}”? Existing transfers retain their recorded decisions.`,
        async () => {
          await api.deletePolicy(originalProfile, id);
          await refresh();
          toast("Policy deleted");
        },
      );
    } else if (button.dataset.move !== undefined) {
      if (orderBusy) return;
      orderBusy = true;
      document.querySelectorAll("[data-move]").forEach((control) => {
        control.disabled = true;
      });
      const index = Number(button.dataset.move),
        next = index + Number(button.dataset.direction);
      const ids = state.policies.map((policy) => policy.id);
      [ids[index], ids[next]] = [ids[next], ids[index]];
      try {
        await api.reorderPolicies(profile, ids);
        await refresh();
      } catch (error) {
        toast(error.message);
        await refresh().catch(() => {});
      } finally {
        orderBusy = false;
        renderPolicyList();
      }
    }
  });
  return {
    render() {
      synchronize();
      renderPolicyList();
    },
  };
}
