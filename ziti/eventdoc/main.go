//go:build all

package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/openziti/foundation/v2/stringz"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type TypeDef struct {
	baseName    string
	name        string
	doc         string
	fieldNames  []string
	fields      map[string]*typeField
	namespace   string
	isEventType bool
	extraTypes  map[string]struct{}
	isVersioned bool
	version     uint
}

func (self *TypeDef) GetTitle() string {
	return strings.ReplaceAll(self.name, "Event", "")
}

func (self *TypeDef) FormatDoc() {
	leftBraceRegex, err := regexp.Compile(`^\s+\{`)
	if err != nil {
		panic(err)
	}
	rightBraceRegex, err := regexp.Compile(`^\s+[}\]]`)
	if err != nil {
		panic(err)
	}

	result := &bytes.Buffer{}
	scanner := bufio.NewScanner(strings.NewReader(self.doc))

	indent := 0

	detailSummary := ""

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " ")
		if indent > 0 && rightBraceRegex.MatchString(line) {
			indent -= 4
			self.writeIndented(result, indent, line)
			if indent == 0 {
				result.WriteString("```\n</details>\n")
			}
		} else if indent > 0 {
			self.writeIndented(result, indent, line)
			if strings.HasSuffix(line, "{") || strings.HasSuffix(line, "[") {
				indent += 4
			}
		} else if leftBraceRegex.MatchString(line) {
			if indent == 0 {
				result.WriteString("<details>\n")
				result.WriteString("<summary>")
				if detailSummary != "" {
					result.WriteString(detailSummary)
					detailSummary = ""
				} else {
					result.WriteString("Example")
				}
				result.WriteString("</summary>\n")
			}
			result.WriteString("```text\n")
			self.writeIndented(result, indent, line)
			indent += 4
		} else if strings.Contains(line, "Example: ") {
			detailSummary = strings.TrimSpace(line)
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}
	self.doc = result.String()

	for _, field := range self.fields {
		field.doc = strings.ReplaceAll(field.doc, "\n", " ")
	}
}

func (self *TypeDef) writeIndented(buf *bytes.Buffer, indent int, line string) {
	line = strings.TrimSpace(line)
	for range indent {
		buf.WriteString(" ")
	}
	buf.WriteString(line)
	buf.WriteString("\n")
}

func (self *TypeDef) getExtraTypes(v *visitor, m map[string]struct{}) []string {
	for name := range self.extraTypes {
		if _, ok := m[name]; !ok {
			m[name] = struct{}{}
			extraType := v.extraTypes[name]
			extraType.getExtraTypes(v, m)
		}
	}

	var result []string
	for name := range m {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func (self *TypeDef) GetDoc(v *visitor) string {
	buf := &bytes.Buffer{}
	buf.WriteString("## ")
	buf.WriteString(self.GetTitle())
	buf.WriteString("\n\n**Namespace:** `")
	buf.WriteString(self.namespace)
	buf.WriteString("` \n\n")
	buf.WriteString(self.doc)
	buf.WriteString("\n\n*******************\n\n**Fields**\n\n")

	self.AppendFields(buf)

	extraTypes := self.getExtraTypes(v, map[string]struct{}{})
	for _, name := range extraTypes {
		extraType := v.extraTypes[name]
		buf.WriteString("### ")
		buf.WriteString(name)
		buf.WriteString("\n\n")
		buf.WriteString(extraType.doc)
		buf.WriteString("\n\n*******************\n\n**Fields**\n\n")

		extraType.AppendFields(buf)
	}

	return buf.String()
}

//func (self *TypeDef) AppendFields(buf *bytes.Buffer) {
//	for _, name := range self.fieldNames {
//		field := self.fields[name]
//		buf.WriteString(fmt.Sprintf("**`%s`**\n\n", field.jsName))
//		buf.WriteString("The " + field.doc)
//		buf.WriteString("\n\n* Type: ")
//		buf.WriteString(field.jsType)
//		buf.WriteString("\n\n")
//	}
//	buf.WriteString("\n")
//}

func (self *TypeDef) AppendFields(buf *bytes.Buffer) {
	buf.WriteString("| Field | Description | Type |\n")
	buf.WriteString("| ----- | ----------- | ---- |\n")
	for _, name := range self.fieldNames {
		field := self.fields[name]
		buf.WriteString("| **")
		buf.WriteString(field.jsName)
		buf.WriteString("** |")
		buf.WriteString(field.doc)
		buf.WriteString("|")
		buf.WriteString(field.jsType)
		buf.WriteString("|\n")
	}
	buf.WriteString("\n")
}

func (self *TypeDef) HasErrors() bool {
	hasErrors := false
	if self.isEventType {
		if self.namespace == "" {
			_, _ = fmt.Fprintf(os.Stderr, "%s has no namespace\n", self.name)
			hasErrors = true
		}
		if _, ok := self.fields["Timestamp"]; !ok {
			_, _ = fmt.Fprintf(os.Stderr, "%s has no timestamp field\n", self.name)
		}
	}

	if self.doc == "" {
		_, _ = fmt.Fprintf(os.Stderr, "%s has no doc\n", self.name)
		hasErrors = true
	}

	for _, field := range self.fields {
		if len(field.doc) < 10 {
			_, _ = fmt.Fprintf(os.Stderr, "%s.%s has no doc\n", self.name, field.name)
			hasErrors = true
		}
	}

	return hasErrors
}

type typeField struct {
	name   string
	jsType string
	jsName string
	doc    string
}

type visitor struct {
	extraTypes   map[string]*TypeDef
	eventTypes   map[string]*TypeDef
	namespaces   map[string]string
	typeMappings map[string]string
	currentDoc   string
	currentType  *TypeDef
}

func (self *visitor) postProcess() {
	for _, eventDesc := range self.eventTypes {
		for _, fieldDesc := range eventDesc.fields {
			if fieldDesc.name == "Namespace" {
				ns := self.namespaces[eventDesc.baseName]
				eventDesc.namespace = ns
				if fieldDesc.doc == "" {
					fieldDesc.doc = fmt.Sprintf("The event group. The namespace for %ss is %s", eventDesc.name, ns)
				}
			} else if fieldDesc.name == "Timestamp" {
				if fieldDesc.doc == "" {
					fieldDesc.doc = "The datetime that the event was generated"
				}
			} else if fieldDesc.name == "EventSrcId" {
				if fieldDesc.doc == "" {
					fieldDesc.doc = "The identifier of the controller which emitted the event"
				}
			}
			fieldDesc.doc = strings.ReplaceAll(fieldDesc.doc, fieldDesc.name, fieldDesc.jsName)
			fieldDesc.doc = strings.TrimSuffix(fieldDesc.doc, "\n")
		}
	}

	for _, t := range self.eventTypes {
		t.FormatDoc()
	}

	for _, t := range self.extraTypes {
		t.FormatDoc()
	}
}

func (self *visitor) GetTypeMappingsAndAliases(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.ValueSpec:
		self.VisitValueDecl(n)
	case *ast.TypeSpec:
		name := n.Name.String()
		if ident, ok := n.Type.(*ast.Ident); ok {
			self.typeMappings[name] = ident.Name
		}
	}
	return true
}

func (self *visitor) VisitEventTypes(node ast.Node) bool {
	switch n := node.(type) {

	case *ast.GenDecl:
		self.VisitGenDecl(n)
	case *ast.TypeSpec:
		name := n.Name.String()
		baseName, isEvent, version := self.getTypeName(n)
		if !isEvent {
			return false
		}
		if strct, ok := n.Type.(*ast.StructType); ok {
			typeDef := self.getTypeDef(n, strct)
			typeDef.baseName = baseName
			if version != nil {
				typeDef.isVersioned = true
				typeDef.version = *version
			}
			typeDef.isEventType = true
			self.eventTypes[name] = typeDef
		}
		return false
	}
	return true
}

func (self *visitor) getTypeName(node *ast.TypeSpec) (string, bool, *uint) {
	name := node.Name.String()
	re := regexp.MustCompile(`(\w+Event)(V(\d+))?`)
	parts := re.FindStringSubmatch(name)
	if len(parts) == 0 {
		return name, false, nil
	}

	//for x, part := range parts {
	//	_, _ = fmt.Fprintf(os.Stderr, "%d:%s\n", x, part)
	//}

	var version *uint

	if len(parts) == 4 && parts[3] != "" {
		versionNumber, err := strconv.Atoi(parts[3])
		if err != nil {
			panic(err)
		}
		v := uint(versionNumber)
		version = &v
	}

	return parts[1], true, version
}

func (self *visitor) VisitOtherTypesFirstPass(node ast.Node) bool {
	switch n := node.(type) {

	case *ast.TypeSpec:
		name := n.Name.String()
		_, isEvent, _ := self.getTypeName(n)
		if isEvent {
			return false
		}
		if _, ok := n.Type.(*ast.StructType); ok {
			self.extraTypes[name] = nil
		}
	}
	return true
}

func (self *visitor) VisitOtherTypes(node ast.Node) bool {
	switch n := node.(type) {

	case *ast.GenDecl:
		self.VisitGenDecl(n)
	case *ast.TypeSpec:
		name := n.Name.String()
		_, isEvent, _ := self.getTypeName(n)
		if isEvent {
			return false
		}
		if strct, ok := n.Type.(*ast.StructType); ok {
			typeDef := self.getTypeDef(n, strct)
			self.extraTypes[name] = typeDef
		}
	}
	return true
}

func (self *visitor) getTypeDef(n *ast.TypeSpec, strct *ast.StructType) *TypeDef {
	name := n.Name.String()

	typeDef := &TypeDef{
		name:       name,
		fields:     map[string]*typeField{},
		extraTypes: map[string]struct{}{},
	}
	self.currentType = typeDef

	typeDef.doc = self.currentDoc

	// iterate over and append field names, types, and documentation.
	for _, field := range strct.Fields.List {
		if len(field.Names) == 0 {
			// embedded field
			continue
		}
		fieldName := field.Names[0].String()

		jsonName := fieldName
		if field.Tag != nil {
			tags, err := self.parseTags(field.Tag.Value)
			if err != nil {
				fmt.Printf("error parsing tag: %v\n", field.Tag.Value)
				panic(err)
			}
			for _, tag := range tags {
				if tag.key == "json" {
					jsonName = tag.values[0]
				}
			}
		}

		if jsonName == "-" {
			continue
		}

		fieldType := self.getType(field.Type)
		if strings.Contains(fieldType, "unhandled") {
			fmt.Printf("unhandled field type %T (%s) for %s.%s\n", field.Type, fieldType, name, fieldName)
		}

		var fieldDoc string
		if fd := field.Doc; fd != nil {
			fieldDoc = fd.Text()
		}

		typeDef.fieldNames = append(typeDef.fieldNames, fieldName)
		typeDef.fields[fieldName] = &typeField{
			name:   fieldName,
			jsType: fieldType,
			jsName: jsonName,
			doc:    fieldDoc,
		}
	}

	self.currentType = nil

	return typeDef
}

func (self *visitor) getType(goFieldType ast.Expr) string {
	if identType, ok := goFieldType.(*ast.Ident); ok {
		return self.getJsTypeName(identType.Name)
	}

	if selType, ok := goFieldType.(*ast.SelectorExpr); ok {
		return self.getJsTypeName(selType.X.(*ast.Ident).Name + "." + selType.Sel.Name)
	}

	if starType, ok := goFieldType.(*ast.StarExpr); ok {
		return self.getType(starType.X)
	}

	if arrayType, ok := goFieldType.(*ast.ArrayType); ok {
		return "list of " + self.getType(arrayType.Elt)
	}

	if _, ok := goFieldType.(*ast.InterfaceType); ok {
		return "object"
	}

	if mapType, ok := goFieldType.(*ast.MapType); ok {
		return "map of " + self.getType(mapType.Key) + " -> " + self.getType(mapType.Value)
	}

	fmt.Printf("unhandled type: %T\n", goFieldType)
	return "unhandled"
}

func (self *visitor) getJsTypeName(name string) string {
	if alias, ok := self.typeMappings[name]; ok {
		name = alias
	}

	if _, ok := self.extraTypes[name]; ok {
		self.currentType.extraTypes[name] = struct{}{}
		return fmt.Sprintf("[%s](#%s)", name, strings.ToLower(name))
	}

	if name == "string" {
		return "string"
	}
	if name == "bool" {
		return "boolean"
	}

	if stringz.Contains([]string{"uint32", "int32", "uint64", "int64", "uint16", "int16", "int"}, name) {
		return fmt.Sprintf("number (%s)", name)
	}

	if name == "time.Time" {
		return "timestamp"
	}

	if name == "time.Duration" {
		return "duration"
	}

	if name == "any" {
		return "object"
	}

	return fmt.Sprintf("unhandled: %s", name)
}

func (self *visitor) VisitGenDecl(n *ast.GenDecl) {
	if n.Doc != nil {
		self.currentDoc = n.Doc.Text()
	} else {
		self.currentDoc = ""
	}
}

func (self *visitor) VisitValueDecl(node *ast.ValueSpec) {
	self.ExtractNamespace(node)
}

func (self *visitor) ExtractNamespace(node *ast.ValueSpec) {
	for _, nameIdent := range node.Names {
		name := nameIdent.String()
		if strings.HasSuffix(name, "EventNS") {
			if len(node.Values) == 1 {
				v := node.Values[0]
				if l, ok := v.(*ast.BasicLit); ok {
					ns := strings.ReplaceAll(l.Value, `"`, "")
					eventType := strings.TrimSuffix(name, "NS")
					self.namespaces[eventType] = ns
				}
			}
		}
	}
}

func (self *visitor) parseTags(s string) ([]*Tag, error) {
	var result []*Tag
	s = strings.ReplaceAll(s, "`", "")
	tagStrings := strings.Split(s, " ")
	for _, tagString := range tagStrings {
		if strings.TrimSpace(tagString) == "" {
			continue
		}
		parts := strings.Split(tagString, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tag: %s", s)
		}
		key := strings.TrimSpace(parts[0])
		valuesString := strings.ReplaceAll(parts[1], `"`, "")
		values := strings.Split(valuesString, ",")
		result = append(result, &Tag{
			key:    key,
			values: values,
		})
	}
	return result, nil
}

func (self *visitor) validate() bool {
	var hasErrors bool
	extraTypes := make(map[string]struct{})
	for _, typeDef := range self.eventTypes {
		if typeDef.HasErrors() {
			hasErrors = true
		}
		for k := range typeDef.extraTypes {
			extraTypes[k] = struct{}{}
		}
	}

	for k := range extraTypes {
		typeDef := self.extraTypes[k]
		if typeDef.HasErrors() {
			hasErrors = true
		}
	}

	return hasErrors
}

func main() {
	if len(os.Args) < 2 {
		panic(errors.New("GOFILE environment variable not set and no filename passed in"))
	}
	fileName := os.Args[1]

	fileSet := token.NewFileSet()
	pkgMap, err := parser.ParseDir(fileSet, fileName, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	v := &visitor{
		eventTypes:   map[string]*TypeDef{},
		extraTypes:   map[string]*TypeDef{},
		namespaces:   map[string]string{},
		typeMappings: map[string]string{},
	}

	for _, pkg := range pkgMap {
		ast.Inspect(pkg, v.GetTypeMappingsAndAliases)
	}

	for _, pkg := range pkgMap {
		ast.Inspect(pkg, v.VisitOtherTypesFirstPass)
	}

	for _, pkg := range pkgMap {
		ast.Inspect(pkg, v.VisitOtherTypes)
	}

	for _, pkg := range pkgMap {
		ast.Inspect(pkg, v.VisitEventTypes)
	}

	v.postProcess()
	v.validate()

	var eventTypes []string
	for k := range v.eventTypes {
		eventTypes = append(eventTypes, k)
	}
	sort.Strings(eventTypes)

	fmt.Printf(`---
title: Events
---

# Events

## Introduction
The controller can emit many kinds of events, useful for monitoring, management and integration with other systems.
They can be enabled in the controller configuration. 

### Common Fields

All events have the following fields:

| Type | Description | Type |
|------|-------------|----------|
| **namespace** | The name indicating the overall event type | string |
| **timestamp** | The date/time when the event was generated | timestamp |
| **event_src_id** | The id of the controller which emitted the event | string |

### Time Related Types

| Type | Description | Examples |
|------|-------------|----------|
| **timestamp** | RFC3339 formatted timestamp string | "2024-10-02T12:17:39.501821249-04:00" |
| **duration** | Number representing a duration in nanoseconds | 104100 |

## Event Types

`)

	for _, eventType := range eventTypes {
		def := v.eventTypes[eventType]
		title := def.GetTitle()
		fmt.Printf("* [%s](#%s)\n", title, strings.ToLower(title))
	}
	fmt.Printf("\n\n")

	fmt.Printf(`
## Event Configuration

For a complete event configuration reference, please refer to the 
[controller event configuration](configuration/controller#events).

**Note**: Many namespaces changed in OpenZiti v1.4.0. Old namespaces are noted below.

Example Configuration
`)
	fmt.Printf("\n```\n")
	fmt.Printf(`events:
  jsonLogger:
    subscriptions:
`)
	ns := map[string]struct{}{}

	for _, eventType := range v.eventTypes {
		ns[eventType.namespace] = struct{}{}
	}

	for _, eventType := range eventTypes {
		def := v.eventTypes[eventType]
		if _, ok := ns[def.namespace]; ok {
			fmt.Printf("      - %s\n", def.namespace)
			delete(ns, def.namespace)
		}
	}

	fmt.Printf("```\n\n")

	for _, eventType := range eventTypes {
		def := v.eventTypes[eventType]
		fmt.Print(def.GetDoc(v))
		fmt.Println()
	}
}

type Tag struct {
	key    string
	values []string
}
