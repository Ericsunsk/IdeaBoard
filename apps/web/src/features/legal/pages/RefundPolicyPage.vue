<template>
  <PublicLegalPage title="Refund Policy" :version="LEGAL_DOCUMENT_VERSIONS.refund">
    <p>This policy applies to Ideas purchased through IdeaBoard Stripe Checkout.</p>

    <h2>1. Request Window</h2>
    <p>
      Refund eligibility lasts for 14 days from the time Stripe confirms payment. Self-service
      refunds are not currently available; contact the administrator within this period.
    </p>

    <h2>2. Eligible Quantity</h2>
    <p>
      The refundable quantity is the smaller of the amount not already refunded for the order and
      the Ideas currently available in your account. Ideas that have already been consumed cannot be
      refunded from that order.
    </p>

    <h2>3. Refund Amount</h2>
    <p>
      The product refund amount is calculated in the smallest unit of the original payment currency,
      in proportion to the Ideas in the original order. Any taxes or other adjustments handled by
      the payment provider are included according to the original Stripe transaction.
    </p>

    <h2>4. Processing</h2>
    <p>
      After reviewing a request, the administrator processes the refund through the corresponding
      original Stripe payment. Completion and the time it takes for funds to arrive are determined
      by Stripe and the payment institution.
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
import { LEGAL_CONTACTS, LEGAL_DOCUMENT_VERSIONS } from '../legalDocuments'
</script>
