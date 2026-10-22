# IdeaBoard Legal Compliance Spec

## 1. Status

- Status: Draft for legal review
- Product: IdeaBoard, operated under the unregistered trade name Eric Studio
- Market: Globally accessible B2C SaaS
- Public document language: English
- Canonical source: Repository-managed page source

This document defines product and engineering requirements. It is not legal advice. Before enabling global paid access, the operator must obtain professional review of the final public text, Stripe account eligibility, cross-border tax obligations, and consumer-law requirements.

## 2. Operator Facts

- The service is operated by an individual located in mainland China.
- Eric Studio and IdeaBoard are unregistered trade names, not companies or separate legal entities.
- The Hong Kong Stripe account is registered to the same individual.
- Public legal documents must identify the operator by legal name and must not imply that Eric Studio or IdeaBoard is an incorporated entity.
- Public documents must provide role-based support, legal, privacy, and copyright contact details. A private residential address must not be published unless legally required; use a lawful business correspondence address where available.

Required before publication:

- Operator legal name: `Shikang Sun`
- Customer support email: `support@ideaboard.art`
- Legal notices email: `legal@ideaboard.art`
- Privacy and data-rights email: `privacy@ideaboard.art`
- Copyright report email: `copyright@ideaboard.art`
- Governing law and dispute forum: `TBD after professional review`

## 3. Product Decisions

- Users must be at least 18 years old.
- Boards are private by default.
- An owner may explicitly enable an unlisted share link.
- A non-registered link holder may view but may not edit, comment, or manage members.
- Edit access requires an authenticated, explicitly invited collaborator.
- User content is not sold or used to train AI models, whether private or shared, unless a future separate opt-in explicitly says otherwise.
- Only transactional email is sent. Marketing email remains unavailable until separate consent and unsubscribe capabilities exist.
- Optional Analytics/RUM remains disabled. Strictly necessary authentication, security, error, and operational logs remain allowed.
- AI outputs may be used commercially subject to applicable law, Provider terms, and third-party rights. IdeaBoard does not guarantee copyrightability, originality, non-infringement, accuracy, or fitness for purpose.
- NSFW/adult content and the prohibited uses defined below are not allowed.
- Purchased Ideas have no scheduled expiry, are non-transferable, cannot be sold or redeemed for cash, and are usable only within IdeaBoard. Permanent Organization closure makes its remaining Ideas permanently unavailable.
- Purchases are non-refundable except where required by applicable law or expressly promised at purchase. Organization closure does not automatically trigger a refund, restore Ideas, or transfer them to Personal; the same exceptions still apply.

## 4. Required Public Pages

IdeaBoard maintains four public legal pages. Do not split the listed content into additional pages until the product adds subscriptions, enterprise contracts, a Public API, optional tracking, or another capability that requires a distinct contract.

### 4.1 Terms of Service

The Terms must cover:

- The individual operator and the status of Eric Studio and IdeaBoard as trade names.
- Eligibility, the 18+ requirement, account security, suspension, and termination.
- Private Boards, share-link visibility, collaborator responsibility, and revocation.
- User ownership of inputs and the limited license required to host, process, transmit, render, and share content at the user's direction.
- AI Provider processing, commercial-use permission, output limitations, and third-party rights.
- Ideas as non-cash, non-transferable service credits with no scheduled expiry; permanent Organization closure ends access to its remaining Ideas.
- Stripe payment processing, displayed currency, price, taxes, successful payment confirmation, and credit delivery.
- Service availability, maintenance, feature and price changes, warranty disclaimers, liability limitations, and indemnity, subject to mandatory consumer law.
- Governing law, disputes, legal notices, changes, and contact information after professional review.

### 4.2 Privacy Policy

The Privacy Policy must cover:

- Account, OAuth, profile, Board, Prompt, uploaded media, generated output, usage, payment metadata, support, security, and technical-log data.
- Processing purposes and applicable legal bases for contract performance, legitimate interests, consent where required, and legal obligations.
- No sale of personal information and no training on user content, whether private or shared, unless a future separate opt-in explicitly says otherwise.
- Third-party recipients, including Stripe, Google/Discord OAuth, Resend, Cloudflare, configured AI Providers, and configured Storage Providers.
- For each recipient category: purpose, data category, transfer/hosting region where known, and a link to the Provider policy.
- The published Provider inventory is a controlled release artifact. Adding or changing an AI, Storage, Analytics, Auth, Email, or Payment Provider requires updating Privacy Policy content and its effective date before the Provider is enabled for users.
- Cross-border processing and transfer safeguards after professional review.
- Retention rules, including deletion of account content and continued retention of the minimum payment, refund, Dispute, fraud, tax, accounting, and security records required by law or legitimate obligations.
- User rights, including access, correction, export, deletion, objection, restriction, withdrawal of consent where applicable, and complaint channels.
- Backup lifecycle limitations: deleted data may remain in protected backups until normal overwrite or expiry.
- Strictly necessary cookies and the absence of optional Analytics/RUM. A separate Cookie Policy and consent manager are required before optional tracking is enabled.
- Regional supplements for EU/EEA, UK, California, and other applicable jurisdictions after professional review.

### 4.3 Refund Policy

The Refund Policy must cover:

- Purchases are non-refundable except where required by law or expressly promised at purchase; there is no general refund window or automatic refund entitlement.
- Approved exceptions are reviewed against the applicable legal requirement or purchase promise and the original payment facts, without duplicate refunds.
- Admin exception refunds use the persisted payer account and original order, reserve available Ideas before provider I/O, enforce cumulative payment caps, and settle idempotently. These technical limits do not override mandatory consumer rights; any shortfall requires an audited support resolution. Partial refunds define minor-unit rounding and tax/fee/FX treatment.
- Permanent Organization closure makes its remaining Ideas unavailable, cannot be reversed, never transfers funds to Personal, and does not automatically trigger a refund; legal requirements and purchase promises still apply.
- Treatment of consumed Ideas, failed tasks, duplicate charges, unauthorized charges, delayed credit delivery, taxes, fees, and exchange-rate differences.
- Refunds to the original payment method only.
- Expected review and Provider processing timelines without guaranteeing bank settlement time.
- Temporary account or Idea restrictions needed to prevent double spending during refund or Dispute resolution.
- Mandatory local consumer rights, including any required consent for immediate performance of digital services and its effect on withdrawal rights.
- Safe support instructions that never request card numbers, verification codes, or full payment credentials.

### 4.4 Acceptable Use & Copyright Policy

The combined policy must prohibit:

- Child sexual exploitation material and any sexual content involving minors.
- Explicit adult content, non-consensual intimate imagery, and sexual deepfakes.
- Harassment, threats, hate, severe violence, unlawful content, fraud, phishing, impersonation, and malicious automation.
- Unauthorized use of copyrighted works, trademarks, privacy rights, publicity rights, or personal likenesses.
- Attempts to bypass access controls, payment controls, rate limits, moderation, or security protections.

The policy must define:

- Content-report requirements: reporter identity, contact details, protected work or violated right, exact URL/share link, good-faith statement, accuracy statement, and electronic signature.
- Temporary restriction, investigation, removal, account action, and repeat-infringer handling.
- User notice and counter-report/appeal where legally and operationally appropriate.
- Consequences for knowingly false or abusive reports.
- A general copyright and content-reporting process. Do not label it a DMCA Policy unless the operator completes and maintains the required United States Designated Agent process.

## 5. Product Entry Points

- Public footer: Terms, Privacy, Refunds, Acceptable Use.
- Registration: direct links to Terms and Privacy next to the registration action.
- Checkout: direct links to Terms and Refund Policy, plus the actual currency, Ideas quantity, total price, and applicable tax display.
- First AI use: direct links to Acceptable Use and the AI sections of the Terms.
- Share settings: an explicit warning that anyone with the link can view, plus immediate revoke controls.
- Account settings: Export My Data and Delete Account.
- Privacy Policy: direct references to export/deletion controls and the privacy contact channel.

Legal links must remain available without authentication. They must not be hidden only in a modal or Admin surface.

## 6. Acceptance Contract

Keep acceptance versioning intentionally small:

- Public pages display an `Effective date`.
- The date is the document version, formatted as `YYYY-MM-DD`.
- Each acceptance payload contains only:
  - `document_type`
  - `accepted_version`
  - `accepted_at`
- The owning account or order record supplies the subject identity and business boundary; these legal fields do not replace the existing `user_id` or immutable Payment Order relation. Acceptance writes use server UTC time and are idempotent for the same subject, document, version, and boundary.
- Only one effective version per document may be published on a given UTC date. A material revision prepared on the same date must wait for the next effective date.
- Git history is the internal change record.
- Material changes require renewed acceptance at the next relevant boundary. Material changes include changes to the operator, data use, Provider disclosure, user-content license, AI training, payment/refund rights, prohibited uses, governing law, dispute terms, or liability allocation.
- Spelling, formatting, link, and other non-material corrections do not require renewed acceptance.

Required acceptance boundaries:

- Registration: Terms and Privacy.
- First AI use: Acceptable Use and current Terms AI provisions.
- Checkout: Terms and Refund Policy. The Payment Order retains the accepted versions as immutable order facts.

Do not add document hashes, an in-product historical-version browser, or a general-purpose legal CMS in the first implementation.

## 7. Functional Requirements

### 7.1 Sharing

- Share links are disabled by default.
- Tokens must be high entropy and revocable. Expiration is a product decision that must define a default, maximum lifetime, and behavior for already-open pages before expiration is implemented.
- Shared pages use `noindex, nofollow`.
- Anonymous access is read-only and enforced by the API, including media access.
- Revoking a token immediately removes anonymous access.

### 7.2 Data Rights

- Export includes the user's available account profile, Boards, Prompts, media references, and generation history in a documented portable format.
- Account deletion revokes sessions and share links, deletes or schedules deletion of Boards and media, and prevents future access.
- Payment, refund, Dispute, fraud, tax, accounting, and security records are retained only to the minimum extent and duration justified by applicable obligations.

### 7.3 Email And Tracking

- Transactional and marketing email code paths remain separate.
- Registration, purchase, or Terms acceptance must not be treated as marketing consent.
- Optional tracking code must not load until a consent design is approved and implemented.

### 7.4 Age Eligibility

- Registration states that the service is available only to users aged 18 or older.
- The first implementation does not collect birth dates or claim to perform formal identity-based age verification.
- A report that an account belongs to a minor triggers account restriction, identity-safe review, and deletion of the minor's content unless retention is legally required.

## 8. Launch Blockers

The following block global paid launch:

- Missing operator legal name or public contact channels.
- Final public copy has not received professional legal review.
- Stripe has not confirmed that the Hong Kong account setup is valid for the operator's residence, identity, and business activity.
- Cross-border tax, VAT/GST, invoice, and consumer-withdrawal obligations have not been reviewed.
- The actual tax display/calculation, invoice/receipt behavior, and any required digital-service withdrawal consent are not implemented and tested for the launch markets.
- Any of the four public legal routes is missing, unauthenticated, undated, or materially inconsistent with live behavior.
- Checkout does not show or retain the applicable Terms and Refund Policy versions.
- Registration and first AI use do not show and persist their required acceptance facts.
- Export and Delete Account are not available, or their retention/backup behavior is not documented.
- Anonymous share links expose edit access or bypass Board/media authorization.
- Optional Analytics/RUM is enabled without the required disclosure and consent behavior.

## 9. Explicit Non-Goals

The first implementation does not add:

- Billing Terms, Subscription Terms, DPA, SLA, Enterprise Terms, or API Terms.
- A standalone AI Notice, Subprocessors page, Cookie Policy, Data Retention page, or Privacy Rights page.
- A DMCA Policy, Law Enforcement Guidelines, or Transparency Report.
- A legal CMS, document hashes, or public historical-version UI.
- Marketing consent, Broadcast email, optional Analytics/RUM, or support for users under 18.

## 10. Acceptance Criteria

- Four public routes exist and are accessible without authentication.
- Every page identifies its effective date and the individual operator after the required identity fields are completed.
- Terms, Privacy, Refund, and Acceptable Use content match the actual product behavior and Provider configuration.
- Registration, first AI use, and Checkout expose the required links and store the required acceptance facts.
- Payment Orders retain immutable Terms and Refund Policy versions.
- Anonymous share links are opt-in, read-only, revocable, and non-indexable.
- Export and Delete Account are available from Account Settings.
- Optional Analytics/RUM and Marketing Email remain disabled.
- Automated tests cover route accessibility, acceptance payloads, share authorization, and payment legal-version persistence.
- A qualified professional reviews the final public text before global paid launch.

## 11. Reader-Safety Notes

- “Globally accessible” does not mean every jurisdiction is accepted without restriction. The operator must define blocked or unsupported regions if tax, sanctions, consumer, or transfer requirements cannot be met.
- English is the initial public document language. Where local law requires a local-language notice or grants mandatory rights, that law and notice take priority over an English-only presentation.
- A standalone DPA, Cookie Policy, or Subprocessors page remains out of scope only while the corresponding processing, tracking, and enterprise obligations are fully covered in Privacy Policy and optional tracking/enterprise features remain disabled.
