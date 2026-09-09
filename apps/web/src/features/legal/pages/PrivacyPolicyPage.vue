<template>
  <PublicLegalPage title="Privacy Policy" :version="LEGAL_DOCUMENT_VERSIONS.privacy">
    <p>
      IdeaBoard takes your privacy seriously. This policy explains how we collect, use, store, and
      protect your personal information.
    </p>

    <h2>1. Information We Collect</h2>
    <p>
      When you use IdeaBoard, we may collect the following information that is necessary to provide
      the service:
    </p>
    <ul>
      <li>
        <strong>Account information</strong>: Your email address, display name, avatar, and
        necessary identity details returned by an OAuth provider, such as Google, when you register
        or sign in.
      </li>
      <li>
        <strong>Usage and consumption data</strong>: Transaction status, Idea credit usage records,
        canvas activity, and generation history.
      </li>
      <li>
        <strong>Device and technical logs</strong>: IP address, browser type, device model, and
        security login logs used for alerts and suspicious-activity prevention.
      </li>
    </ul>

    <h2>2. How We Use Information</h2>
    <p>We use collected information only to:</p>
    <ul>
      <li>
        Provide, maintain, and improve the IdeaBoard infinite canvas and AI ideation services.
      </li>
      <li>Process recharge orders, reconcile payments, and provide support and refunds.</li>
      <li>Send security notices, verification codes, and necessary account-service updates.</li>
      <li>Prevent fraud, unauthorized payment activity, and threats to platform security.</li>
    </ul>

    <h2>3. Sharing and Third-Party Services</h2>
    <p>
      We do not sell your personal information. We share only the data necessary for the following
      services with trusted providers:
    </p>
    <ul>
      <li>
        <strong>Payment processor (Stripe)</strong>: Processes order payments and settlement. We do
        not store your sensitive card details on our servers.
      </li>
      <li>
        <strong>Email provider (Resend)</strong>: Delivers verification codes and service
        notifications.
      </li>
      <li>
        <strong>Infrastructure provider (Cloudflare)</strong>: Provides network proxying, page
        hosting, or security protection when those services are enabled.
      </li>
    </ul>

    <h2>4. Data Security and Storage</h2>
    <p>
      We protect your data with access controls, encrypted storage, and HTTPS/TLS encryption in
      transit. Sensitive credentials are stored as ciphertext according to our server-side
      key-management practices.
    </p>

    <h2>5. Your Rights</h2>
    <p>
      You may review or update your account information and request account deletion. After
      deletion, identity data will be disabled, deleted, or anonymized according to the deletion
      process. Records needed for payment settlement, accounting, refunds, disputes, fraud
      prevention, or security audits may be retained when required by law or legitimate business
      obligations.
    </p>

    <h2>6. Contact Us</h2>
    <p>
      For privacy and data-rights requests, contact
      <a :href="`mailto:${LEGAL_CONTACTS.privacy}`">{{ LEGAL_CONTACTS.privacy }}</a
      >. For general support, contact
      <a :href="`mailto:${LEGAL_CONTACTS.support}`">{{ LEGAL_CONTACTS.support }}</a
      >.
    </p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'
import { LEGAL_CONTACTS, LEGAL_DOCUMENT_VERSIONS } from '../documents'
</script>
