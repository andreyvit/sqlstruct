// Code generated by github.com/andreyvit/sqlstruct. DO NOT EDIT. (@generated)
package foos

import (
	"context"
	"database/sql"
	"github.com/andreyvit/sqlexpr"
)

type facet int

const (
	noneFacet facet = iota
	allFacet
	contentFacet
	immutableFacet
	mutableFacet
	pkFacet
	stateFacet
)

func (f facet) String() string {
	switch f {
	case noneFacet:
		return "none"
	case allFacet:
		return "all"
	case contentFacet:
		return "content"
	case immutableFacet:
		return "immutable"
	case mutableFacet:
		return "mutable"
	case pkFacet:
		return "pk"
	case stateFacet:
		return "state"
	default:
		panic("unknown facet")
	}
}

func scanPost(row *sql.Row, fct facet) (*Post, error) {
	v := new(Post)
	err := scanPostInto(row, v, fct)
	return v, err
}

func scanNextPost(rows *sql.Rows, fct facet) (*Post, error) {
	v := new(Post)
	err := scanNextPostInto(rows, v, fct)
	return v, err
}

func scanPostInto(row *sql.Row, v *Post, fct facet) error {
	switch fct {
	case pkFacet:
		return row.Scan(&v.ID)
	case allFacet:
		return row.Scan(&v.ID, &v.UUID, &v.AccountID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt, &v.State, &v.PublisedAt, &v.Title, &v.Body, &v.Music)
	case immutableFacet:
		return row.Scan(&v.ID, &v.UUID, &v.AccountID, &v.CreatedAt)
	case mutableFacet:
		return row.Scan(&v.UpdatedAt, &v.DeletedAt, &v.State, &v.PublisedAt, &v.Title, &v.Body, &v.Music)
	case stateFacet:
		return row.Scan(&v.State, &v.PublisedAt)
	case contentFacet:
		return row.Scan(&v.Title, &v.Body, &v.Music)
	default:
		panic("foos.Post does not support facet " + fct.String())
	}
}

func scanNextPostInto(rows *sql.Rows, v *Post, fct facet) error {
	switch fct {
	case pkFacet:
		return rows.Scan(&v.ID)
	case allFacet:
		return rows.Scan(&v.ID, &v.UUID, &v.AccountID, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt, &v.State, &v.PublisedAt, &v.Title, &v.Body, &v.Music)
	case immutableFacet:
		return rows.Scan(&v.ID, &v.UUID, &v.AccountID, &v.CreatedAt)
	case mutableFacet:
		return rows.Scan(&v.UpdatedAt, &v.DeletedAt, &v.State, &v.PublisedAt, &v.Title, &v.Body, &v.Music)
	case stateFacet:
		return rows.Scan(&v.State, &v.PublisedAt)
	case contentFacet:
		return rows.Scan(&v.Title, &v.Body, &v.Music)
	default:
		panic("foos.Post does not support facet " + fct.String())
	}
}

func scanAllPosts(rows *sql.Rows, fct facet) ([]*Post, error) {
	defer rows.Close()
	var result []*Post
	for rows.Next() {
		v, err := scanNextPost(rows, fct)
		if err != nil {
			return result, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func addPostFields(s sqlexpr.Fieldable, fct facet) {
	switch fct {
	case pkFacet:
		s.AddField(sqlexpr.Column("id"))
	case allFacet:
		s.AddField(sqlexpr.Column("id"))
		s.AddField(sqlexpr.Column("uuid"))
		s.AddField(sqlexpr.Column("account_id"))
		s.AddField(sqlexpr.Column("created_at"))
		s.AddField(sqlexpr.Column("updated_at"))
		s.AddField(sqlexpr.Column("deleted_at"))
		s.AddField(sqlexpr.Column("state"))
		s.AddField(sqlexpr.Column("published_at"))
		s.AddField(sqlexpr.Column("title"))
		s.AddField(sqlexpr.Column("description"))
		s.AddField(sqlexpr.Column("music"))
	case immutableFacet:
		s.AddField(sqlexpr.Column("id"))
		s.AddField(sqlexpr.Column("uuid"))
		s.AddField(sqlexpr.Column("account_id"))
		s.AddField(sqlexpr.Column("created_at"))
	case mutableFacet:
		s.AddField(sqlexpr.Column("updated_at"))
		s.AddField(sqlexpr.Column("deleted_at"))
		s.AddField(sqlexpr.Column("state"))
		s.AddField(sqlexpr.Column("published_at"))
		s.AddField(sqlexpr.Column("title"))
		s.AddField(sqlexpr.Column("description"))
		s.AddField(sqlexpr.Column("music"))
	case stateFacet:
		s.AddField(sqlexpr.Column("state"))
		s.AddField(sqlexpr.Column("published_at"))
	case contentFacet:
		s.AddField(sqlexpr.Column("title"))
		s.AddField(sqlexpr.Column("description"))
		s.AddField(sqlexpr.Column("music"))
	default:
		panic("foos.Post does not support facet " + fct.String())
	}
}

func addPostSetters(s sqlexpr.Settable, v *Post, fct facet) {
	switch fct {
	case pkFacet:
		s.Set(sqlexpr.Column("id"), v.ID)
	case allFacet:
		s.Set(sqlexpr.Column("id"), v.ID)
		s.Set(sqlexpr.Column("uuid"), v.UUID)
		s.Set(sqlexpr.Column("account_id"), v.AccountID)
		s.Set(sqlexpr.Column("created_at"), v.CreatedAt)
		s.Set(sqlexpr.Column("updated_at"), v.UpdatedAt)
		s.Set(sqlexpr.Column("deleted_at"), v.DeletedAt)
		s.Set(sqlexpr.Column("state"), v.State)
		s.Set(sqlexpr.Column("published_at"), v.PublisedAt)
		s.Set(sqlexpr.Column("title"), v.Title)
		s.Set(sqlexpr.Column("description"), v.Body)
		s.Set(sqlexpr.Column("music"), v.Music)
	case immutableFacet:
		s.Set(sqlexpr.Column("id"), v.ID)
		s.Set(sqlexpr.Column("uuid"), v.UUID)
		s.Set(sqlexpr.Column("account_id"), v.AccountID)
		s.Set(sqlexpr.Column("created_at"), v.CreatedAt)
	case mutableFacet:
		s.Set(sqlexpr.Column("updated_at"), v.UpdatedAt)
		s.Set(sqlexpr.Column("deleted_at"), v.DeletedAt)
		s.Set(sqlexpr.Column("state"), v.State)
		s.Set(sqlexpr.Column("published_at"), v.PublisedAt)
		s.Set(sqlexpr.Column("title"), v.Title)
		s.Set(sqlexpr.Column("description"), v.Body)
		s.Set(sqlexpr.Column("music"), v.Music)
	case stateFacet:
		s.Set(sqlexpr.Column("state"), v.State)
		s.Set(sqlexpr.Column("published_at"), v.PublisedAt)
	case contentFacet:
		s.Set(sqlexpr.Column("title"), v.Title)
		s.Set(sqlexpr.Column("description"), v.Body)
		s.Set(sqlexpr.Column("music"), v.Music)
	default:
		panic("foos.Post does not support facet " + fct.String())
	}
}

func addPostConditions(s sqlexpr.Whereable, v *Post, fct facet) {
	switch fct {
	case pkFacet:
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("id"), v.ID))
	case allFacet:
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("id"), v.ID))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("uuid"), v.UUID))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("account_id"), v.AccountID))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("created_at"), v.CreatedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("updated_at"), v.UpdatedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("deleted_at"), v.DeletedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("state"), v.State))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("published_at"), v.PublisedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("title"), v.Title))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("description"), v.Body))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("music"), v.Music))
	case immutableFacet:
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("id"), v.ID))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("uuid"), v.UUID))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("account_id"), v.AccountID))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("created_at"), v.CreatedAt))
	case mutableFacet:
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("updated_at"), v.UpdatedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("deleted_at"), v.DeletedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("state"), v.State))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("published_at"), v.PublisedAt))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("title"), v.Title))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("description"), v.Body))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("music"), v.Music))
	case stateFacet:
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("state"), v.State))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("published_at"), v.PublisedAt))
	case contentFacet:
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("title"), v.Title))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("description"), v.Body))
		s.AddWhere(sqlexpr.Eq(sqlexpr.Column("music"), v.Music))
	default:
		panic("foos.Post does not support facet " + fct.String())
	}
}

func buildSelectFromPosts(fct facet) *sqlexpr.Select {
	s := &sqlexpr.Select{From: sqlexpr.Table("posts")}
	addPostFields(s, fct)
	return s
}

func buildInsertPost(v *Post, setFct, retFct facet) *sqlexpr.Insert {
	s := &sqlexpr.Insert{Table: sqlexpr.Table("posts")}
	addPostSetters(s, v, setFct)
	addPostFields(s, retFct)
	return s
}

func buildUpdatePost(v *Post, condFct, updateFct, retFct facet) *sqlexpr.Update {
	s := &sqlexpr.Update{Table: sqlexpr.Table("posts")}
	addPostSetters(s, v, updateFct)
	addPostConditions(s, v, condFct)
	addPostFields(s, retFct)
	return s
}

func buildDeletePost(v *Post, condFct facet) *sqlexpr.Delete {
	s := &sqlexpr.Delete{Table: sqlexpr.Table("posts")}
	addPostConditions(s, v, condFct)
	return s
}

func fetchPost(ctx context.Context, ex sqlexpr.Executor, fct facet, f func(*sqlexpr.Select)) (*Post, error) {
	s := buildSelectFromPosts(fct)
	if f != nil {
		f(s)
	}
	row := s.QueryRow(ctx, ex)
	return scanPost(row, fct)
}

func fetchPosts(ctx context.Context, ex sqlexpr.Executor, fct facet, f func(*sqlexpr.Select)) ([]*Post, error) {
	s := buildSelectFromPosts(fct)
	if f != nil {
		f(s)
	}
	rows, err := s.Query(ctx, ex)
	if err != nil {
		return nil, err
	}
	return scanAllPosts(rows, fct)
}

func insertPost(ctx context.Context, ex sqlexpr.Executor, v *Post, setFct, retFct facet, f func(*sqlexpr.Insert)) error {
	s := buildInsertPost(v, setFct, retFct)
	if f != nil {
		f(s)
	}
	row := s.QueryRow(ctx, ex)
	return scanPostInto(row, v, retFct)
}

func updatePost(ctx context.Context, ex sqlexpr.Executor, v *Post, condFct, updateFct, retFct facet, f func(*sqlexpr.Update)) error {
	s := buildUpdatePost(v, condFct, updateFct, retFct)
	if f != nil {
		f(s)
	}
	row := s.QueryRow(ctx, ex)
	return scanPostInto(row, v, retFct)
}

func deletePost(ctx context.Context, ex sqlexpr.Executor, v *Post, condFct facet, f func(*sqlexpr.Delete)) error {
	s := buildDeletePost(v, condFct)
	if f != nil {
		f(s)
	}
	_, err := s.Exec(ctx, ex)
	return err
}
