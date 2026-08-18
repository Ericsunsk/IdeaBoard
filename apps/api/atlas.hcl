// Atlas owns API schema migrations. Auth keeps its Better Auth migration
// lifecycle separate because its tables and runtime role have a different owner.
env "local" {
  migration {
    dir = "file://internal/db/migrations"
  }
}

env "ci" {
  migration {
    dir = "file://internal/db/migrations"
  }
}

