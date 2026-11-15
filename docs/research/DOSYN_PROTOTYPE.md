**DOSYN — what’s in the prototype and what we still need to build**

DOSYN connects patients with pharmacies and lets pharmacies ask other branches about medicines they can’t supply. The prototype shows the full enquiry from a reviewed prescription to a pharmacist’s reply.

**What’s in the prototype**

- **Entrance and accounts.** The homepage has a floating DOSYN capsule and two entry paths. Clients can log in with a demo profile or sign up. Pharmacists enter their own pharmacy name and town and see **Pending verification**. A separate pre-approved demo branch lets us test the pharmacy workspace.
- **Client pharmacy.** **My medicines** shows only items marked prescribed and paid. **Prescriptions** lets clients check availability before paying. The requested quantity comes from the reviewed sample prescription and is read-only. If the quantity needs review, the enquiry stops there.
- **Pharmacy finder and replies.** Clients can choose a pharmacy from a list or an interactive demo map. Opening a medicine shows branch-by-branch responses. Confirmations include the exact medicine, quantity, branch, and time checked. Clients can also see their enquiry history.
- **Documents.** Each client gets a personal folder and can create case folders, add sample PDFs or images, and preview them in the current tab.
- **Pharmacy office and stockroom.** The business side has a response queue, client and network enquiries, approvals, notifications, and searchable inventory grouped into condition folders. Pharmacists can edit stock counts in the demo and see **In stock**, **Out of stock**, or **Unknown**.
- **AI handoff.** The demo agent matches and routes an exact medicine enquiry, tracks progress, and records the audit trail. If a branch has zero stock, its pharmacist can select **Ask the network** to send a linked enquiry with the same medicine and quantity. The receiving pharmacist confirms availability. A timeout is shown as **Unable to confirm**.
- **Design and access.** The client side looks like a pharmacy front, while the business side has a vault-inspired office. Both have light and dark modes. English, Arabic, and French are selectable, with right-to-left layout for Arabic. Some text still falls back to English.

**What we need to decide about inventory**

The current demo has manual stock editing. For the real product, we should first ask a pilot pharmacy how it tracks stock and whether it can export it. If it can, the proposed workflow is **Import stock → Review uncertain matches → Apply updates**. AI can suggest matches for messy medicine names. The pharmacy supplies the counts, and staff can correct them. Manual entry is the fallback if there’s no export. The agent can use recorded stock to help route enquiries, and the receiving pharmacist confirms the reply.

**What’s still simulated**

Accounts, pharmacy approval, prescriptions, stock, and agent activity use demo data. We still need real authentication and branch authorization, persistent inventory and request records, private document storage, and integrations for prescription transcription, payment status, and notifications.

**First end-to-end milestone:** Verify one pilot pharmacy, load its stock, send it a client enquiry using the quantity from a reviewed prescription, and show the pharmacist’s timestamped response in the client view.
