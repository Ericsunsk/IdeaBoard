## Local Tools Available
- **`sqlc`**: The Go SQL compiler is installed locally. When you modify the SQL schema (`apps/api/internal/db/schema/schema.sql`) or queries (`apps/api/internal/db/queries/*.sql`), run `sqlc generate` directly in the `apps/api` directory to automatically compile and generate type-safe Go database access code.
- **`ripgrep (rg)` & `fd`**: Ultra-fast text search and file finding tools are installed globally. Please prioritize using these CLI tools when doing codebase-wide search or locating files.
- **`golangci-lint`**: The Go static analysis tool is installed locally. Run `golangci-lint run ./...` in the `apps/api` directory to run lint and security checks before submitting or committing Go code.
- **`hurl`**: The HTTP testing tool is installed locally. You can write `.hurl` files and run `hurl --test` to perform API endpoint regression testing.
- **`air`**: Live-reload/hot-reload tool for Go applications. Run `air` in the `apps/api` directory to automatically compile and restart the backend server upon file modifications.
- **`task` (go-task)**: A task runner / build tool. Use `task --list` at the project root directory to inspect and execute local orchestration tasks (e.g., database initialization, Docker service management, or full-stack setup).
- **`atlas`**: Declarative schema migration and management tool for PostgreSQL. Use `atlas schema inspect` or `atlas migrate diff` to compare your database schema definitions and generate SQL migration scripts automatically.
- **`stylelint`**: CSS and Vue style linter. Run `npm run lint:style` in the `apps/web` directory to audit CSS and Vue component styling through the project Stylelint configuration.
