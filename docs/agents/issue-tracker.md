# Issue Tracker Protocol

Issues and PRDs for this repository live in GitHub Issues for `Ericsunsk/IdeaBoard`. Use the `gh` CLI from inside this clone.

## Repository

- Repo: `Ericsunsk/IdeaBoard`
- Remote: `https://github.com/Ericsunsk/IdeaBoard`

Prefer inferring the repository from `git remote -v`; `gh` does this automatically inside the clone.

## Issue Operations

- Create: `gh issue create --title "..." --body "..."`
- View: `gh issue view <number> --comments`
- List: `gh issue list --state open --json number,title,body,labels,comments`
- Comment: `gh issue comment <number> --body "..."`
- Label: `gh issue edit <number> --add-label "..."` or `--remove-label "..."`
- Close: `gh issue close <number> --comment "..."`

Use a heredoc or file for multi-line bodies.

## Pull Requests as a Triage Surface

**PRs as a request surface: no.**

Do not include pull requests in the `triage` queue. If this repository later accepts external PRs as feature requests, update this setting before triaging them.

## Skill Phrases

- "Publish to the issue tracker" means create a GitHub issue.
- "Fetch the relevant ticket" means view the GitHub issue and its comments.

## Wayfinding Operations

The `wayfinder` skill uses one map issue with child issues as tickets.

- Map: label it `wayfinder:map`.
- Child ticket: use a GitHub sub-issue when available; otherwise link it from the map task list and add `Part of #<map>` to the child.
- Ticket type: use `wayfinder:research`, `wayfinder:prototype`, `wayfinder:grilling`, or `wayfinder:task`.
- Blocking: prefer GitHub native issue dependencies; otherwise add `Blocked by: #<number>` to the ticket.
- Claim: `gh issue edit <number> --add-assignee @me`.
- Resolve: comment with the outcome, close the ticket, and update the map's Decisions-so-far.
