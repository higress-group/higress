// Copyright (c) 2022 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package types

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/structtag"
	"github.com/pkg/errors"
)

const (
	ArrayPrefix  = "array of "
	MapPrefix    = "map of "
	ObjectSuffix = "object"
)

// IsArray returns true if the given type is an `array of <type>`
func IsArray(typ string) bool {
	return strings.HasPrefix(typ, ArrayPrefix)
}

// GetItemType returns the item type of array, e.g.: array of int -> int
func GetItemType(typ string) string {
	if !IsArray(typ) {
		return typ
	}
	return typ[len(ArrayPrefix):]
}

// IsMap returns true if the given type is a `map of <type>`
func IsMap(typ string) bool {
	return strings.HasPrefix(typ, MapPrefix)
}

// GetValueType returns the value type of map, e.g.: map of int -> int
func GetValueType(typ string) string {
	if !IsMap(typ) {
		return typ
	}
	return typ[len(MapPrefix):]
}

// IsObject returns true if the given type is an `object` or an `array of object`
func IsObject(typ string) bool {
	return strings.HasSuffix(typ, ObjectSuffix)
}

var (
	ErrInvalidModel     = errors.New("invalid model")
	ErrInvalidFieldType = errors.New("invalid field type")
)

type ModelParser struct {
	structs map[string]*astNode

	// alias for a basic type, such as type MyInt int: MyInt -> int
	// TODO(WeixinX): Support alias for package name
	alias map[string]*astNode
}

type Model struct {
	Name   string
	Type   string
	Doc    string
	Tag    string
	Fields []Model
}

type astNode struct {
	name string
	doc  string
	expr ast.Expr
}

func (m *Model) Inspect(f func(model *Model) bool) {
	ctn := f(m)
	if !ctn {
		return
	}

	for _, field := range m.Fields {
		field.Inspect(f)
	}
}

// NewModelParser new a model parser based on the dir where the given model exists
func NewModelParser(dir string) (*ModelParser, error) {
	pkgs, err := walkGoSrc(dir)
	if err != nil {
		return nil, err
	}
	p := &ModelParser{
		structs: make(map[string]*astNode),
		alias:   make(map[string]*astNode),
	}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, decl := range f.Decls {
				x, ok := decl.(*ast.GenDecl)
				if !ok || x.Tok != token.TYPE {
					continue
				}
				for _, spec := range x.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}

					switch t := ts.Type.(type) {
					case *ast.StructType:
						if !t.Struct.IsValid() {
							continue
						}
						s := &astNode{
							name: ts.Name.String(),
							expr: t,
						}
						if pkg.Name != "main" { // ignore main package prefix
							s.name = fmt.Sprintf("%s.%s", pkg.Name, s.name)
						}
						if x.Doc != nil {
							s.doc = x.Doc.Text()
						}
						p.structs[s.name] = s
					case *ast.InterfaceType:
						continue
					default: // for alias, such as `type MyInt int`
						alias := ts.Name.String()
						if pkg.Name != "main" {
							alias = fmt.Sprintf("%s.%s", pkg.Name, alias)
						}
						name, err := p.getModelName(t)
						if err != nil {
							continue
						}
						p.alias[alias] = &astNode{
							name: name,
							expr: t,
						}
					}
				}
			}
		}
	}

	// gets the true type (ast node) of the alias
	for alias := range p.alias {
		n := p.recursiveAlias(alias)
		if n != nil {
			p.alias[alias] = n
		}
	}

	return p, nil
}

func walkGoSrc(dir string) (map[string]*ast.Package, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.Errorf("%q is not a directory", dir)
	}

	fset := token.NewFileSet()
	pkgs := make(map[string]*ast.Package)
	walk := func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}
		tmp, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for k, v := range tmp {
			pkgs[k] = v
		}
		return nil
	}
	if err := filepath.Walk(dir, walk); err != nil {
		return nil, errors.Wrapf(err, "failed to walk path %q", dir)
	}
	return pkgs, nil
}

func (p *ModelParser) recursiveAlias(alias string) *astNode {
	if s, ok := p.structs[alias]; ok {
		return s
	}
	if n, ok := p.alias[alias]; ok {
		if n.name != alias {
			ret := p.recursiveAlias(n.name)
			if ret != nil {
				return ret
			}
		}
		return n
	}
	return nil
}

// GetModel return the specified model
func (p *ModelParser) GetModel(model string) (*Model, error) {
	fields, err := p.parseModelFields(model)
	if err != nil {
		return nil, err
	}

	m := &Model{
		Name:   model,
		Type:   "object",
		Fields: fields,
	}
	m.setDoc(p.structs[model].doc)
	return m, nil
}

func (p *ModelParser) parseModelFields(model string) (fields []Model, err error) {
	var s *astNode
	if _, ok := p.structs[model]; ok {
		s = p.structs[model]
	} else if _, ok = p.alias[model]; ok {
		s = p.alias[model]
	} else {
		return nil, ErrInvalidModel
	}

	st, ok := s.expr.(*ast.StructType)
	if !ok || st.Fields == nil {
		return nil, ErrInvalidModel
	}
	pkgName := ""
	if idx := strings.Index(model, "."); idx != -1 {
		pkgName = model[:idx+1] // pkgName includes "."
	}
	for _, field := range st.Fields.List {
		if skipField(field) {
			continue
		}
		fd := Model{Name: field.Names[0].String()}
		if field.Doc != nil {
			fd.setDoc(field.Doc.Text())
		}
		if field.Tag != nil {
			ignore, err := fd.setTag(field.Tag.Value)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse tag %q of the field %q", field.Tag.Value, fd.Name)
			}
			if ignore {
				continue
			}
		}
		fd.Type, err = p.parseFieldType(pkgName, field.Type)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse type %q of the field %q", gotypes.ExprString(field.Type), fd.Name)
		}
		if IsObject(fd.Type) {
			subModel, err := p.doGetModelName(pkgName, field.Type)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get the sub-model name of the field %q with type %q", fd.Name, gotypes.ExprString(field.Type))
			}
			fd.Fields, err = p.parseModelFields(subModel)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse sub-model of the field %q with type %q", fd.Name, gotypes.ExprString(field.Type))
			}
		}
		fields = append(fields, fd)
	}
	return fields, nil
}

func skipField(field *ast.Field) bool {
	name := field.Names
	return field == nil || name == nil || len(name) < 1 || name[0] == nil || name[0].String() == "_"
}

func (m *Model) setDoc(str string) {
	m.Doc = strings.TrimSpace(str)
}

func (m *Model) setTag(str string) (bool, error) {
	str = strings.Trim(str, "` ")
	if str == "" {
		return false, nil
	}

	ignore := false
	tag, err := structtag.Parse(str)
	if err != nil {
		return false, err
	}
	m.Tag = str
	val, err := tag.Get("yaml")
	if err == nil {
		if val.Name == "-" || val.Name == "" {
			ignore = true
		}
	}
	return ignore, nil
}

func (p *ModelParser) getModelName(typ ast.Expr) (string, error) {
	return p.doGetModelName("", typ)
}

func (p *ModelParser) doGetModelName(pkgName string, typ ast.Expr) (string, error) {
	switch t := typ.(type) {
	case *ast.StarExpr: // *int -> int
		return p.doGetModelName(pkgName, t.X)
	case *ast.ArrayType: // slice or array
		return p.doGetModelName(pkgName, t.Elt)
	case *ast.MapType:
		return p.doGetModelName(pkgName, t.Value)
	case *ast.SelectorExpr: // <pkg_name>.<field_name>
		pkg, ok := t.X.(*ast.Ident)
		if !ok {
			return "", ErrInvalidFieldType
		}
		pName := pkg.Name + "."
		return p.doGetModelName(pName, t.Sel)
	case *ast.Ident:
		return pkgName + t.Name, nil
	default:
		return "", ErrInvalidFieldType
	}
}

func (p *ModelParser) parseFieldType(pkgName string, typ ast.Expr) (string, error) {
	switch t := typ.(type) {
	case *ast.StructType: // nested struct
		return string(JsonTypeObject), nil
	case *ast.StarExpr: // *int -> int
		return p.parseFieldType(pkgName, t.X)
	case *ast.ArrayType: // slice or array
		ret, err := p.parseFieldType(pkgName, t.Elt)
		if err != nil {
			return "", err
		}
		return ArrayPrefix + ret, nil
	case *ast.MapType:
		if keyIdent, ok := t.Key.(*ast.Ident); !ok {
			return "", ErrInvalidFieldType
		} else if keyIdent.Name != "string" {
			return "", ErrInvalidFieldType
		} else if ret, err := p.parseFieldType(pkgName, t.Value); err != nil {
			return "", err
		} else {
			return MapPrefix + ret, nil
		}
	case *ast.SelectorExpr: // <pkg_name>.<field_name>
		pkg, ok := t.X.(*ast.Ident)
		if !ok {
			return "", ErrInvalidFieldType
		}
		pName := pkg.Name + "."
		return p.parseFieldType(pName, t.Sel)
	case *ast.Ident:
		fName := pkgName + t.Name
		if _, ok := p.structs[fName]; ok {
			return string(JsonTypeObject), nil
		}
		if alias, ok := p.alias[fName]; ok {
			return p.parseFieldType(pkgName, alias.expr)
		}
		jsonType, err := convert2JsonType(t.Name)
		return string(jsonType), err
	default:
		return "", ErrInvalidFieldType
	}
}

func convert2JsonType(typ string) (JsonType, error) {
	switch typ {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64":
		return JsonTypeInteger, nil
	case "float32", "float64":
		return JsonTypeNumber, nil
	case "bool":
		return JsonTypeBoolean, nil
	case "string":
		return JsonTypeString, nil
	case "struct":
		return JsonTypeObject, nil
	default:
		return "", ErrInvalidFieldType
	}
}

type JsonType string

const (
	JsonTypeInteger JsonType = "integer"
	JsonTypeNumber  JsonType = "number"
	JsonTypeBoolean JsonType = "boolean"
	JsonTypeString  JsonType = "string"
	JsonTypeObject  JsonType = "object"
	JsonTypeArray   JsonType = "array"
	JsonTypeMap     JsonType = "map"
)
