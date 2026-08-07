<template>
  <PublicLegalPage title="Terms of Service" :version="TERMS_VERSION">
    <p>
      By using IdeaBoard, you agree to these Terms of Service. Please read them before purchasing or
      using Ideas.
    </p>

    <h2>1. Service and Accounts</h2>
    <p>
      You are responsible for keeping your account secure and for activity performed through it. You
      may not use the service to violate another person's rights, bypass security measures, or
      engage in unlawful activity.
    </p>

    <h2>2. Ideas and Purchases</h2>
    <p>
      Ideas are virtual credits used only for features within IdeaBoard. They are not currency,
      deposits, or transferable assets. The price shown at Checkout controls the purchase price.
      Stripe Checkout displays the payment method, currency, and required billing information
      according to the current configuration. Stripe Automatic Tax is not currently enabled for
      recharges, so Stripe does not automatically calculate taxes for this service.
    </p>

    <h2>3. Payment and Credit Delivery</h2>
    <p>
      Payments are processed by Stripe. Returning to IdeaBoard in your browser does not mean that
      payment succeeded. Ideas are credited only after the system confirms the Stripe payment.
    </p>

    <h2>4. AI Content</h2>
    <p>
      AI output may be inaccurate or unsuitable for a particular purpose. You must review output
      before using, publishing, or relying on it, and ensure that your inputs and use comply with
      applicable law and third-party rights.
    </p>

    <h2>5. Service Restrictions</h2>
    <p>
      We may restrict account activity to protect security, handle payment disputes, or comply with
      legal requirements. Scheduled maintenance or events beyond our control may cause temporary
      interruptions.
    </p>

    <h2>6. Changes to These Terms</h2>
    <p>
      We will publish a new version when these Terms change. We record the version you accepted for
      each purchase; an update does not alter historical order facts.
    </p>

    <h2>7. Contact Us</h2>
    <p>
      If you have questions about your account, a purchase, or these Terms, contact the
      administrator through the in-product support channel.
    </p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'

const TERMS_VERSION = '2026-08-01'
</script>
