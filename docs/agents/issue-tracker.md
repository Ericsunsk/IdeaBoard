# Issue Tracker Protocol

Issues and PRDs for this repository live in GitHub Issues for `Ericsunsk/IdeaBoard`. Use the `gh` CLI from inside this clone.

## Repository

- Repo: `Ericsunsk/IdeaBoard`
- Remote: `https://github.com/Ericsunsk/IdeaBoard`

Prefer inferring the repository from `git remote -v`.
Before the first issue creation in a session, verify the target with
`gh repo view --json nameWithOwner`; it must be `Ericsunsk/IdeaBoard`.

## Issue Operations

- Create: `gh issue create --title "..." --body-file <path>`
- View: `gh issue view <number> --json number,title,body,labels,comments`
- List: `gh issue list --state open --json number,title,body,labels,comments`
- Comment: `gh issue comment <number> --body-file <path>`
- Label: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`
- Close: post the explanation as a comment, then run `gh issue close <number>`.

Use a file for multiline bodies. Comment separately before closing:
`gh issue close --comment` can discard the comment when the issue is already closed.

## Pull Requests as a Triage Surface

**PRs as a request surface: no.**

Do not include pull requests in the `triage` queue.

## Skill Phrases

- "Publish to the issue tracker" means create a GitHub issue.
- "Fetch the relevant ticket" means view the GitHub issue and its comments.

## Triage Labels

Use the role-to-label mapping in `docs/agents/triage-labels.md`.

## Wayfinding Operations

The `wayfinder` skill uses one map issue with child issues as tickets.

- Map: label it `wayfinder:map`.
- Child ticket: use a GitHub sub-issue when available; otherwise link it from the map task list and add `Part of #<map>` to the child.
- Ticket type: use `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling`, or `wayfinder:task`.
- Blocking: prefer GitHub native issue dependencies; otherwise add `Blocked by: #<number>` to the ticket.
- Claim: `gh issue edit <number> --add-assignee @me`.
- Resolve: comment with the outcome, close the ticket, and update the map's Decisions-so-far.
