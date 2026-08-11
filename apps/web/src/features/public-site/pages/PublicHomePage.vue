<template>
  <div class="public-home min-h-dvh overflow-y-auto bg-ib-surface-muted text-ib-text">
    <header class="public-home__nav">
      <RouterLink class="public-home__brand" to="/" aria-label="IdeaBoard home"
        >IdeaBoard</RouterLink
      >
      <RouterLink class="public-home__nav-link" :to="USER_HOME_PATH">Open workspace</RouterLink>
    </header>

    <main>
      <section class="public-home__hero" aria-labelledby="public-home-heading">
        <div class="public-home__hero-copy">
          <p class="public-home__eyebrow">AI Canvas for Visual Designers</p>
          <h1 id="public-home-heading">Shape visual ideas before they become finished work.</h1>
          <p class="public-home__lede">
            Explore, compare, and refine image directions in one connected workspace built for
            Visual Designers and Pattern Designers.
          </p>
          <RouterLink class="public-home__cta" :to="USER_HOME_PATH">Open the workspace</RouterLink>
          <p class="public-home__note">
            No setup tour required. Start with a direction and keep the thread visible.
          </p>
        </div>
        <div class="public-home__canvas" aria-label="Connected AI canvas preview" role="img">
          <div class="public-home__canvas-grid"></div>
          <div class="public-home__node public-home__node--prompt">
            <span>Direction</span><strong>soft geometry</strong>
          </div>
          <div class="public-home__node public-home__node--reference">
            <span>References</span><strong>tone + texture</strong>
          </div>
          <div class="public-home__node public-home__node--result">
            <span>Next signal</span><strong>compare and refine</strong>
          </div>
          <div class="public-home__connector public-home__connector--one"></div>
          <div class="public-home__connector public-home__connector--two"></div>
        </div>
      </section>

      <section class="public-home__section" aria-labelledby="public-home-workflow-heading">
        <div>
          <p class="public-home__eyebrow">A connected way to think</p>
          <h2 id="public-home-workflow-heading">Keep the visual thread intact.</h2>
        </div>
        <div class="public-home__steps">
          <article>
            <span>01</span>
            <h3>Set a direction</h3>
            <p>Bring a prompt, sketch, or reference image into the same canvas.</p>
          </article>
          <article>
            <span>02</span>
            <h3>Compare the paths</h3>
            <p>Keep visual choices together so the strongest signal is easy to see.</p>
          </article>
          <article>
            <span>03</span>
            <h3>Refine what matters</h3>
            <p>Iterate on composition, style, and detail without losing context.</p>
          </article>
        </div>
      </section>

      <section class="public-home__facts" aria-labelledby="public-home-facts-heading">
        <div>
          <p class="public-home__eyebrow">Made for visual exploration</p>
          <h2 id="public-home-facts-heading">
            A practical starting point for pattern and image ideas.
          </h2>
        </div>
        <dl>
          <div>
            <dt>Inputs</dt>
            <dd>{{ PUBLIC_HOME_FACT.inputs.join(', ') }}</dd>
          </div>
          <div>
            <dt>Outputs</dt>
            <dd>{{ PUBLIC_HOME_FACT.outputs.join(', ') }}</dd>
          </div>
          <div>
            <dt>Scope</dt>
            <dd>{{ PUBLIC_HOME_FACT.limits[0] }}</dd>
          </div>
        </dl>
      </section>

      <section class="public-home__activation" aria-labelledby="public-home-activation-heading">
        <div>
          <p class="public-home__eyebrow">Workspace Activation</p>
          <h2 id="public-home-activation-heading">Give the next idea somewhere to move.</h2>
          <p>Open a working Canvas Board and keep exploring from there.</p>
        </div>
        <RouterLink class="public-home__cta public-home__cta--dark" :to="USER_HOME_PATH"
          >Open the workspace</RouterLink
        >
      </section>
    </main>

    <footer class="public-home__footer">
      <span>IdeaBoard AI Canvas</span>
      <nav aria-label="Legal">
        <RouterLink v-for="link in PUBLIC_LEGAL_LINKS" :key="link.to" :to="link.to">{{
          link.shortLabel
        }}</RouterLink>
      </nav>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { PUBLIC_LEGAL_LINKS } from '../../../shared/constants/legalLinks'
import { USER_HOME_PATH } from '../../../shared/constants/routePaths'
import { PUBLIC_HOME_FACT } from '../publicFactRegistry'
</script>

<style scoped>
.public-home {
  --public-home-ink: #1d2939;
  --public-home-muted: #667085;
  --public-home-border: #d9e2ef;
  --public-home-soft: #f5f8fb;
  font-family:
    -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
}

.public-home__nav,
.public-home__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  width: min(1180px, calc(100% - 48px));
  margin: 0 auto;
}

.public-home__nav {
  padding: 24px 0;
}
.public-home__brand {
  color: var(--public-home-ink);
  font-size: 18px;
  font-weight: 760;
  letter-spacing: -0.01em;
}
.public-home__nav-link {
  color: var(--public-home-muted);
  font-size: 14px;
  font-weight: 650;
}
.public-home__nav-link:hover,
.public-home__footer a:hover {
  color: var(--public-home-ink);
}
.public-home__hero {
  display: grid;
  grid-template-columns: minmax(0, 0.95fr) minmax(420px, 1.05fr);
  align-items: center;
  gap: clamp(32px, 6vw, 92px);
  width: min(1180px, calc(100% - 48px));
  min-height: 660px;
  margin: 0 auto;
  padding: 64px 0 108px;
}
.public-home__hero-copy {
  max-width: 560px;
}
.public-home__eyebrow {
  margin: 0 0 18px;
  color: var(--color-ib-creative-accent-pressed);
  font-size: 12px;
  font-weight: 760;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.public-home h1,
.public-home h2,
.public-home h3,
.public-home p {
  margin-top: 0;
}
.public-home h1 {
  max-width: 640px;
  margin-bottom: 22px;
  color: var(--public-home-ink);
  font-size: clamp(42px, 5.4vw, 76px);
  line-height: 0.98;
  letter-spacing: -0.035em;
}
.public-home h2 {
  max-width: 580px;
  margin-bottom: 0;
  color: var(--public-home-ink);
  font-size: clamp(30px, 3.2vw, 48px);
  line-height: 1.05;
  letter-spacing: -0.025em;
}
.public-home h3 {
  margin-bottom: 8px;
  color: var(--public-home-ink);
  font-size: 17px;
}
.public-home__lede {
  max-width: 510px;
  margin-bottom: 28px;
  color: var(--public-home-muted);
  font-size: 18px;
  line-height: 1.6;
}
.public-home__note {
  margin: 14px 0 0;
  color: var(--public-home-muted);
  font-size: 12px;
}
.public-home__cta {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 46px;
  padding: 0 22px;
  border: 1px solid var(--color-ib-creative-accent-pressed);
  border-radius: var(--radius-ib-control);
  background: var(--color-ib-creative-accent);
  color: white;
  font-size: 14px;
  font-weight: 730;
  box-shadow: 0 8px 20px rgba(109, 95, 208, 0.18);
  transition:
    transform var(--motion-control-duration) var(--motion-control-easing),
    background-color var(--motion-control-duration) var(--motion-control-easing);
}
.public-home__cta:hover {
  transform: translateY(-1px);
  background: var(--color-ib-creative-accent-hover);
}
.public-home__cta:focus-visible,
.public-home a:focus-visible {
  outline: 2px solid var(--color-ib-creative-accent);
  outline-offset: 3px;
}
.public-home__cta--dark {
  background: var(--public-home-ink);
  border-color: var(--public-home-ink);
  box-shadow: none;
}
.public-home__cta--dark:hover {
  background: #26364f;
}
.public-home__canvas {
  position: relative;
  min-height: 460px;
  overflow: hidden;
  border: 1px solid var(--public-home-border);
  border-radius: 22px;
  background: #eef2f8;
  box-shadow: 0 24px 64px rgba(38, 54, 79, 0.1);
}
.public-home__canvas-grid {
  position: absolute;
  inset: 0;
  background: rgba(226, 232, 240, 0.42);
}
.public-home__canvas::before {
  position: absolute;
  inset: 24px;
  border: 1px solid rgba(148, 162, 182, 0.45);
  border-radius: 14px;
  content: '';
}
.public-home__node {
  position: absolute;
  display: flex;
  flex-direction: column;
  gap: 5px;
  width: 170px;
  padding: 16px;
  border: 1px solid var(--public-home-border);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.94);
  box-shadow: 0 12px 26px rgba(38, 54, 79, 0.1);
}
.public-home__node span {
  color: var(--public-home-muted);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.public-home__node strong {
  color: var(--public-home-ink);
  font-size: 16px;
}
.public-home__node--prompt {
  top: 18%;
  left: 9%;
}
.public-home__node--reference {
  top: 52%;
  left: 28%;
}
.public-home__node--result {
  right: 8%;
  bottom: 15%;
}
.public-home__connector {
  position: absolute;
  height: 2px;
  transform-origin: left center;
  background: var(--color-ib-creative-accent);
  opacity: 0.65;
}
.public-home__connector--one {
  top: 38%;
  left: 30%;
  width: 28%;
  transform: rotate(28deg);
}
.public-home__connector--two {
  top: 63%;
  left: 52%;
  width: 28%;
  transform: rotate(-14deg);
}
.public-home__section,
.public-home__facts,
.public-home__activation {
  width: min(1180px, calc(100% - 48px));
  margin: 0 auto;
}
.public-home__section {
  display: grid;
  grid-template-columns: minmax(240px, 0.65fr) minmax(0, 1.35fr);
  gap: 72px;
  padding: 112px 0;
  border-top: 1px solid var(--public-home-border);
}
.public-home__steps {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 24px;
}
.public-home__steps article {
  padding-top: 4px;
}
.public-home__steps span {
  display: block;
  margin-bottom: 32px;
  color: var(--color-ib-creative-accent-pressed);
  font-size: 12px;
  font-weight: 760;
  letter-spacing: 0.1em;
}
.public-home__steps p,
.public-home__activation p {
  margin-bottom: 0;
  color: var(--public-home-muted);
  font-size: 14px;
  line-height: 1.6;
}
.public-home__facts {
  display: grid;
  grid-template-columns: minmax(240px, 0.85fr) minmax(0, 1.15fr);
  gap: 72px;
  padding: 112px 0;
  border-top: 1px solid var(--public-home-border);
}
.public-home__facts dl {
  display: grid;
  gap: 0;
  margin: 0;
}
.public-home__facts dl > div {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr);
  gap: 16px;
  padding: 18px 0;
  border-bottom: 1px solid var(--public-home-border);
}
.public-home__facts dt {
  color: var(--public-home-ink);
  font-size: 13px;
  font-weight: 740;
}
.public-home__facts dd {
  margin: 0;
  color: var(--public-home-muted);
  font-size: 14px;
  line-height: 1.55;
}
.public-home__activation {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 32px;
  padding: 64px 0 104px;
  border-top: 1px solid var(--public-home-border);
}
.public-home__footer {
  padding: 22px 0 34px;
  border-top: 1px solid var(--public-home-border);
  color: var(--public-home-muted);
  font-size: 12px;
}
.public-home__footer nav {
  display: flex;
  flex-wrap: wrap;
  gap: 14px;
}

@media (max-width: 840px) {
  .public-home__hero,
  .public-home__section,
  .public-home__facts {
    grid-template-columns: 1fr;
    gap: 44px;
  }
  .public-home__hero {
    padding-top: 42px;
  }
  .public-home__canvas {
    min-height: 390px;
  }
}

@media (max-width: 620px) {
  .public-home__nav,
  .public-home__footer,
  .public-home__hero,
  .public-home__section,
  .public-home__facts,
  .public-home__activation {
    width: min(100% - 32px, 520px);
  }
  .public-home__steps {
    grid-template-columns: 1fr;
    gap: 28px;
  }
  .public-home__steps span {
    margin-bottom: 10px;
  }
  .public-home__activation {
    align-items: flex-start;
    flex-direction: column;
  }
  .public-home__canvas {
    min-height: 330px;
  }
  .public-home__node {
    width: 140px;
    padding: 12px;
  }
  .public-home__node strong {
    font-size: 13px;
  }
}
</style>
