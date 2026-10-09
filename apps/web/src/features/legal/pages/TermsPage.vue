<template>
  <PublicLegalPage title="Terms of Service" :version="LEGAL_DOCUMENT_VERSIONS.terms">
    <p>
      These Terms of Service govern your access to and use of IdeaBoard. IdeaBoard is operated by
      Shikang Sun under the trade name Eric Studio. By accessing or using IdeaBoard, you agree to
      these Terms. Please read them before purchasing or using Ideas.
    </p>

    <h2>1. Eligibility, Service, and Accounts</h2>
    <p>
      You must be at least 18 years old. You are responsible for keeping your account secure and for
      activity performed through it. Boards are private by default. An owner may enable an unlisted
      share link; anyone with that link may view the shared content, but editing requires an
      authenticated invitation. You may not use the service to violate another person's rights,
      bypass security measures, or engage in unlawful activity.
    </p>

    <h2>2. Ideas and Purchases</h2>
    <p>
      Ideas are virtual credits used only for features within IdeaBoard. They are not currency,
      deposits, or transferable assets. The price shown at Checkout controls the purchase price. The
      checkout page displays the payment method, currency, and required billing information
      according to the current configuration. Automatic tax calculation is not currently enabled for
      recharges, so taxes are not automatically calculated for this service.
    </p>

    <h2>3. Payment and Credit Delivery</h2>
    <p>
      Payments are processed by our payment provider. Returning to IdeaBoard in your browser does
      not mean that payment succeeded. Ideas are credited only after the system confirms the
      payment.
    </p>

    <h2>4. User Content and AI Content</h2>
    <p>
      You retain your rights in your inputs and outputs to the extent provided by applicable law.
      You grant IdeaBoard only the limited license needed to host, process, transmit, render, and
      share content at your direction. User content is not sold or used to train AI models unless a
      future separate opt-in explicitly says otherwise. AI output may be inaccurate or unsuitable
      for a particular purpose. You must review output before using, publishing, or relying on it,
      and ensure that your inputs and use comply with applicable law and third-party rights.
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
      For account and purchase support, contact
      <a :href="`mailto:${LEGAL_CONTACTS.support}`">{{ LEGAL_CONTACTS.support }}</a
      >. Legal notices may be sent to
      <a :href="`mailto:${LEGAL_CONTACTS.legal}`">{{ LEGAL_CONTACTS.legal }}</a
      >.
    </p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'
import { LEGAL_CONTACTS, LEGAL_DOCUMENT_VERSIONS } from '../documents'
</script>
