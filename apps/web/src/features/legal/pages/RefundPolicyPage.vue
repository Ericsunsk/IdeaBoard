<template>
  <PublicLegalPage title="Refund Policy" :version="LEGAL_DOCUMENT_VERSIONS.refund">
    <p>This policy applies to Ideas purchased through IdeaBoard Checkout.</p>

    <h2>1. Purchase Policy</h2>
    <p>
      Purchases of Ideas are non-refundable, except where required by applicable law or expressly
      promised at the time of purchase. Unused Ideas do not by themselves entitle you to a refund.
      Contact support if you believe an exception applies; self-service refunds are not available.
    </p>

    <h2>2. Organization Closure</h2>
    <p>
      Closing an Organization is permanent. Its remaining Ideas become permanently unavailable,
      cannot be restored, and are not transferred to a Personal account. Organization closure does
      not automatically trigger a refund. Exceptions required by applicable law or expressly
      promised at the time of purchase still apply.
    </p>

    <h2>3. Approved Exceptions</h2>
    <p>
      For an approved exception, the refund amount and eligible quantity are determined by the
      applicable legal requirement or purchase promise and the original payment facts. We do not
      refund the same payment twice. Taxes, fees, and currency adjustments are handled according to
      the applicable requirement and the original transaction.
    </p>

    <h2>4. Processing</h2>
    <p>
      Approved refunds are processed through the original payment method. Completion and the time it
      takes for funds to arrive depend on the payment provider and the payment institution. Nothing
      in this policy limits mandatory consumer rights.
    </p>

    <h2>5. Payment Disputes</h2>
    <p>
      If you notice an unauthorized payment, contact us as soon as possible. Filing a Dispute
      directly with your card issuer may trigger payment and usage restrictions on your account
      until the dispute is resolved.
    </p>

    <h2>6. Contact Us</h2>
    <p>
      When contacting us, provide your account information and the transaction time shown in your
      recharge history. Do not send your card number, verification code, or other sensitive payment
      information. Contact
      <a :href="`mailto:${LEGAL_CONTACTS.support}`">{{ LEGAL_CONTACTS.support }}</a> for support.
    </p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'
import { LEGAL_CONTACTS, LEGAL_DOCUMENT_VERSIONS } from '../documents'
</script>
