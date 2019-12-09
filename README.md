# Use Go structs with `database/sql`

Two goals:

1. Build SQL statements (SELECT, INSERT, UPDATE, DELETE) from structs.

2. Scan `*sql.Row` and `*sql.Rows` into structs.

Principles:

1. Does two things, and does them well. Not an ORM. Not a framework. Does not take over the entire app.
2. Handles NULLs and custom types.
3. Allows to specify which fields to use for every operation (via named field sets that we call _facets_).
4. Works with [`github.com/andreyvit/sqlexpr`](https://github.com/andreyvit/sqlexpr).
