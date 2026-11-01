# Wiki Location Decision

- On 2026-11-01, user selected `docs/wiki/` as the shared wiki location (session decision, no prior wiki found).
- Convention (from chief-of-staff skill):
  - `index.md` lists every page with link + one-line summary, grouped by category.
  - One topic per page; link related pages; cite sources, never copy them in.
  - Date every code claim with repo+commit; mark superseded, don't rewrite.

## 2026-11-01 — Process-document retention

- On 2026-11-01, the user requested cleanup of procedural and redundant Markdown (session decision). The retention policy was added to [DESIGN.md](../../../DESIGN.md#文档职责): accepted rules belong in canonical docs, confirmed work belongs in GitHub Issues, and the wiki retains dated decisions and verification gaps rather than execution transcripts.
- On 2026-11-01, cleanup based on `Ericsunsk/IdeaBoard@97a364044` selected three unreferenced process documents for removal from the working tree. Their source snapshots remain in Git history: [backend naming plan](https://github.com/Ericsunsk/IdeaBoard/blob/97a364044/docs/architecture/backend-naming-remediation-plan.md), [backend redundancy audit](https://github.com/Ericsunsk/IdeaBoard/blob/97a364044/docs/architecture/backend-redundancy-audit.md), and [completed API comparison](https://github.com/Ericsunsk/IdeaBoard/blob/97a364044/docs/architecture/api-standards-research.md). Removal does not mean every proposed change was accepted or implemented.
- On 2026-11-01, the cleanup retained research sources, runbooks, ADRs, local context docs, and the Seam inventory / Knip ledger consumed by quality gates. The earlier [static-analysis baseline](../operations/static-analysis-baseline.md) remained as dated verification evidence, including its recorded DB/NATS and Fresh replay gaps.

Related: [Wiki Index](../index.md)
