package renderer

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// declaredShape carries the generated types and conversion source for one declared field.
type declaredShape struct {
	RawType              string
	ObservedType         string
	ComparisonType       string
	ObservedConvert      string
	ComparisonConvert    string
	Definitions          []DeclaredType
	Functions            []string
	ObservedExpression   string
	ComparisonExpression string
}

func toDeclaredShape(spec Type, name string, root bool) *declaredShape {
	shape := &declaredShape{}
	switch spec.Kind {
	case KindString, KindInt64, KindBool, KindDouble, KindBytes:
		shape.RawType = spec.GoType()
		if root {
			rawName := name + "Value"
			shape.Definitions = append(shape.Definitions, DeclaredType{Name: rawName, Definition: shape.RawType})
			shape.RawType = rawName
		}
		shape.ObservedType = "*gcmp.ObservedField[" + shape.RawType + "]"
		shape.ComparisonType = "*gcmp.ComparisonField[" + shape.RawType + "]"
		shape.ObservedConvert = "_sgTo" + name + "ObservedValue"
		shape.ComparisonConvert = "_sgTo" + name + "ComparisonValue"
		comparison := "gcmp.Equal(value)"
		if spec.SkipComparison {
			comparison = fmt.Sprintf("gcmp.Ignore[%s](%q)", shape.RawType, "compare: false")
		}
		if !root {
			shape.ObservedExpression = "gcmp.Observed(%s)"
			shape.ComparisonExpression = "gcmp.Equal(%s)"
			if spec.SkipComparison {
				shape.ComparisonExpression = comparison
			}
			return shape
		}
		shape.Functions = []string{
			fmt.Sprintf("func %s(value %s) %s { return gcmp.Observed(value) }", shape.ObservedConvert, shape.RawType, shape.ObservedType),
			fmt.Sprintf("func %s(value %s) %s { return %s }", shape.ComparisonConvert, shape.RawType, shape.ComparisonType, comparison),
		}
	case KindMessage:
		buildMessageShape(shape, spec, name, root)
	case KindList, KindMap:
		buildCollectionShape(shape, spec, name, root)
	}
	shape.ObservedExpression = shape.ObservedConvert + "(%s)"
	shape.ComparisonExpression = shape.ComparisonConvert + "(%s)"
	return shape
}

func buildMessageShape(shape *declaredShape, spec Type, name string, root bool) {
	rawName := name + "Field"
	if root {
		rawName = name + "Value"
	}
	observedName, comparisonName := name+"ObservedField", name+"ComparisonField"
	shape.RawType = rawName
	if spec.Nullable {
		shape.RawType = "*" + rawName
	}
	shape.ObservedType = "*gcmp.ObservedMessage[" + observedName + "]"
	shape.ComparisonType = "*gcmp.ComparisonMessage[" + comparisonName + "]"
	shape.ObservedConvert, shape.ComparisonConvert = "_sgTo"+name+"ObservedMessage", "_sgTo"+name+"ComparisonMessage"
	var rawFields, observedFields, comparisonFields, observedValues, comparisonValues []string
	used := make(map[string]struct{})
	names := make([]string, 0, len(spec.Fields))
	for fieldName := range spec.Fields {
		names = append(names, fieldName)
	}
	sort.Strings(names)
	for _, fieldName := range names {
		goName := uniqueGoName(fieldName, used)
		if spec.Fields[fieldName].SkipComparison {
			rawType, definitions := toRawDeclaredShape(spec.Fields[fieldName], name+goName)
			shape.Definitions = append(shape.Definitions, definitions...)
			rawFields = append(rawFields, fmt.Sprintf("%s %s `json:%s`", goName, rawType, strconv.Quote(fieldName)))
			continue
		}
		child := toDeclaredShape(spec.Fields[fieldName], name+goName, false)
		appendChildShape(shape, child)
		rawFields = append(rawFields, fmt.Sprintf("%s %s `json:%s`", goName, child.RawType, strconv.Quote(fieldName)))
		observedFields = append(observedFields, goName+" "+child.ObservedType)
		comparisonFields = append(comparisonFields, goName+" "+child.ComparisonType)
		observedValues = append(observedValues, goName+": "+fmt.Sprintf(child.ObservedExpression, "value."+goName))
		comparisonValues = append(comparisonValues, goName+": "+comparisonExpression(child, "value."+goName))
	}
	shape.Definitions = append(shape.Definitions,
		DeclaredType{Name: rawName, Definition: "struct {\n" + strings.Join(rawFields, "\n") + "\n}"},
		DeclaredType{Name: observedName, Definition: "struct {\n" + strings.Join(observedFields, "\n") + "\n}"},
		DeclaredType{Name: comparisonName, Definition: "struct {\n" + strings.Join(comparisonFields, "\n") + "\n}"},
	)
	observedBody := fmt.Sprintf("return gcmp.MessageObserved(\n&%s{\n%s\n},\n)", observedName, strings.Join(observedValues, ",\n")+trailingComma(observedValues))
	comparisonBody := fmt.Sprintf("return gcmp.MessageEqual(\n&%s{\n%s\n},\n)", comparisonName, strings.Join(comparisonValues, ",\n")+trailingComma(comparisonValues))
	if spec.Nullable {
		observedBody = fmt.Sprintf("if value == nil { return gcmp.MessageObserved[%s](nil) }; ", observedName) + observedBody
		comparisonBody = fmt.Sprintf("if value == nil { return gcmp.MessageEqual[%s](nil) }; ", comparisonName) + comparisonBody
	}
	if spec.SkipComparison {
		comparisonBody = fmt.Sprintf("return gcmp.MessageIgnore[%s](%q)", comparisonName, "compare: false")
	}
	shape.Functions = append(shape.Functions,
		fmt.Sprintf("func %s(value %s) %s {\n%s\n}", shape.ObservedConvert, shape.RawType, shape.ObservedType, observedBody),
		fmt.Sprintf("func %s(value %s) %s {\n%s\n}", shape.ComparisonConvert, shape.RawType, shape.ComparisonType, comparisonBody),
	)
}

func buildCollectionShape(shape *declaredShape, spec Type, name string, root bool) {
	childName := name
	if spec.Values.Kind != KindMessage {
		childName += "Item"
	}
	child := toDeclaredShape(*spec.Values, childName, false)
	appendChildShape(shape, child)
	prefix := "[]"
	if spec.Kind == KindMap {
		prefix = "map[string]"
	}
	shape.RawType = prefix + child.RawType
	if root && spec.Kind == KindList {
		rawName := name + "Value"
		shape.Definitions = append(shape.Definitions, DeclaredType{Name: rawName, Definition: shape.RawType})
		shape.RawType = rawName
	}
	if spec.Nullable {
		shape.RawType = "*" + shape.RawType
	}
	shape.ObservedType = prefix + child.ObservedType
	shape.ComparisonType = prefix + child.ComparisonType
	observedContainer, comparisonContainer := shape.ObservedType, shape.ComparisonType
	shape.ObservedConvert, shape.ComparisonConvert = "_sgTo"+name+"ObservedField", "_sgTo"+name+"ComparisonField"
	if spec.Kind == KindList {
		shape.ObservedType = "*gcmp.ObservedList[" + child.ObservedType + "]"
		shape.ComparisonType = "*gcmp.ComparisonList[" + child.ComparisonType + "]"
		shape.ObservedConvert, shape.ComparisonConvert = "_sgTo"+name+"ObservedList", "_sgTo"+name+"ComparisonList"
	}
	source := "value"
	if spec.Nullable {
		source = "(*value)"
	}
	observedBody := collectionConversionBody(observedContainer, fmt.Sprintf(child.ObservedExpression, "item"), source)
	comparisonBody := collectionConversionBody(comparisonContainer, comparisonExpression(child, "item"), source)
	if spec.Kind == KindList {
		observedBody += "; return gcmp.ListObserved(result)"
		compare := "gcmp.ListEqual"
		if spec.IgnoreOrder {
			compare = "gcmp.ListUnordered"
		}
		comparisonBody += "; return " + compare + "(result)"
		if spec.SkipComparison {
			comparisonBody = fmt.Sprintf("return gcmp.ListIgnore[%s](%q)", child.ComparisonType, "compare: false")
		}
	} else {
		observedBody += "; return result"
		comparisonBody += "; return result"
		// A whole ignored map is a leaf; its children do not participate in comparison.
		if spec.SkipComparison {
			shape.ObservedType = "*gcmp.ObservedField[" + shape.RawType + "]"
			shape.ComparisonType = "*gcmp.ComparisonField[" + shape.RawType + "]"
			observedBody = "return gcmp.Observed(value)"
			comparisonBody = fmt.Sprintf("return gcmp.Ignore[%s](%q)", shape.RawType, "compare: false")
		}
	}
	if spec.Nullable && !(spec.Kind == KindMap && spec.SkipComparison) {
		nullObserved := "nil"
		nullComparison := "nil"
		if spec.Kind == KindList {
			nullObserved = "gcmp.ListObserved[" + child.ObservedType + "](nil)"
			nullComparison = "gcmp.ListEqual[" + child.ComparisonType + "](nil)"
		}
		observedBody = nullableCollectionConversionBody(nullObserved, observedBody)
		if !(spec.Kind == KindList && spec.SkipComparison) {
			comparisonBody = nullableCollectionConversionBody(nullComparison, comparisonBody)
		}
	}
	shape.Functions = append(shape.Functions,
		fmt.Sprintf("func %s(value %s) %s {\n%s\n}", shape.ObservedConvert, shape.RawType, shape.ObservedType, observedBody),
		fmt.Sprintf("func %s(value %s) %s {\n%s\n}", shape.ComparisonConvert, shape.RawType, shape.ComparisonType, comparisonBody),
	)
}

// nullableCollectionConversionBody preserves null while converting non-null elements.
func nullableCollectionConversionBody(nullExpression, body string) string {
	return fmt.Sprintf("if value == nil { return %s }; ", nullExpression) + body
}

func collectionConversionBody(containerType, expression, source string) string {
	allocation := "make(" + containerType + ", len(" + source + "))"
	iteration := "key, item"
	if !strings.Contains(expression, "item") {
		iteration = "key"
	}
	return fmt.Sprintf("var result %s; if %s != nil { result = %s }; for %s := range %s { result[key] = %s }", containerType, source, allocation, iteration, source, expression)
}

func appendChildShape(shape, child *declaredShape) {
	shape.Definitions = append(shape.Definitions, child.Definitions...)
	shape.Functions = append(shape.Functions, child.Functions...)
}

func comparisonExpression(shape *declaredShape, value string) string {
	if strings.Contains(shape.ComparisonExpression, "%s") {
		return fmt.Sprintf(shape.ComparisonExpression, value)
	}
	return shape.ComparisonExpression
}

func trailingComma(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return ","
}

// toRawDeclaredShape retains ignored fields in the model value without generating observation helpers.
func toRawDeclaredShape(spec Type, name string) (string, []DeclaredType) {
	switch spec.Kind {
	case KindString, KindInt64, KindBool, KindDouble, KindBytes:
		return spec.GoType(), nil
	case KindMessage:
		names := make([]string, 0, len(spec.Fields))
		for field := range spec.Fields {
			names = append(names, field)
		}
		sort.Strings(names)
		var fields []string
		var definitions []DeclaredType
		used := make(map[string]struct{})
		for _, field := range names {
			goName := uniqueGoName(field, used)
			rawType, child := toRawDeclaredShape(spec.Fields[field], name+goName)
			definitions = append(definitions, child...)
			fields = append(fields, fmt.Sprintf("%s %s `json:%s`", goName, rawType, strconv.Quote(field)))
		}
		rawName := name + "Field"
		definitions = append(definitions, DeclaredType{Name: rawName, Definition: "struct {\n" + strings.Join(fields, "\n") + "\n}"})
		if spec.Nullable {
			return "*" + rawName, definitions
		}
		return rawName, definitions
	case KindList, KindMap:
		childName := name
		if spec.Values.Kind != KindMessage {
			childName += "Item"
		}
		rawType, definitions := toRawDeclaredShape(*spec.Values, childName)
		prefix := "[]"
		if spec.Kind == KindMap {
			prefix = "map[string]"
		}
		if spec.Nullable {
			prefix = "*" + prefix
		}
		return prefix + rawType, definitions
	default:
		return "", nil
	}
}
