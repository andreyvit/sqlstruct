package sqlstruct

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/structtag"
)

const (
	databaseSQL = "database/sql"
	sqlexpr     = "github.com/andreyvit/sqlexpr"
	contextPkg  = "context"
)

type Logger interface {
	Printf(format string, v ...interface{})
}

type Options struct {
	InputFileName string
	InputPkgName  string

	PkgName string

	Published bool

	Logger  Logger
	Verbose bool
}

type generator struct {
	log Logger

	inPkg string

	published bool
	types     map[string]Type

	persistentStructs []*Struct

	facets map[string]*Facet

	facetTypeIdent string
	noneFacetIdent string
}

func Generate(src []byte, opt Options) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, opt.InputFileName, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	if opt.PkgName == "" {
		opt.PkgName = file.Name.Name
	}

	g := &generator{
		log:       opt.Logger,
		published: opt.Published,
		inPkg:     opt.InputPkgName,

		types:  make(map[string]Type),
		facets: make(map[string]*Facet),
	}
	g.initBuiltInTypes()

	for pass := 1; pass <= 2; pass++ {
		var err error
		for _, d := range file.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				g.log.Printf("Func %s", fn.Name.Name)
			} else if decl, ok := d.(*ast.GenDecl); ok {
				switch decl.Tok {
				case token.TYPE:
					for _, s := range decl.Specs {
						if ts, ok := s.(*ast.TypeSpec); ok {
							name := Name{g.inPkg, ts.Name.Name}
							if stru, ok := ts.Type.(*ast.StructType); ok {
								if pass == 1 {
									err = g.preprocessStruct(ts, name, stru)
								} else {
									err = g.processStruct(ts, name, stru)
								}
								if err != nil {
									return nil, err
								}
							} else {
								if pass == 1 {
									alias, err := g.resolveType(ts.Type)
									if err == nil {
										err = g.processAliasTypeDef(ts, name, alias)
									} else {
										err = g.processUnknownTypeDef(ts, name)
									}
									if err != nil {
										return nil, err
									}
								}
							}
						} else {
							panic("unexpected spec in type declaration")
						}
					}
				}
			}
		}
	}

	g.facetTypeIdent = g.pubOrUnpub("Facet")
	g.noneFacetIdent = g.pubOrUnpub("NoneFacet")

	cg := newCodeGen(opt.PkgName)
	g.generateCode(cg)

	formatted, err := format.Source(cg.Bytes())
	if err != nil {
		return nil, err
	}

	return formatted, nil
}

func (g *generator) initBuiltInTypes() {
	g.addType(&WellKnownPrimitive{Name: Name{"", "int64"}, ZeroValue: "0"})
	g.addType(&WellKnownPrimitive{Name: Name{"", "int"}, ZeroValue: "0"})
	g.addType(&WellKnownPrimitive{Name: Name{"", "string"}, ZeroValue: `""`})
	g.addType(&WellKnownPrimitive{Name: Name{"", "bool"}, ZeroValue: "false"})
	g.addType(&WellKnownStruct{Name: Name{"time", "Time"}, IsZeroMethod: "IsZero", EqualMethod: "Equal"})
}

func (g *generator) addType(t Type) {
	fqn := t.TypeName().FQN()
	g.types[fqn] = t
}

func (g *generator) pubOrUnpub(ident string) string {
	if g.published {
		return publishedName(ident)
	} else {
		return unpublishedName(ident)
	}
}

func (g *generator) makeStructIdent(s *Struct, prefix, suffix string, plural bool) string {
	var ident string
	if plural {
		ident = s.PluralIdent
	} else {
		ident = s.SingularIdent
	}
	return g.pubOrUnpub(prefix + ident + suffix)
}

func (g *generator) processAliasTypeDef(ts *ast.TypeSpec, name Name, aliasOf Type) error {
	g.log.Printf("type %v is an alias of %v", name, aliasOf)
	g.addType(&AliasType{name, aliasOf})
	return nil
}

func (g *generator) processUnknownTypeDef(ts *ast.TypeSpec, name Name) error {
	g.log.Printf("type %v not a struct", name)
	g.addType(&UnsupportedType{name})
	return nil
}

func (g *generator) preprocessStruct(ts *ast.TypeSpec, structName Name, stru *ast.StructType) error {
	s := &Struct{
		Name: structName,
	}
	g.addType(s)
	return nil
}

func (g *generator) lookupFacet(name string) *Facet {
	if f := g.facets[name]; f != nil {
		return f
	}

	f := &Facet{
		Name:  name,
		Ident: g.pubOrUnpub(name) + "Facet",
	}
	g.facets[name] = f
	return f
}

func (g *generator) processStruct(ts *ast.TypeSpec, structName Name, stru *ast.StructType) error {
	structType, err := g.resolveTypeName(structName)
	if err != nil {
		panic("struct not found on second pass: " + ts.Name.Name)
	}
	s := structType.(*Struct)

	g.log.Printf("type %s struct", s.Name.Ident)
	for _, field := range stru.Fields.List {
		if field.Names == nil {
			err := g.processField(s, field, "")
			if err != nil {
				return err
			}
		} else {
			for _, nameIdent := range field.Names {
				err := g.processField(s, field, nameIdent.Name)
				if err != nil {
					return err
				}
			}
		}
	}

	if ts.Comment != nil {
		for _, comment := range ts.Comment.List {
			text := comment.Text
			if t := strings.TrimPrefix(text, "/*"); t != text {
				text = strings.TrimSuffix(t, "*/")
			} else if t := strings.TrimPrefix(text, "//"); t != text {
				text = t
			} else {
				panic(fmt.Errorf("invalid comment text %q", text))
			}
			text = strings.TrimSpace(text)
			if !strings.HasPrefix(text, "db:") {
				continue
			}

			for _, line := range strings.Split(text, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				fields := append(strings.Fields(line), "", "", "", "")
				switch fields[0] {
				case "db:plural":
					s.PluralIdent = g.pubOrUnpub(fields[1])
				case "db:table":
					s.TableName = fields[1]
				case "db:facet":
					//
				case "db:select_by":
					//
				default:
					return fmt.Errorf("invalid struct %s DB comment line %q", s.Name, line)
				}
			}
			g.log.Printf("******* comment for %s: %q", s.Name, comment.Text)
		}
		// panic("XXX")
	}

	if s.SingularIdent == "" {
		s.SingularIdent = structName.Ident
	}
	s.SingularIdent = publishedName(s.SingularIdent)

	if s.PluralIdent == "" {
		s.PluralIdent = s.SingularIdent + "s"
	}
	s.PluralIdent = publishedName(s.PluralIdent)

	if s.TableName == "" {
		s.TableName = strings.ToLower(s.PluralIdent)
	}

	g.finalizeStructWithoutRefs(s)
	if s.Persistent {
		g.persistentStructs = append(g.persistentStructs, s)
	}

	return nil
}

func (g *generator) finalizeStructWithoutRefs(s *Struct) {
	s.Persistent = (len(s.Cols) > 0)
	if !s.Persistent {
		return
	}

	for i, col := range s.Cols {
		col.IndexInStruct = i
	}

	for _, col := range s.Cols {
		col.Facets = append(col.Facets, g.lookupFacet("all"))
		if col.Immutable {
			col.Facets = append(col.Facets, g.lookupFacet("immutable"))
		} else {
			col.Facets = append(col.Facets, g.lookupFacet("mutable"))
		}
	}

	facets := make(map[string]bool)
	for _, col := range s.Cols {
		for _, f := range col.Facets {
			if !facets[f.Name] {
				facets[f.Name] = true
				s.Facets = append(s.Facets, f)
			}
		}
	}
}

func (g *generator) finalizeStructWithRefs(s *Struct) {
}

func (g *generator) processField(s *Struct, field *ast.Field, name string) error {
	var tag *structtag.Tag
	if field.Tag != nil && field.Tag.Kind == token.STRING {
		tagStr, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", s.Name, name, err)
		}

		tags, err := structtag.Parse(tagStr)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", s.Name, name, err)
		}

		tag, _ = tags.Get("db")
	}

	if tag == nil {
		return nil
	}

	typ, err := g.resolveType(field.Type)
	if err != nil {
		return fmt.Errorf("%s.%s: %w", s.Name, name, err)
	}

	if name == "" {
		name = typ.TypeName().Ident
	}

	col := &Col{
		FieldName: name,
	}
	col.TempVarName = makeSafeTempName(name)

	if tag.Name != "" {
		col.DBName = tag.Name
	} else {
		col.DBName = col.FieldName
	}

	for _, opt := range tag.Options {
		if opt == "nullable" {
			col.Nullable = true
		} else if opt == "immutable" {
			col.Immutable = true
		} else if opt == "pk" {
			col.Immutable = true
			col.Facets = append(col.Facets, g.lookupFacet("pk"))
		} else if strings.HasPrefix(opt, "#") {
			facetName := strings.TrimPrefix(opt, "#")
			col.Facets = append(col.Facets, g.lookupFacet(facetName))
		} else {
			return fmt.Errorf("%s.%s tag has invalid option %q", s.Name, col.FieldName, opt)
		}
	}

	s.Cols = append(s.Cols, col)

	g.log.Printf("field %s, tag %s, type %#v", name, tag, field.Type)

	return nil
}

func makeSafeTempName(ident string) string {
	n := unpublishedName(ident)
	switch n {
	case "v", "fct":
		return n + "Val"
	default:
		return n
	}
}

func (g *generator) resolveType(t ast.Expr) (Type, error) {
	switch v := t.(type) {
	case *ast.Ident:
		name := Name{"", v.Name}
		return g.resolveTypeName(name)
	case *ast.SelectorExpr:
		switch v1 := v.X.(type) {
		case *ast.Ident:
			name := Name{v1.Name, v.Sel.Name}
			return g.resolveTypeName(name)
		default:
			return &UnknownType{"!unknown", v.Sel.Name}, fmt.Errorf("unknown selector expr %#v", v.X)
		}
	case *ast.StarExpr:
		typ, err := g.resolveType(v.X)
		if err != nil {
			return &UnknownType{"!unknown", "unknown"}, err
		}
		return PtrType{typ}, nil
	default:
		return &UnknownType{"!unknown", "unknown"}, fmt.Errorf("unknown type %#v", t)
	}
}

func (g *generator) resolveTypeName(n Name) (Type, error) {
	fqn := n.FQN()
	if t, ok := g.types[fqn]; ok {
		return t, nil
	} else {
		g.log.Printf("Cannot resolve %v, known types are %+v", n, g.types)
		return UnknownType(n), fmt.Errorf("unknown type %s", fqn)
	}
}

func (g *generator) generateCode(cg *codeGen) {
	for _, s := range g.persistentStructs {
		g.finalizeStructWithRefs(s)
	}

	g.generateFacetsCode(cg)

	for _, s := range g.persistentStructs {
		g.generateStructCode(cg, s)
	}
}

func (g *generator) generateFacetsCode(cg *codeGen) {
	var facets []*Facet
	for _, f := range g.facets {
		facets = append(facets, f)
	}

	sort.Slice(facets, func(i, j int) bool {
		return facets[i].Name < facets[j].Name
	})

	cg.Printf("type %s int\n\n", g.facetTypeIdent)
	cg.Printf("const (\n")
	cg.Printf("\t%s %s = iota\n", g.noneFacetIdent, g.facetTypeIdent)
	for i, f := range facets {
		f.Value = i + 1
		cg.Printf("\t%s\n", f.Ident)
	}
	cg.Printf(")\n\n")
	cg.Printf("func (f %s) String() string {\n", g.facetTypeIdent)
	cg.Printf("\tswitch f {\n")
	cg.Printf("\tcase %s:\n", g.noneFacetIdent)
	cg.Printf("\t\treturn %q\n", "none")
	for _, f := range facets {
		cg.Printf("\tcase %s:\n", f.Ident)
		cg.Printf("\t\treturn %q\n", f.Name)
	}
	cg.Printf("\tdefault:\n")
	cg.Printf("\t\tpanic(%q)\n", "unknown facet")
	cg.Printf("\t}\n")
	cg.Printf("}\n\n")
}

type funcNames struct {
	scanOne          string
	scanOneNew       string
	scanOneOfMany    string
	scanOneOfManyNew string
	scanMult         string

	addFields     string
	addSetters    string
	addConditions string

	buildSelect string
	buildInsert string
	buildUpdate string
	buildDelete string

	selectOne  string
	selectMany string
	insertOne  string
	updateOne  string
	deleteOne  string
}

func (g *generator) generateStructCode(cg *codeGen, s *Struct) {
	fns := &funcNames{
		scanOne:          g.makeStructIdent(s, "Scan", "Into", false),
		scanOneNew:       g.makeStructIdent(s, "Scan", "", false),
		scanOneOfMany:    g.makeStructIdent(s, "ScanNext", "Into", false),
		scanOneOfManyNew: g.makeStructIdent(s, "ScanNext", "", false),
		scanMult:         g.makeStructIdent(s, "ScanAll", "", true),

		addFields:     g.makeStructIdent(s, "Add", "Fields", false),
		addSetters:    g.makeStructIdent(s, "Add", "Setters", false),
		addConditions: g.makeStructIdent(s, "Add", "Conditions", false),

		buildSelect: g.makeStructIdent(s, "BuildSelectFrom", "", true),
		buildInsert: g.makeStructIdent(s, "BuildInsert", "", false),
		buildUpdate: g.makeStructIdent(s, "BuildUpdate", "", false),
		buildDelete: g.makeStructIdent(s, "BuildDelete", "", false),

		selectOne:  g.makeStructIdent(s, "Fetch", "", false),
		selectMany: g.makeStructIdent(s, "Fetch", "", true),
		insertOne:  g.makeStructIdent(s, "Insert", "", false),
		updateOne:  g.makeStructIdent(s, "Update", "", false),
		deleteOne:  g.makeStructIdent(s, "Delete", "", false),
	}

	g.generateStructScanOneNew(cg, s, fns.scanOneNew, fns.scanOne, "row", Name{databaseSQL, "Row"})
	g.generateStructScanOneNew(cg, s, fns.scanOneOfManyNew, fns.scanOneOfMany, "rows", Name{databaseSQL, "Rows"})
	g.generateStructScanOne(cg, s, fns.scanOne, "row", Name{databaseSQL, "Row"})
	g.generateStructScanOne(cg, s, fns.scanOneOfMany, "rows", Name{databaseSQL, "Rows"})
	g.generateStructScanMult(cg, s, fns)

	g.generateStructFields(cg, s, fns)
	g.generateStructSetters(cg, s, fns)
	g.generateStructConditions(cg, s, fns)

	g.generateStructBuildSelect(cg, s, fns)
	g.generateStructBuildInsert(cg, s, fns)
	g.generateStructBuildUpdate(cg, s, fns)
	g.generateStructBuildDelete(cg, s, fns)

	g.generateStructSelectOne(cg, s, fns)
	g.generateStructSelectMany(cg, s, fns)
	g.generateStructInsertOne(cg, s, fns)
	g.generateStructUpdateOne(cg, s, fns)
	g.generateStructDeleteOne(cg, s, fns)
}

func (g *generator) generateStructScanOneNew(cg *codeGen, s *Struct, fn string, scanfn string, argName string, argType Name) {
	cg.Printf("func %s(%s *%s, fct %s) (*%s, error) {\n", fn, argName, cg.qualified(argType), g.facetTypeIdent, cg.qualified(s.Name))
	cg.Printf("\tv := new(%s)\n", cg.qualified(s.Name))
	cg.Printf("\terr := %s(%s, v, fct)\n", scanfn, argName)
	cg.Printf("\treturn v, err\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructScanOne(cg *codeGen, s *Struct, fn string, argName string, argType Name) {
	cg.Printf("func %s(%s *%s, v *%s, fct %s) error {\n", fn, argName, cg.qualified(argType), cg.qualified(s.Name), g.facetTypeIdent)

	scanners := makeScanners("v", s)

	generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		colScanners := filterExprs(scanners, cols)

		appendBefore(cg, colScanners)

		isSimple := true
		for _, expr := range colScanners {
			if !(len(expr.Vars) == 0 && len(expr.Before) == 0 && len(expr.After) == 0) {
				isSimple = false
				break
			}
		}

		if isSimple {
			cg.Printf("\t\treturn %s.Scan(", argName)
		} else {
			cg.Printf("\t\terr := %s.Scan(", argName)
		}
		for i, expr := range colScanners {
			if i > 0 {
				cg.Printf(", ")
			}

			cg.Printf(expr.Value)
		}
		cg.Printf(")\n")

		appendAfter(cg, colScanners)

		if !isSimple {
			cg.Printf("\t\treturn err\n")
		}
	})

	cg.Printf("}\n\n")
}

func (g *generator) generateStructScanMult(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)

	cg.Printf("func %s(rows *%s, fct %s) ([]*%s, error) {\n", fns.scanMult, cg.qualified(Name{databaseSQL, "Rows"}), g.facetTypeIdent, sFQN)
	cg.Printf("\tdefer rows.Close()\n")

	cg.Printf("\tvar result []*%s\n", sFQN)
	cg.Printf("\tfor rows.Next() {\n")
	cg.Printf("\t\tv, err := %s(rows, fct)\n", fns.scanOneOfManyNew)
	cg.Printf("\t\tif err != nil {\n")
	cg.Printf("\t\t\treturn result, err\n")
	cg.Printf("\t\t}\n")
	cg.Printf("\t\tresult = append(result, v)\n")
	cg.Printf("\t}\n")
	cg.Printf("\treturn result, rows.Err()\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructFields(cg *codeGen, s *Struct, fns *funcNames) {
	se := cg.imported(sqlexpr)

	cg.Printf("func %s(s %s.Fieldable, fct %s) {\n", fns.addFields, se, g.facetTypeIdent)
	generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		for _, col := range cols {
			cg.Printf("\t\ts.AddField(%s.Column(%q))\n", se, col.DBName)
		}
	})
	cg.Printf("}\n\n")
}

func (g *generator) generateStructSetters(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)

	encoders := makeEncoders("v", s)

	cg.Printf("func %s(s %s.Settable, v *%s, fct %s) {\n", fns.addSetters, se, sFQN, g.facetTypeIdent)
	generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		colEncoders := filterExprs(encoders, cols)
		appendBefore(cg, colEncoders)
		for i, col := range cols {
			cg.Printf("\t\ts.Set(%s.Column(%q), %s)\n", se, col.DBName, colEncoders[i].Value)
		}
		appendAfter(cg, colEncoders)
	})
	cg.Printf("}\n\n")
}

func (g *generator) generateStructConditions(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)

	encoders := makeEncoders("v", s)

	cg.Printf("func %s(s %s.Whereable, v *%s, fct %s) {\n", fns.addConditions, se, sFQN, g.facetTypeIdent)
	generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		colEncoders := filterExprs(encoders, cols)
		appendBefore(cg, colEncoders)
		for i, col := range cols {
			cg.Printf("\t\ts.AddWhere(%s.Eq(%s.Column(%q), %s))\n", se, se, col.DBName, colEncoders[i].Value)
		}
		appendAfter(cg, colEncoders)
	})
	cg.Printf("}\n\n")
}

func (g *generator) generateStructBuildSelect(cg *codeGen, s *Struct, fns *funcNames) {
	se := cg.imported(sqlexpr)

	cg.Printf("func %s(fct %s) *%s.Select {\n", fns.buildSelect, g.facetTypeIdent, se)
	cg.Printf("\ts := &%s.Select{From: %s.Table(%q)}\n", se, se, s.TableName)
	cg.Printf("\t%s(s, fct)\n", fns.addFields)
	cg.Printf("\treturn s\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructBuildInsert(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)

	cg.Printf("func %s(v *%s, setFct, retFct %s) *%s.Insert {\n", fns.buildInsert, sFQN, g.facetTypeIdent, se)
	cg.Printf("\ts := &%s.Insert{Table: %s.Table(%q)}\n", se, se, s.TableName)
	cg.Printf("\t%s(s, v, setFct)\n", fns.addSetters)
	cg.Printf("\t%s(s, retFct)\n", fns.addFields)
	cg.Printf("\treturn s\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructBuildUpdate(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)

	cg.Printf("func %s(v *%s, condFct, updateFct, retFct %s) *%s.Update {\n", fns.buildUpdate, sFQN, g.facetTypeIdent, se)
	cg.Printf("\ts := &%s.Update{Table: %s.Table(%q)}\n", se, se, s.TableName)
	cg.Printf("\t%s(s, v, updateFct)\n", fns.addSetters)
	cg.Printf("\t%s(s, v, condFct)\n", fns.addConditions)
	cg.Printf("\t%s(s, retFct)\n", fns.addFields)
	cg.Printf("\treturn s\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructBuildDelete(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)

	cg.Printf("func %s(v *%s, condFct %s) *%s.Delete {\n", fns.buildDelete, sFQN, g.facetTypeIdent, se)
	cg.Printf("\ts := &%s.Delete{Table: %s.Table(%q)}\n", se, se, s.TableName)
	cg.Printf("\t%s(s, v, condFct)\n", fns.addConditions)
	cg.Printf("\treturn s\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructSelectOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, fct %s, f func(*%s.Select)) (*%s, error) {\n", fns.selectOne, ctx, se, g.facetTypeIdent, se, sFQN)
	cg.Printf("\ts := %s(fct)\n", fns.buildSelect)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\trow := s.QueryRow(ctx, ex)\n")
	cg.Printf("\treturn %s(row, fct)\n", fns.scanOneNew)
	cg.Printf("}\n\n")
}

func (g *generator) generateStructSelectMany(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, fct %s, f func(*%s.Select)) ([]*%s, error) {\n", fns.selectMany, ctx, se, g.facetTypeIdent, se, sFQN)
	cg.Printf("\ts := %s(fct)\n", fns.buildSelect)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\trows, err := s.Query(ctx, ex)\n")
	cg.Printf("\tif err != nil {\n")
	cg.Printf("\t\treturn nil, err\n")
	cg.Printf("\t}\n")
	cg.Printf("\treturn %s(rows, fct)\n", fns.scanMult)
	cg.Printf("}\n\n")
}

func (g *generator) generateStructInsertOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, v *%s, setFct, retFct %s, f func(*%s.Insert)) error {\n", fns.insertOne, ctx, se, sFQN, g.facetTypeIdent, se)
	cg.Printf("\ts := %s(v, setFct, retFct)\n", fns.buildInsert)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\trow := s.QueryRow(ctx, ex)\n")
	cg.Printf("\treturn %s(row, v, retFct)\n", fns.scanOne)
	cg.Printf("}\n\n")
}

func (g *generator) generateStructUpdateOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, v *%s, condFct, updateFct, retFct %s, f func(*%s.Update)) error {\n", fns.updateOne, ctx, se, sFQN, g.facetTypeIdent, se)
	cg.Printf("\ts := %s(v, condFct, updateFct, retFct)\n", fns.buildUpdate)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\trow := s.QueryRow(ctx, ex)\n")
	cg.Printf("\treturn %s(row, v, retFct)\n", fns.scanOne)
	cg.Printf("}\n\n")
}

func (g *generator) generateStructDeleteOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, v *%s, condFct %s, f func(*%s.Delete)) error {\n", fns.deleteOne, ctx, se, sFQN, g.facetTypeIdent, se)
	cg.Printf("\ts := %s(v, condFct)\n", fns.buildDelete)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\t_, err := s.Exec(ctx, ex)\n")
	cg.Printf("\treturn err\n")
	cg.Printf("}\n\n")
}

func filterExprs(exprs []*WrappedExpr, cols []*Col) []*WrappedExpr {
	var filtered []*WrappedExpr
	for _, col := range cols {
		filtered = append(filtered, exprs[col.IndexInStruct])
	}
	return filtered
}

func generateFacetSwitch(cg *codeGen, s *Struct, fctVar string, f func(fct *Facet, cols []*Col)) {
	cg.Printf("\tswitch %s {\n", fctVar)
	for _, fct := range s.Facets {
		cg.Printf("\tcase %s:\n", fct.Ident)

		var cols []*Col
		for _, col := range s.Cols {
			if col.IncludedInFacet(fct) {
				cols = append(cols, col)
			}
		}

		f(fct, cols)
	}
	cg.Printf("\tdefault:\n")
	cg.Printf("\t\tpanic(%q+fct.String())\n", fmt.Sprintf("%s does not support facet ", s.Name))
	cg.Printf("\t}\n")
}

func makeScanners(sVar string, s *Struct) []*WrappedExpr {
	var exprs []*WrappedExpr
	for _, col := range s.Cols {
		expr := makeScanner(sVar+"."+col.FieldName, col.TempVarName, col)
		exprs = append(exprs, expr)
	}
	return exprs
}

func makeScanner(dest string, temp string, col *Col) *WrappedExpr {
	if col.Nullable && !isInherentlyNullable(col.Type) {
		// TODO: handle nullable
	}
	return &WrappedExpr{
		Value: "&" + dest,
	}
}

func makeEncoders(sVar string, s *Struct) []*WrappedExpr {
	var exprs []*WrappedExpr
	for _, col := range s.Cols {
		expr := makeEncoder(sVar+"."+col.FieldName, col.TempVarName, col)
		exprs = append(exprs, expr)
	}
	return exprs
}

func makeEncoder(src string, temp string, col *Col) *WrappedExpr {
	return &WrappedExpr{
		Value: src,
	}
}

func isInherentlyNullable(typ Type) bool {
	switch typ.(type) {
	case PtrType:
		return true
	default:
		return false
	}
}

type Facet struct {
	Name  string
	Ident string
	Value int
}

type Type interface {
	TypeName() Name
}

type WellKnownPrimitive struct {
	Name      Name
	ZeroValue string
}

func (t WellKnownPrimitive) TypeName() Name {
	return t.Name
}

type UnsupportedType struct {
	Name Name
}

func (t UnsupportedType) TypeName() Name {
	return t.Name
}

type AliasType struct {
	Name    Name
	AliasOf Type
}

func (t AliasType) TypeName() Name {
	return t.Name
}

type WellKnownStruct struct {
	Name Name

	IsZeroMethod string
	EqualMethod  string
}

func (t WellKnownStruct) TypeName() Name {
	return t.Name
}

type UnknownType Name

func (u UnknownType) TypeName() Name {
	return Name(u)
}

type PtrType struct {
	To Type
}

func (t PtrType) TypeName() Name {
	panic("no type name of ptr type")
}

type Struct struct {
	Persistent bool

	Name Name

	TableName     string
	SingularIdent string
	PluralIdent   string

	Cols []*Col

	Facets []*Facet
}

func (s *Struct) TypeName() Name {
	return s.Name
}

type Col struct {
	IndexInStruct int

	DBName    string
	FieldName string
	Facets    []*Facet
	Type      Type

	Nullable  bool
	Immutable bool

	TempVarName string
}

func (c *Col) IncludedInFacet(f *Facet) bool {
	for _, cf := range c.Facets {
		if cf == f {
			return true
		}
	}
	return false
}

type Name struct {
	Pkg   string
	Ident string
}

func (n Name) Equal(o Name) bool {
	return n.Pkg == o.Pkg && n.Ident == o.Ident
}

func (n Name) FQN() string {
	if n.Pkg == "" {
		return n.Ident
	} else {
		return n.Pkg + "." + n.Ident
	}
}

func (n Name) String() string {
	return n.FQN()
}

type WrappedExpr struct {
	Vars   []string
	Before []string
	Value  string
	DBExpr string
	After  []string
}

func appendBefore(cg *codeGen, exprs []*WrappedExpr) {
	for _, expr := range exprs {
		for _, s := range expr.Vars {
			cg.Printf("\t\tvar %s\n", s)
		}
		for _, s := range expr.Before {
			cg.Printf("\t\t%s\n", s)
		}
	}
}

func appendAfter(cg *codeGen, exprs []*WrappedExpr) {
	for _, expr := range exprs {
		for _, s := range expr.After {
			cg.Printf("\t\t%s\n", s)
		}
	}
}

type codeGen struct {
	pkgName          string
	importBuf        bytes.Buffer
	importAliasByPkg map[string]string

	Buf bytes.Buffer
}

func newCodeGen(pkgName string) *codeGen {
	return &codeGen{
		pkgName:          pkgName,
		importAliasByPkg: make(map[string]string),
	}
}

func (cg *codeGen) imported(pkg string) string {
	if alias, ok := cg.importAliasByPkg[pkg]; ok {
		return alias
	}

	alias := filepath.Base(pkg)
	cg.importAliasByPkg[pkg] = alias
	fmt.Fprintf(&cg.importBuf, "\t%q\n", pkg)
	return alias
}

func (cg *codeGen) qualified(n Name) string {
	if n.Pkg == "" {
		return n.Ident
	} else {
		return cg.imported(n.Pkg) + "." + n.Ident
	}
}

func (cg *codeGen) Printf(format string, a ...interface{}) {
	fmt.Fprintf(&cg.Buf, format, a...)
}

func (cg *codeGen) Bytes() []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "// Code generated by github.com/andreyvit/sqlstruct. DO NOT EDIT. (@generated)\n")
	fmt.Fprintf(&buf, "package %s\n\n", cg.pkgName)
	if cg.importBuf.Len() > 0 {
		fmt.Fprintf(&buf, "import (\n")
		cg.importBuf.WriteTo(&buf)
		fmt.Fprintf(&buf, ")\n")
	}
	cg.Buf.WriteTo(&buf)
	return buf.Bytes()
}
