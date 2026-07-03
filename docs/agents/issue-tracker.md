# Issue Tracker Protocol

Issues, PRDs, and external PR triage live in GitHub for `Ericsunsk/IdeaBoard`. Use `gh` from inside this clone whenever possible.

## Repository

- Repo: `Ericsunsk/IdeaBoard`
- Remote: `https://github.com/Ericsunsk/IdeaBoard`

Prefer inferring the repo from `git remote -v`; `gh` does this automatically inside the clone.

## Issues

- Create: `gh issue create --title "..." --body "..."`
- View: `gh issue view <number> --comments`
- List: `gh issue list --state open --json number,title,body,labels,comments`
- Comment: `gh issue comment <number> --body "..."`
- Label: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- Close: `gh issue close <number> --comment "..."`

Use a heredoc or a file for multi-line bodies.

## Pull Requests

Treat external PRs as a triage surface.

- View: `gh pr view <number> --comments`
- Diff: `gh pr diff <number>`
- List: `gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`
- Comment/label/close: `gh pr comment`, `gh pr edit --add-label` / `--remove-label`, `gh pr close`

For external PR triage, keep only `authorAssociation` values of `CONTRIBUTOR`, `FIRST_TIME_CONTRIBUTOR`, or `NONE`. Drop `OWNER`, `MEMBER`, and `COLLABORATOR`.

GitHub shares one number space for issues and PRs. Resolve `#42` with `gh pr view 42`; if that fails, use `gh issue view 42`.

## Skill Phrases

- "Publish to the issue tracker" means create a GitHub issue.
- "Fetch the relevant ticket" means view the issue or PR with comments.
