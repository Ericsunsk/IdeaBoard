# Issue tracker: GitHub

Issues and PRDs for this repo live as GitHub issues. Use the `gh` CLI for all operations.

## Repository

- GitHub repo: `Ericsunsk/IdeaBoard`
- Remote: `https://github.com/Ericsunsk/IdeaBoard`

Infer the repo from `git remote -v` when possible. The `gh` CLI does this automatically when run inside the clone.

## Conventions

- Create an issue: `gh issue create --title "..." --body "..."`
- Read an issue: `gh issue view <number> --comments`
- List issues: `gh issue list --state open --json number,title,body,labels,comments`
- Comment on an issue: `gh issue comment <number> --body "..."`
- Apply or remove labels: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- Close an issue: `gh issue close <number> --comment "..."`

Use a heredoc or a file for multi-line bodies to keep shell quoting reliable.

## Pull requests as a triage surface

PRs as a request surface: yes.

External PRs should run through the same labels and states as issues. Use the `gh pr` equivalents:

- Read a PR: `gh pr view <number> --comments`
- Inspect a PR diff: `gh pr diff <number>`
- List open PRs: `gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`
- Comment, label, or close: `gh pr comment`, `gh pr edit --add-label` / `--remove-label`, `gh pr close`

When triaging external PRs, keep only `authorAssociation` values of `CONTRIBUTOR`, `FIRST_TIME_CONTRIBUTOR`, or `NONE`. Drop `OWNER`, `MEMBER`, and `COLLABORATOR`.

GitHub shares one number space across issues and PRs, so a bare `#42` may be either. Resolve with `gh pr view 42` and fall back to `gh issue view 42`.

## When a skill says "publish to the issue tracker"

Create a GitHub issue.

## When a skill says "fetch the relevant ticket"

Run `gh issue view <number> --comments`. If the number is a PR, use `gh pr view <number> --comments`.
