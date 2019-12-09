# Use Go structs with `database/sql`

Two goals:

1. Build SQL statements (SELECT, INSERT, UPDATE, DELETE) from structs.

2. Scan `*sql.Row` and `*sql.Rows` into structs.

Principles:

1. Does two things, and does them well. Not an ORM. Not a framework. Does not take over the entire app.
2. Handles NULLs and custom types.
3. Allows to specify which fields to use for every operation (via named field sets that we call _facets_).
4. Works with [`github.com/andreyvit/sqlexpr`](https://github.com/andreyvit/sqlexpr).

Installation:

    go get -u github.com/andreyvit/sqlstruct/cmd/sqlstruct

## Example

Given a struct like:

```go
type Post struct {
    ID   int64  `json:"-" db:"id,pk"`

    CreatedAt time.Time `db:"created_at,immutable"`
    UpdatedAt time.Time `db:"updated_at"`
    DeletedAt time.Time `db:"deleted_at,nullable"`

    Title string `db:"title"`
    Body  string `db:"description"`
}
```

Generates scanning functions that `.Scan` into this struct:

```go
func scanPost(row *sql.Row, fct facet) (*model.Post, error)
func scanPostInto(row *sql.Row, v *model.Post, fct facet) error

func scanNextPost(rows *sql.Rows, fct facet) (*model.Post, error)
func scanNextPostInto(rows *sql.Rows, v *model.Post, fct facet) error

func scanAllPosts(rows *sql.Rows, fct facet) ([]*model.Post, error)
```

Generates SQL building functions that build [`sqlexpr`](https://github.com/andreyvit/sqlexpr) statements for selecting, inserting, updating and deleting:


```go
func buildSelectFromPosts(fct facet) *sqlexpr.Select
func buildInsertPost(v *model.Post, setFct, retFct facet) *sqlexpr.Insert
func buildUpdatePost(v *model.Post, condFct, updateFct, retFct facet) *sqlexpr.Update
func buildDeletePost(v *model.Post, condFct facet) *sqlexpr.Delete

func addPostFields(s sqlexpr.Fieldable, fct facet)
func addPostSetters(s sqlexpr.Settable, v *model.Post, fct facet)
func addPostConditions(s sqlexpr.Whereable, v *model.Post, fct facet)
```

And generates execution functions that tie all the above together: they generate a statement, call a callback to customize it, then execute it via `sqlexpr.Executor` (which is an interface that `*sql.DB` and `*sql.Tx` conform to), then scan the results:

```go
func fetchPost(ctx context.Context, ex sqlexpr.Executor, fct facet, f func(*sqlexpr.Select)) (*model.Post, error)
func fetchPosts(ctx context.Context, ex sqlexpr.Executor, fct facet, f func(*sqlexpr.Select)) ([]*model.Post, error)
func insertPost(ctx context.Context, ex sqlexpr.Executor, v *model.Post, setFct, retFct facet, f func(*sqlexpr.Insert)) error
func updatePost(ctx context.Context, ex sqlexpr.Executor, v *model.Post, condFct, updateFct, retFct facet, f func(*sqlexpr.Update)) error
func deletePost(ctx context.Context, ex sqlexpr.Executor, v *model.Post, condFct facet, f func(*sqlexpr.Delete)) error
```

Facet enum is generated based on all facets (hashtags) defined by the structs. The facets determine the list of fields that are used in `WHERE`, `SET`, `SELECT` or `RETURNING` clauses. Using facets, you can fetch or update only the relevant fields.


## Development

Use [modd](https://github.com/cortesi/modd) (`go get github.com/cortesi/modd/cmd/modd`) to re-run tests automatically during development by running `modd` (recommended).


## License

MIT.
