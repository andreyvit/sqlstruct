package sqlstruct

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

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
	PkgName string

	Published bool

	Logger  Logger
	Verbose bool
}

type generator struct {
	log Logger

	outPkgName       string
	currentInPkgName string

	published bool
	types     map[string]Type

	nullStrategies map[string]CodingStrategy

	persistentStructs []*Struct

	facets map[string]*Facet

	queryResultLoggerInterface string
	logQueryResultFunc         string
	facetTypeIdent             string
	noneFacetIdent             string
	verifyAffected             string
	errNoneAffected            string
}

func newGenerator(opt Options) *generator {
	g := &generator{
		log:        opt.Logger,
		published:  opt.Published,
		outPkgName: opt.PkgName,

		types:  make(map[string]Type),
		facets: make(map[string]*Facet),

		nullStrategies: make(map[string]CodingStrategy),
	}
	g.initBuiltInTypes()
	return g
}

func GeneratePkg(pkgPatterns []string, opt Options) ([]byte, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes,
	}, pkgPatterns...)
	if err != nil {
		return nil, err
	}

	g := newGenerator(opt)

	for pass := 1; pass <= 2; pass++ {
		for _, pkg := range pkgs {
			for _, file := range pkg.Syntax {
				err := g.processFile(file, pkg.PkgPath, pass)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return g.finalizeAndGenerateCode()
}

func Generate(src []byte, opt Options) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	g := newGenerator(opt)

	for pass := 1; pass <= 2; pass++ {
		err = g.processFile(file, "", pass)
		if err != nil {
			return nil, err
		}
	}

	return g.finalizeAndGenerateCode()
}

func (g *generator) processFile(file *ast.File, pkgPath string, pass int) error {
	if pkgPath == "" {
		pkgPath = file.Name.Name
	}
	if g.outPkgName == "" {
		g.outPkgName = pkgPath
	}
	g.currentInPkgName = pkgPath
	defer func() {
		g.currentInPkgName = ""
	}()

	ourFile := &File{
		imports: make(map[string]string),
	}
	for _, imp := range file.Imports {
		importedPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return fmt.Errorf("cannot process import of imp.Path.Value: %w", err)
		}
		var alias string
		if imp.Name == nil || imp.Name.Name == "." {
			alias = path.Base(importedPath)
		} else {
			alias = imp.Name.Name
		}
		ourFile.imports[alias] = importedPath
	}
	// log.Printf("*** imports in %v: %#v", file.Name.Name, ourFile.imports)

	var err error
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok {
			_ = fn
			// g.log.Printf("Func %s", fn.Name.Name)
		} else if decl, ok := d.(*ast.GenDecl); ok {
			switch decl.Tok {
			case token.TYPE:
				for _, s := range decl.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						name := Name{pkgPath, ts.Name.Name}
						if stru, ok := ts.Type.(*ast.StructType); ok {
							if pass == 1 {
								err = g.preprocessStruct(ourFile, ts, name, stru)
							} else {
								err = g.processStruct(ts, name, stru)
							}
							if err != nil {
								return err
							}
						} else {
							if pass == 1 {
								alias, err := g.resolveType(ts.Type, ourFile)
								if err == nil {
									err = g.processAliasTypeDef(ts, name, alias)
								} else {
									err = g.processUnknownTypeDef(ts, name)
								}
								if err != nil {
									return err
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
	return nil
}

func (g *generator) finalizeAndGenerateCode() ([]byte, error) {
	g.queryResultLoggerInterface = g.pubOrUnpub("QueryResultLogger")
	g.logQueryResultFunc = g.pubOrUnpub("logQueryResult")
	g.facetTypeIdent = g.pubOrUnpub("Facet")
	g.noneFacetIdent = g.pubOrUnpub("NoneFacet")
	g.verifyAffected = g.pubOrUnpub("VerifyAffected")
	g.errNoneAffected = g.pubOrUnpub("ErrNoneAffected")

	cg := newCodeGen(g.outPkgName)
	g.generateCode(cg)

	raw := cg.Bytes()

	formatted, err := format.Source(raw)
	if err != nil {
		return raw, err
	}

	return formatted, nil
}

func (g *generator) initBuiltInTypes() {
	i64 := g.addType(&WellKnownPrimitive{Name: Name{"", "int64"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "int32"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "int16"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "int8"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "uint64"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "uint32"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "uint16"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "uint8"}, ZeroValue: "0"})
	i := g.addType(&WellKnownPrimitive{Name: Name{"", "int"}, ZeroValue: "0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "uint"}, ZeroValue: "0"})
	f64 := g.addType(&WellKnownPrimitive{Name: Name{"", "float64"}, ZeroValue: "0.0"})
	_ = g.addType(&WellKnownPrimitive{Name: Name{"", "float32"}, ZeroValue: "0.0"})
	s := g.addType(&WellKnownPrimitive{Name: Name{"", "string"}, ZeroValue: `""`})
	g.addType(&WellKnownPrimitive{Name: Name{"", "bool"}, ZeroValue: "false"})
	tm := g.addType(&WellKnownStruct{Name: Name{"time", "Time"}, IsZeroMethod: "IsZero", EqualMethod: "Equal"})

	nullt := g.addType(&WellKnownStruct{Name: Name{databaseSQL, "NullString"}, ValidField: "Valid"})
	nullwr := &NullWrapping{Type: nullt, ValueType: s, ValueField: "String", ValidField: "Valid", Next: directAssignment{}}
	g.nullStrategies[s.TypeName().FQN()] = nullwr

	nullt = g.addType(&WellKnownStruct{Name: Name{databaseSQL, "NullInt64"}, ValidField: "Valid"})
	nullwr = &NullWrapping{Type: nullt, ValueType: i64, ValueField: "Int64", ValidField: "Valid", Next: directAssignment{}}
	g.nullStrategies[i64.TypeName().FQN()] = nullwr
	g.nullStrategies[i.TypeName().FQN()] = nullwr

	nullt = g.addType(&WellKnownStruct{Name: Name{databaseSQL, "NullTime"}, ValidField: "Valid"})
	nullwr = &NullWrapping{Type: nullt, ValueType: tm, ValueField: "Time", ValidField: "Valid", Next: directAssignment{}}
	g.nullStrategies[tm.TypeName().FQN()] = nullwr

	nullt = g.addType(&WellKnownStruct{Name: Name{databaseSQL, "NullFloat64"}, ValidField: "Valid"})
	nullwr = &NullWrapping{Type: nullt, ValueType: f64, ValueField: "Float64", ValidField: "Valid", Next: directAssignment{}}
	g.nullStrategies[f64.TypeName().FQN()] = nullwr

}

func (g *generator) addType(t Type) Type {
	fqn := t.TypeName().FQN()
	g.types[fqn] = t
	return t
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
	// g.log.Printf("type %v is an alias of %v", name, aliasOf)
	g.addType(&AliasType{name, aliasOf})
	return nil
}

func (g *generator) processUnknownTypeDef(ts *ast.TypeSpec, name Name) error {
	// g.log.Printf("type %v not a struct", name)
	g.addType(&UnsupportedType{name})
	return nil
}

func (g *generator) preprocessStruct(file *File, ts *ast.TypeSpec, structName Name, stru *ast.StructType) error {
	s := &Struct{
		Name: structName,
		File: file,
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
				case "db:skip":
					s.Skipped = true
				case "db:facet":
					//
				case "db:select_by":
					//
				case "db:value":
					s.IsValue = true
				default:
					return fmt.Errorf("invalid struct %s DB comment line %q", s.Name, line)
				}
			}
			// g.log.Printf("******* comment for %s: %q", s.Name, comment.Text)
		}
		// panic("XXX")
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

	// g.log.Printf("type %s struct", s.Name.Ident)
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
	s.Persistent = (len(s.Cols) > 0) && !s.Skipped
	if !s.Persistent {
		return
	}
}

func (g *generator) finalizeStructWithRefs(s *Struct) {
	g.resolveEmbeds(s)

	s.TableNameConst = g.pubOrUnpub(s.PluralIdent + "Table")

	for _, col := range s.Cols {
		col.AddFacet(g.lookupFacet("allall"))
		if !col.ExcludeFromAll {
			col.AddFacet(g.lookupFacet("all"))
		}
		if col.Immutable {
			col.AddFacet(g.lookupFacet("immutable"))
		} else if !col.ReadOnly {
			col.AddFacet(g.lookupFacet("mutable"))
		}
	}

	for i, col := range s.Cols {
		col.IndexInStruct = i
		col.CodingStg = g.decideCodingStrategy(s, col)
		col.DBNameConst = g.pubOrUnpub(s.SingularIdent + makeIdentFromFieldName(col.FieldName))
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

func makeIdentFromFieldName(fn string) string {
	fn = publishedName(fn)
	fn = strings.ReplaceAll(fn, ".", "")
	return fn
}

func (g *generator) decideCodingStrategy(s *Struct, col *Col) CodingStrategy {
	if !col.Nullable {
		return directAssignment{}
	}
	if col.NullableStrategy != nil {
		return col.NullableStrategy
	}
	if isInherentlyNullable(col.Type) {
		return directAssignment{}
	}
	stg := g.nullStrategies[col.Type.TypeName().FQN()]
	if stg == nil {
		stg = PtrWrapping{col.Type, directAssignment{}}
		// panic(fmt.Sprintf("cannot make %s nullable, TODO impl ptr strategy", col.Type.TypeName().FQN()))
	}
	return stg
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

	if tag == nil || tag.Name == "-" {
		return nil
	}

	typ, err := g.resolveType(field.Type, s.File)
	if err != nil {
		return fmt.Errorf("%s.%s: %w", s.Name, name, err)
	}

	if name == "" {
		name = typ.TypeName().Ident
	}

	col := &Col{
		FieldName: name,
		Type:      typ,
	}
	col.TempVarName = makeSafeTempName(name)

	if tag.Name != "" {
		col.DBName = tag.Name
	} else {
		col.DBName = col.FieldName
	}

	var embedding *Embedding

	for _, opt := range tag.Options {
		if strings.HasPrefix(opt, "#") {
			facetName := strings.TrimPrefix(opt, "#")
			col.AddFacet(g.lookupFacet(facetName))
		} else if p := strings.IndexByte(opt, ':'); p > 0 {
			val := opt[p+1:]
			opt = opt[:p]
			switch opt {
			case "insert":
				switch val {
				case "default":
					col.InsertDefault = true
				case "now":
					col.InsertDefault = true
					// TODO: insert NOW()
				default:
					return fmt.Errorf("%s.%s tag option %q has invalid value %q", s.Name, col.FieldName, opt, val)
				}
			case "null_magic":
				subvals := strings.Split(val, ":")
				switch subvals[0] {
				case "none":
					col.NullableStrategy = directAssignment{}
				default:
					return fmt.Errorf("%s.%s tag option %q specifies invalid strategy %q", s.Name, col.FieldName, opt, subvals[0])
				}
			default:
				return fmt.Errorf("%s.%s tag has invalid option %q:%q", s.Name, col.FieldName, opt, val)
			}
		} else {
			switch opt {
			case "nullable":
				col.Nullable = true
			case "immutable":
				col.Immutable = true
			case "readonly":
				col.ReadOnly = true
			case "extra":
				col.ExcludeFromAll = true
			case "pk":
				col.Immutable = true
				col.AddFacet(g.lookupFacet("pk"))
			case "embed":
				embeddedStruct, ok := typ.(*Struct)
				if !ok {
					return fmt.Errorf("%s.%s: can only embed a struct, got %v", s.Name, col.FieldName, typ.TypeName())
				}
				embedding = &Embedding{
					FieldName: col.FieldName,
					Struct:    embeddedStruct,
					DBPrefix:  tag.Name,
				}
			default:
				return fmt.Errorf("%s.%s tag has invalid option %q", s.Name, col.FieldName, opt)
			}
		}
	}

	if !col.ReadOnly {
		if col.InsertDefault {
			col.AddFacet(g.lookupFacet("insertDefaults"))
		} else {
			col.AddFacet(g.lookupFacet("insert"))
		}
	}

	if embedding == nil {
		g.addCol(s, col)
	} else {
		embedding.Facets = col.Facets
		embedding.Nullable = col.Nullable
		embedding.Immutable = col.Immutable
		s.Embeddings = append(s.Embeddings, embedding)
		// g.log.Printf("field %s embeds %s", name, embedding.Struct.Name)
	}

	return nil
}

func (g *generator) resolveEmbeds(s *Struct) {
	if s.embedsResolved {
		return
	}
	s.embedsResolved = true

	for _, embedding := range s.Embeddings {
		g.processEmbedding(s, embedding)
	}
}

func (g *generator) processEmbedding(s *Struct, embedding *Embedding) {
	g.resolveEmbeds(embedding.Struct)

	for _, ecol := range embedding.Struct.Cols {
		col := &Col{
			DBName:      embedding.DBPrefix + ecol.DBName,
			FieldName:   embedding.FieldName + "." + ecol.FieldName,
			Facets:      append([]*Facet(nil), ecol.Facets...),
			Type:        ecol.Type,
			TempVarName: unpublishedName(embedding.FieldName + publishedName(ecol.TempVarName)),

			Nullable:         ecol.Nullable || embedding.Nullable,
			NullableStrategy: ecol.NullableStrategy,
			Immutable:        ecol.Immutable || embedding.Immutable,
			InsertDefault:    ecol.InsertDefault,
		}
		for _, fct := range embedding.Facets {
			col.AddFacet(fct)
		}
		g.addCol(s, col)
	}
}

func (g *generator) addCol(s *Struct, col *Col) {
	s.Cols = append(s.Cols, col)
	// g.log.Printf("%v.%s type %v", s.Name, col.FieldName, col.Type)
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

func (g *generator) resolveType(t ast.Expr, file *File) (Type, error) {
	switch v := t.(type) {
	case *ast.Ident:
		name := Name{"", v.Name}
		return g.resolveTypeName(name)
	case *ast.SelectorExpr:
		switch v1 := v.X.(type) {
		case *ast.Ident:
			if imported := file.imports[v1.Name]; imported != "" {
				name := Name{imported, v.Sel.Name}
				return g.resolveTypeName(name)
			} else {
				return &UnknownType{v1.Name, v.Sel.Name}, fmt.Errorf("unknown import %#v", v1.Name)
			}
		default:
			return &UnknownType{"!unknown", v.Sel.Name}, fmt.Errorf("unknown selector expr %#v", v.X)
		}
	case *ast.StarExpr:
		typ, err := g.resolveType(v.X, file)
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
		if g.currentInPkgName != "" && n.Pkg == "" {
			if t, err := g.resolveTypeName(Name{g.currentInPkgName, n.Ident}); err == nil {
				return t, nil
			}
		}
		g.log.Printf("Cannot resolve %v", n)
		// g.log.Printf("Cannot resolve %v, known types are %+v", n, g.types)
		return UnknownType(n), fmt.Errorf("unknown type %s", fqn)
	}
}

func (g *generator) generateCode(cg *codeGen) {
	for _, s := range g.persistentStructs {
		g.finalizeStructWithRefs(s)
	}

	g.generateLogging(cg)
	g.generateVerifyAffected(cg)
	g.generateFacetsCode(cg)

	for _, s := range g.persistentStructs {
		g.generateStructCode(cg, s)
	}
}

func (g *generator) generateLogging(cg *codeGen) {
	cg.Printf("type %s interface {\n", g.queryResultLoggerInterface)
	cg.Printf("\tIsLoggingQueryResults() bool\n")
	cg.Printf("\tLogQueryResult(ctx context.Context, method string, query string, args []interface{}, start time.Time, rowCount int, err error, result interface{})\n")
	cg.Printf("}\n\n")
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

func (g *generator) generateVerifyAffected(cg *codeGen) {
	sql := cg.imported(databaseSQL)
	errors := cg.imported("errors")

	cg.Printf("var %s = %s.New(%q)\n\n", g.errNoneAffected, errors, "no rows affected")

	cg.Printf("func %s(res %s.Result, err error) error {\n", g.verifyAffected, sql)
	cg.Printf("\tif err != nil { return err }\n")
	cg.Printf("\taffected, err := res.RowsAffected()\n")
	cg.Printf("\tif err != nil { return err }\n")
	cg.Printf("\tif affected == 0 {\n")
	cg.Printf("\t\treturn %s\n", g.errNoneAffected)
	cg.Printf("\t}\n")
	cg.Printf("\treturn nil\n")
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

	g.generateStructConsts(cg, s)

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

func (g *generator) generateStructConsts(cg *codeGen, s *Struct) {
	se := cg.imported(sqlexpr)
	cg.Printf("const %s %s.Table = %q\n\n", s.TableNameConst, se, s.TableName)
	cg.Printf("const (\n")
	for _, col := range s.Cols {
		cg.Printf("\t%s %s.Column = %q\n", col.DBNameConst, se, col.DBName)
	}
	cg.Printf(")\n\n")
}
func (g *generator) generateStructScanOneNew(cg *codeGen, s *Struct, fn string, scanfn string, argName string, argType Name) {
	cg.Printf("func %s(%s *%s, fct %s) (*%s, error) {\n", fn, argName, cg.qualified(argType), g.facetTypeIdent, cg.qualified(s.Name))
	cg.Printf("\tv := new(%s)\n", cg.qualified(s.Name))
	cg.Printf("\terr := %s(%s, v, fct)\n", scanfn, argName)
	cg.Printf("\tif err != nil { return nil, err }\n")
	cg.Printf("\treturn v, nil\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructScanOne(cg *codeGen, s *Struct, fn string, argName string, argType Name) {
	cg.Printf("func %s(%s *%s, v *%s, fct %s) error {\n", fn, argName, cg.qualified(argType), cg.qualified(s.Name), g.facetTypeIdent)

	scanners := makeScanners(cg, "v", s)

	g.generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		if fct == nil {
			cg.Printf("\t\tpanic(%q)\n", "cannot scan noneFacet!")
			return
		}

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
	g.generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		if fct == nil {
			cg.Printf("\t\treturn\n")
			return
		}

		for _, col := range cols {
			cg.Printf("\t\ts.AddField(%s.Column(%q))\n", se, col.DBName)
		}
	})
	cg.Printf("}\n\n")
}

func (g *generator) generateStructSetters(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)

	encoders := makeEncoders(cg, "v", s)

	cg.Printf("func %s(s %s.Settable, v *%s, fct %s) {\n", fns.addSetters, se, sFQN, g.facetTypeIdent)
	g.generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		if fct == nil {
			cg.Printf("\t\treturn\n")
			return
		}

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

	encoders := makeEncoders(cg, "v", s)

	cg.Printf("func %s(s %s.Whereable, v *%s, fct %s) {\n", fns.addConditions, se, sFQN, g.facetTypeIdent)
	g.generateFacetSwitch(cg, s, "fct", func(fct *Facet, cols []*Col) {
		if fct == nil {
			cg.Printf("\t\treturn\n")
			return
		}

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

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, fct %s, where %s.Expr, f func(*%s.Select)) (*%s, error) {\n", fns.selectOne, ctx, se, g.facetTypeIdent, se, se, sFQN)
	cg.Printf("\tstart := %s.Now()\n", cg.imported("time"))
	cg.Printf("\ts := %s(fct)\n", fns.buildSelect)
	cg.Printf("\tif where != nil { s.AddWhere(where) }\n")
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\tquery, args := %s.Build(s)\n", se)
	cg.Printf("\trow := ex.QueryRowContext(ctx, query, args...)\n")
	cg.Printf("\tresult, err := %s(row, fct)\n", fns.scanOneNew)
	cg.Printf("\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\tlogger.LogQueryResult(ctx, %q, query, args, start, map[bool]int{false: 0, true: 1}[err == nil], err, result)\n", fns.selectOne)
	cg.Printf("\t}\n")
	cg.Printf("\treturn result, err\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructSelectMany(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, fct %s, where %s.Expr, f func(*%s.Select)) ([]*%s, error) {\n", fns.selectMany, ctx, se, g.facetTypeIdent, se, se, sFQN)
	cg.Printf("\tstart := %s.Now()\n", cg.imported("time"))
	cg.Printf("\ts := %s(fct)\n", fns.buildSelect)
	cg.Printf("\tif where != nil { s.AddWhere(where) }\n")
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\tquery, args := %s.Build(s)\n", se)
	cg.Printf("\trows, err := ex.QueryContext(ctx, query, args...)\n")
	cg.Printf("\tif err != nil {\n")
	cg.Printf("\t\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\t\tlogger.LogQueryResult(ctx, %q, query, args, start, 0, err, nil)\n", fns.selectMany)
	cg.Printf("\t\t}\n")
	cg.Printf("\t\treturn nil, err\n")
	cg.Printf("\t}\n")
	cg.Printf("\tresult, err := %s(rows, fct)\n", fns.scanMult)
	cg.Printf("\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\tlogger.LogQueryResult(ctx, %q, query, args, start, len(result), err, result)\n", fns.selectMany)
	cg.Printf("\t}\n")
	cg.Printf("\treturn result, err\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructInsertOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, v *%s, setFct, retFct %s, f func(*%s.Insert)) error {\n", fns.insertOne, ctx, se, sFQN, g.facetTypeIdent, se)
	cg.Printf("\tstart := %s.Now()\n", cg.imported("time"))
	cg.Printf("\ts := %s(v, setFct, retFct)\n", fns.buildInsert)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\tquery, args := %s.Build(s)\n", se)
	cg.Printf("\tif retFct == %s {\n", g.noneFacetIdent)
	cg.Printf("\t\tres, err := ex.ExecContext(ctx, query, args...)\n")
	cg.Printf("\t\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\t\tlogger.LogQueryResult(ctx, %q, query, args, start, -1, err, res)\n", fns.insertOne)
	cg.Printf("\t\t}\n")
	cg.Printf("\t\treturn err\n")
	cg.Printf("\t} else {\n")
	cg.Printf("\t\trow := ex.QueryRowContext(ctx, query, args...)\n")
	cg.Printf("\t\terr := %s(row, v, retFct)\n", fns.scanOne)
	cg.Printf("\t\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\t\tlogger.LogQueryResult(ctx, %q, query, args, start, 1, err, v)\n", fns.insertOne)
	cg.Printf("\t\t}\n")
	cg.Printf("\t\treturn err\n")
	cg.Printf("\t}\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructUpdateOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, v *%s, condFct, updateFct, retFct %s, f func(*%s.Update)) error {\n", fns.updateOne, ctx, se, sFQN, g.facetTypeIdent, se)
	cg.Printf("\tstart := %s.Now()\n", cg.imported("time"))
	cg.Printf("\ts := %s(v, condFct, updateFct, retFct)\n", fns.buildUpdate)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\tquery, args := %s.Build(s)\n", se)
	cg.Printf("\tif retFct == %s {\n", g.noneFacetIdent)
	cg.Printf("\t\tres, err := ex.ExecContext(ctx, query, args...)\n")
	cg.Printf("\t\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\t\tlogger.LogQueryResult(ctx, %q, query, args, start, -1, err, res)\n", fns.updateOne)
	cg.Printf("\t\t}\n")
	cg.Printf("\t\treturn %s(res, err)\n", g.verifyAffected)
	cg.Printf("\t} else {\n")
	cg.Printf("\t\trow := ex.QueryRowContext(ctx, query, args...)\n")
	cg.Printf("\t\terr := %s(row, v, retFct)\n", fns.scanOne)
	cg.Printf("\t\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\t\tlogger.LogQueryResult(ctx, %q, query, args, start, 1, err, v)\n", fns.updateOne)
	cg.Printf("\t\t}\n")
	cg.Printf("\t\treturn err\n")
	cg.Printf("\t}\n")
	cg.Printf("}\n\n")
}

func (g *generator) generateStructDeleteOne(cg *codeGen, s *Struct, fns *funcNames) {
	sFQN := cg.qualified(s.Name)
	se := cg.imported(sqlexpr)
	ctx := cg.imported(contextPkg)

	cg.Printf("func %s(ctx %s.Context, ex %s.Executor, v *%s, condFct %s, f func(*%s.Delete)) error {\n", fns.deleteOne, ctx, se, sFQN, g.facetTypeIdent, se)
	cg.Printf("\tstart := %s.Now()\n", cg.imported("time"))
	cg.Printf("\ts := %s(v, condFct)\n", fns.buildDelete)
	cg.Printf("\tif f != nil { f(s) }\n")
	cg.Printf("\tquery, args := %s.Build(s)\n", se)
	cg.Printf("\tres, err := ex.ExecContext(ctx, query, args...)\n")
	cg.Printf("\tif logger, ok := ex.(%s); ok && logger.IsLoggingQueryResults() {\n", g.queryResultLoggerInterface)
	cg.Printf("\t\tlogger.LogQueryResult(ctx, %q, query, args, start, -1, err, res)\n", fns.deleteOne)
	cg.Printf("\t}\n")
	cg.Printf("\treturn %s(res, err)\n", g.verifyAffected)
	cg.Printf("}\n\n")
}

func filterExprs(exprs []*WrappedExpr, cols []*Col) []*WrappedExpr {
	var filtered []*WrappedExpr
	for _, col := range cols {
		filtered = append(filtered, exprs[col.IndexInStruct])
	}
	return filtered
}

func (g *generator) generateFacetSwitch(cg *codeGen, s *Struct, fctVar string, f func(fct *Facet, cols []*Col)) {
	cg.Printf("\tswitch %s {\n", fctVar)
	cg.Printf("\tcase %s:\n", g.noneFacetIdent)
	f(nil, nil)
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

func makeScanners(imp importer, sVar string, s *Struct) []*WrappedExpr {
	var exprs []*WrappedExpr
	for _, col := range s.Cols {
		expr := col.CodingStg.makeScanner(imp, sVar+"."+col.FieldName, col.TempVarName, col)
		exprs = append(exprs, expr)
	}
	return exprs
}

func makeEncoders(imp importer, sVar string, s *Struct) []*WrappedExpr {
	var exprs []*WrappedExpr
	for _, col := range s.Cols {
		expr := col.CodingStg.makeEncoder(imp, sVar+"."+col.FieldName, col.TempVarName, col)
		exprs = append(exprs, expr)
	}
	return exprs
}

func isInherentlyNullable(typ Type) bool {
	switch typ.(type) {
	case PtrType:
		return true
	default:
		return false
	}
}

type CodingStrategy interface {
	makeScanner(imp importer, dest string, temp string, col *Col) *WrappedExpr
	makeEncoder(imp importer, src string, temp string, col *Col) *WrappedExpr
}

type directAssignment struct{}

func (e directAssignment) makeScanner(imp importer, dest string, temp string, col *Col) *WrappedExpr {
	return &WrappedExpr{
		Value: "&" + dest,
	}
}

func (e directAssignment) makeEncoder(imp importer, src string, temp string, col *Col) *WrappedExpr {
	return &WrappedExpr{
		Value: src,
	}
}

type NullWrapping struct {
	Type       Type
	ValueType  Type
	ValidField string
	ValueField string
	Next       CodingStrategy
}

func (e NullWrapping) makeScanner(imp importer, dest string, temp string, col *Col) *WrappedExpr {
	subwe := e.Next.makeScanner(imp, temp, temp+"1", col)

	we := &WrappedExpr{
		DBExpr: subwe.DBExpr,
		Value:  "&" + temp,
	}
	we.Vars = append(we.Vars, fmt.Sprintf("%s %s", temp, imp.qualified(e.Type.TypeName())))
	we.Append(subwe)

	if e.ValueType.String() != col.Type.String() {
		we.After = append(we.After, fmt.Sprintf("%s = %s(%s.%s)", dest, col.Type.String(), temp, e.ValueField))
	} else {
		we.After = append(we.After, fmt.Sprintf("%s = %s.%s", dest, temp, e.ValueField))
	}
	return we
}

func (e NullWrapping) makeEncoder(imp importer, src string, temp string, col *Col) *WrappedExpr {
	subwe := e.Next.makeScanner(imp, temp, temp+"1", col)

	we := &WrappedExpr{
		DBExpr: subwe.DBExpr,
		Value:  temp,
	}
	we.Vars = append(we.Vars, fmt.Sprintf("%s %s", temp, imp.qualified(e.Type.TypeName())))

	var expr string
	if e.ValueType.String() != col.Type.String() {
		expr = fmt.Sprintf("%s(%s)", e.ValueType.String(), src)
	} else {
		expr = src
	}

	check := e.ValueType.MakeZeroValueCheck(imp, expr)

	we.Before = append(we.Before, fmt.Sprintf("if !(%s) {", check))
	we.Before = append(we.Before, fmt.Sprintf("\t%s.%s = %s", temp, e.ValueField, expr))
	we.Before = append(we.Before, fmt.Sprintf("\t%s.%s = true", temp, e.ValidField))
	we.Before = append(we.Before, fmt.Sprintf("}"))

	we.Append(subwe)
	return we
}

type PtrWrapping struct {
	Type Type
	Next CodingStrategy
}

func (e PtrWrapping) makeScanner(imp importer, dest string, temp string, col *Col) *WrappedExpr {
	subwe := e.Next.makeScanner(imp, temp, temp+"1", col)

	we := &WrappedExpr{
		DBExpr: subwe.DBExpr,
		Value:  "&" + temp,
	}
	we.Vars = append(we.Vars, fmt.Sprintf("%s *%s", temp, imp.qualified(e.Type.TypeName())))
	we.Append(subwe)

	we.After = append(we.After,
		fmt.Sprintf("if %s != nil {", temp),
		fmt.Sprintf("\t%s = *%s", dest, temp),
		"}")
	return we
}

func (e PtrWrapping) makeEncoder(imp importer, src string, temp string, col *Col) *WrappedExpr {
	subwe := e.Next.makeScanner(imp, temp, temp+"1", col)

	we := &WrappedExpr{
		DBExpr: subwe.DBExpr,
		Value:  temp,
	}
	we.Vars = append(we.Vars, fmt.Sprintf("%s *%s", temp, imp.qualified(e.Type.TypeName())))

	check := e.Type.MakeZeroValueCheck(imp, src)
	if check == "" {
		check = fmt.Sprintf("/* type %s does not support zero value checking */", e.Type.TypeName())
	}

	we.Before = append(we.Before, fmt.Sprintf("if !(%s) {", check))
	we.Before = append(we.Before, fmt.Sprintf("\t%s = &%s", temp, src))
	we.Before = append(we.Before, fmt.Sprintf("}"))

	we.Append(subwe)
	return we
}

type Facet struct {
	Name  string
	Ident string
	Value int
}

type Type interface {
	TypeName() Name
	MakeZeroValue(imp importer) string
	// Returns empty string to indicate failure
	MakeZeroValueCheck(imp importer, v string) string
	fmt.Stringer
}

type WellKnownPrimitive struct {
	Name      Name
	ZeroValue string
}

func (t WellKnownPrimitive) String() string {
	return t.Name.String()
}

func (t WellKnownPrimitive) MakeZeroValue(imp importer) string {
	return t.ZeroValue
}

func (t WellKnownPrimitive) MakeZeroValueCheck(imp importer, v string) string {
	return fmt.Sprintf("%s == %s", v, t.ZeroValue)
}

func (t WellKnownPrimitive) TypeName() Name {
	return t.Name
}

type UnsupportedType struct {
	Name Name
}

func (t UnsupportedType) String() string {
	return t.Name.String()
}

func (t UnsupportedType) MakeZeroValue(imp importer) string {
	return imp.qualified(t.Name) + "{}"
}

func (t UnsupportedType) MakeZeroValueCheck(imp importer, v string) string {
	return ""
}

func (t UnsupportedType) TypeName() Name {
	return t.Name
}

type AliasType struct {
	Name    Name
	AliasOf Type
}

func (t AliasType) String() string {
	return t.Name.String() + "=" + t.AliasOf.String()
}

func (t AliasType) MakeZeroValue(imp importer) string {
	return imp.qualified(t.Name) + "(" + t.AliasOf.MakeZeroValue(imp) + ")"
}

func (t AliasType) MakeZeroValueCheck(imp importer, v string) string {
	return t.AliasOf.MakeZeroValueCheck(imp, v)
}

func (t AliasType) TypeName() Name {
	return t.Name
}

type WellKnownStruct struct {
	Name Name

	IsZeroMethod string
	ValidField   string
	EqualMethod  string
}

func (t WellKnownStruct) String() string {
	return t.Name.String()
}

func (t WellKnownStruct) MakeZeroValue(imp importer) string {
	return imp.qualified(t.Name) + "{}"
}

func (t WellKnownStruct) MakeZeroValueCheck(imp importer, v string) string {
	if t.IsZeroMethod != "" {
		return fmt.Sprintf("%s.%s()", v, t.IsZeroMethod)
	} else if t.ValidField != "" {
		return fmt.Sprintf("!%s.%s", v, t.ValidField)
	}
	return ""
}

func (t WellKnownStruct) TypeName() Name {
	return t.Name
}

type UnknownType Name

func (t UnknownType) String() string {
	return Name(t).String()
}

func (t UnknownType) MakeZeroValue(imp importer) string {
	panic("cannot make zero value of unknown type")
}

func (t UnknownType) MakeZeroValueCheck(imp importer, v string) string {
	return ""
}

func (u UnknownType) TypeName() Name {
	return Name(u)
}

type PtrType struct {
	To Type
}

func (t PtrType) String() string {
	return "*" + t.To.String()
}

func (t PtrType) MakeZeroValue(imp importer) string {
	return "nil"
}

func (t PtrType) MakeZeroValueCheck(imp importer, v string) string {
	return fmt.Sprintf("%s == nil", v)
}

func (t PtrType) TypeName() Name {
	panic("no type name of ptr type")
}

type File struct {
	imports map[string]string
}

type Struct struct {
	Persistent bool
	IsValue    bool
	Skipped    bool

	Name Name
	File *File

	TableName      string
	TableNameConst string
	SingularIdent  string
	PluralIdent    string

	Cols []*Col

	Embeddings     []*Embedding
	embedsResolved bool

	Facets []*Facet
}

func (s *Struct) String() string {
	return s.Name.String() + "{}"
}

func (s *Struct) MakeZeroValue(imp importer) string {
	return imp.qualified(s.Name) + "{}"
}

func (s *Struct) MakeZeroValueCheck(imp importer, v string) string {
	return fmt.Sprintf("%s.IsZero()", v)
}

func (s *Struct) TypeName() Name {
	return s.Name
}

type Col struct {
	IndexInStruct int

	DBNameConst string
	DBName      string
	FieldName   string
	Facets      []*Facet
	Type        Type
	TempVarName string
	CodingStg   CodingStrategy

	ReadOnly         bool
	ExcludeFromAll   bool
	Nullable         bool
	NullableStrategy CodingStrategy
	Immutable        bool
	InsertDefault    bool
}

func (c *Col) HasFacet(f *Facet) bool {
	for _, cf := range c.Facets {
		if cf == f {
			return true
		}
	}
	return false
}

func (c *Col) AddFacet(fct *Facet) {
	if !c.HasFacet(fct) {
		c.Facets = append(c.Facets, fct)
	}
}
func (c *Col) IncludedInFacet(f *Facet) bool {
	for _, cf := range c.Facets {
		if cf == f {
			return true
		}
	}
	return false
}

type Embedding struct {
	FieldName string
	Facets    []*Facet
	Struct    *Struct
	Cols      []*Col
	DBPrefix  string
	Nullable  bool
	Immutable bool
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

func (we *WrappedExpr) Append(another *WrappedExpr) {
	we.Vars = append(we.Vars, another.Vars...)
	we.Before = append(we.Before, another.Before...)
	we.After = append(we.After, another.After...)
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

type importer interface {
	imported(pkg string) string
	qualified(n Name) string
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
	if n.Pkg == "" || n.Pkg == cg.pkgName {
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
