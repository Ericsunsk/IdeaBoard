import { PUBLIC_HOME_FACT } from './publicFactRegistry'
import { PUBLIC_HOME_METADATA, serializePublicHomeJsonLd } from './publicSiteMetadata'

const escapeHtml = (value: string) =>
  value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')

export const PUBLIC_HOME_STATIC_MARKUP = `
  <div class="public-home-static">
    <header class="public-home-static__nav">
      <a class="public-home-static__brand" href="/" aria-label="IdeaBoard home">IdeaBoard</a>
      <a class="public-home-static__nav-link" href="/user/home">Open workspace</a>
    </header>
    <main>
      <section class="public-home-static__hero" aria-labelledby="public-home-heading">
        <div class="public-home-static__hero-copy">
          <p class="public-home-static__eyebrow">AI Canvas for Visual Designers</p>
          <h1 id="public-home-heading">Shape visual ideas before they become finished work.</h1>
          <p class="public-home-static__lede">Explore, compare, and refine image directions in one connected workspace built for Visual Designers and Pattern Designers.</p>
          <a class="public-home-static__cta" href="/user/home">Open the workspace</a>
          <p class="public-home-static__note">No setup tour required. Start with a direction and keep the thread visible.</p>
        </div>
        <div class="public-home-static__canvas" aria-label="Connected AI canvas preview" role="img">
          <div class="public-home-static__canvas-grid"></div>
          <div class="public-home-static__node public-home-static__node--prompt"><span>Direction</span><strong>soft geometry</strong></div>
          <div class="public-home-static__node public-home-static__node--reference"><span>References</span><strong>tone + texture</strong></div>
          <div class="public-home-static__node public-home-static__node--result"><span>Next signal</span><strong>compare and refine</strong></div>
          <div class="public-home-static__connector public-home-static__connector--one"></div>
          <div class="public-home-static__connector public-home-static__connector--two"></div>
        </div>
      </section>
      <section class="public-home-static__section" aria-labelledby="public-home-workflow-heading">
        <div>
          <p class="public-home-static__eyebrow">A connected way to think</p>
          <h2 id="public-home-workflow-heading">Keep the visual thread intact.</h2>
        </div>
        <div class="public-home-static__steps">
          <article><span>01</span><h3>Set a direction</h3><p>Bring a prompt, sketch, or reference image into the same canvas.</p></article>
          <article><span>02</span><h3>Compare the paths</h3><p>Keep visual choices together so the strongest signal is easy to see.</p></article>
          <article><span>03</span><h3>Refine what matters</h3><p>Iterate on composition, style, and detail without losing context.</p></article>
        </div>
      </section>
      <section class="public-home-static__facts" aria-labelledby="public-home-facts-heading">
        <div><p class="public-home-static__eyebrow">Made for visual exploration</p><h2 id="public-home-facts-heading">A practical starting point for pattern and image ideas.</h2></div>
        <dl><div><dt>Inputs</dt><dd>${PUBLIC_HOME_FACT.inputs.map(escapeHtml).join(', ')}</dd></div><div><dt>Outputs</dt><dd>${PUBLIC_HOME_FACT.outputs.map(escapeHtml).join(', ')}</dd></div><div><dt>Scope</dt><dd>${escapeHtml(PUBLIC_HOME_FACT.limits[0] ?? '')}</dd></div></dl>
      </section>
      <section class="public-home-static__activation" aria-labelledby="public-home-activation-heading">
        <div><p class="public-home-static__eyebrow">Workspace Activation</p><h2 id="public-home-activation-heading">Give the next idea somewhere to move.</h2><p>Open a working Canvas Board and keep exploring from there.</p></div>
        <a class="public-home-static__cta public-home-static__cta--dark" href="/user/home">Open the workspace</a>
      </section>
    </main>
    <footer class="public-home-static__footer"><span>IdeaBoard AI Canvas</span><nav aria-label="Legal"><a href="/terms">Terms</a><a href="/privacy-policy">Privacy</a><a href="/refund-policy">Refunds</a><a href="/acceptable-use">Acceptable Use</a></nav></footer>
  </div>
`.trim()

export function renderPublicHomeDocument(html: string): string {
  const metadata = [
    `<meta name="description" content="${escapeHtml(PUBLIC_HOME_METADATA.description)}" />`,
    `<meta name="robots" content="index,follow" />`,
    `<link rel="canonical" href="${PUBLIC_HOME_METADATA.canonical}" />`,
    `<meta property="og:type" content="website" />`,
    `<meta property="og:site_name" content="IdeaBoard" />`,
    `<meta property="og:title" content="${escapeHtml(PUBLIC_HOME_METADATA.title)}" />`,
    `<meta property="og:description" content="${escapeHtml(PUBLIC_HOME_METADATA.description)}" />`,
    `<meta property="og:url" content="${PUBLIC_HOME_METADATA.canonical}" />`,
    `<meta property="og:image" content="${PUBLIC_HOME_METADATA.ogImage}" />`,
    `<meta property="og:locale" content="${PUBLIC_HOME_METADATA.locale}" />`,
    `<script type="application/ld+json">${serializePublicHomeJsonLd()}</script>`,
  ].join('\n    ')

  return html
    .replace('<html lang="zh-CN">', '<html lang="en">')
    .replace('<title>Idea Board</title>', `<title>${PUBLIC_HOME_METADATA.title}</title>`)
    .replace(
      /<meta name="viewport" content="width=device-width, initial-scale=1\.0"\s*\/>/,
      `<meta name="viewport" content="width=device-width, initial-scale=1.0" />\n    ${metadata}`,
    )
    .replace('<div id="app"></div>', `<div id="app">${PUBLIC_HOME_STATIC_MARKUP}</div>`)
}
