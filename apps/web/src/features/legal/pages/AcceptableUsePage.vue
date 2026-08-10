<template>
  <PublicLegalPage
    title="Acceptable Use & Copyright Policy"
    :version="LEGAL_DOCUMENT_VERSIONS.acceptableUse"
  >
    <p>
      This policy describes the content and activity that are not allowed on IdeaBoard. It applies
      to private Boards, shared Boards, prompts, uploaded media, and generated output.
    </p>

    <h2>1. Prohibited Content and Activity</h2>
    <ul>
      <li>Child sexual exploitation material or sexual content involving minors.</li>
      <li>Explicit adult content, non-consensual intimate imagery, or sexual deepfakes.</li>
      <li>Harassment, threats, hate, severe violence, fraud, phishing, or impersonation.</li>
      <li>
        Malicious automation or attempts to bypass access, payment, rate-limit, or security
        controls.
      </li>
      <li>
        Content that violates copyright, trademark, privacy, publicity, or other third-party rights.
      </li>
    </ul>

    <h2>2. Reports and Enforcement</h2>
    <p>
      A report should include your identity and contact details, the protected work or right at
      issue, the exact Board or share URL, a good-faith statement, an accuracy statement, and an
      electronic signature. We may restrict access while we investigate, remove content, suspend
      accounts, or take action against repeat infringers. Where legally and operationally
      appropriate, we provide notice and an opportunity to appeal.
    </p>

    <h2>3. Copyright Contact</h2>
    <p>
      Send copyright and content reports to
      <a :href="`mailto:${LEGAL_CONTACTS.copyright}`">{{ LEGAL_CONTACTS.copyright }}</a
      >. Do not label a report as a DMCA notice unless the applicable legal requirements have been
      satisfied.
    </p>

    <h2>4. False Reports</h2>
    <p>
      Knowingly false, abusive, or fraudulent reports may result in account restrictions and may
      create liability under applicable law.
    </p>
  </PublicLegalPage>
</template>

<script setup lang="ts">
import PublicLegalPage from '../components/PublicLegalPage.vue'
import { LEGAL_CONTACTS, LEGAL_DOCUMENT_VERSIONS } from '../legalDocuments'
</script>
