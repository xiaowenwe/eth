// Copyright 2016 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package bind generates Ethereum contract Go bindings.
//
// Detailed usage document and tutorial available on the go-ethereum Wiki page:
// https://github.com/ethereum/go-ethereum/wiki/Native-DApps:-Go-bindings-to-Ethereum-contracts
package bind

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"golang.org/x/tools/imports"
)

// Lang是一个生成绑定的目标编程语言选择器。
// Lang is a target programming language selector to generate bindings for.
type Lang int

const (
	LangGo Lang = iota
	LangJava
	LangObjC
)

//绑定在合约ABI周围生成一个Go包装器。 这个包装器并不意味着
//在客户端代码中按原样使用，而是作为中间结构使用
//强制编译时类型安全和命名约定，而不是必须
//手动维护在运行时中断的硬编码字符串。
// Bind generates a Go wrapper around a contract ABI. This wrapper isn't meant
// to be used as is in client code, but rather as an intermediate struct which
// enforces compile time type safety and naming convention opposed to having to
// manually maintain hard coded strings that break on runtime.
func Bind(types []string, abis []string, bytecodes []string, pkg string, lang Lang) (string, error) {
	// Process each individual contract requested binding//处理每个单独的合同请求绑定
	contracts := make(map[string]*tmplContract)

	for i := 0; i < len(types); i++ {
		// Parse the actual ABI to generate the binding for//处理每个单独的合同请求绑定
		evmABI, err := abi.JSON(strings.NewReader(abis[i]))
		if err != nil {
			return "", err
		}
		// Strip any whitespace from the JSON ABI//从JSON ABI中删除任何空格
		strippedABI := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, abis[i])

		// Extract the call and transact methods; events; and sort them alphabetically//提取调用和交易方法;事件; 并按字母顺序排序
		var (
			calls     = make(map[string]*tmplMethod)
			transacts = make(map[string]*tmplMethod)
			events    = make(map[string]*tmplEvent)
		)
		for _, original := range evmABI.Methods {
			// Normalize the method for capital cases and non-anonymous inputs/outputs//规范化大写案例和非匿名输入/输出的方法
			normalized := original
			normalized.Name = methodNormalizer[lang](original.Name)

			normalized.Inputs = make([]abi.Argument, len(original.Inputs))
			copy(normalized.Inputs, original.Inputs)
			for j, input := range normalized.Inputs {
				if input.Name == "" {
					normalized.Inputs[j].Name = fmt.Sprintf("arg%d", j)
				}
			}
			normalized.Outputs = make([]abi.Argument, len(original.Outputs))
			copy(normalized.Outputs, original.Outputs)
			for j, output := range normalized.Outputs {
				if output.Name != "" {
					normalized.Outputs[j].Name = capitalise(output.Name)
				}
			}
			// Append the methods to the call or transact lists//将方法附加到调用或交易列表
			if original.Const {
				calls[original.Name] = &tmplMethod{Original: original, Normalized: normalized, Structured: structured(original.Outputs)}
			} else {
				transacts[original.Name] = &tmplMethod{Original: original, Normalized: normalized, Structured: structured(original.Outputs)}
			}
		}
		for _, original := range evmABI.Events {
			// Skip anonymous events as they don't support explicit filtering//跳过匿名事件，因为它们不支持显式过滤
			if original.Anonymous {
				continue
			}
			// Normalize the event for capital cases and non-anonymous outputs//为大写案例和非匿名输出规范化事件
			normalized := original
			normalized.Name = methodNormalizer[lang](original.Name)

			normalized.Inputs = make([]abi.Argument, len(original.Inputs))
			copy(normalized.Inputs, original.Inputs)
			for j, input := range normalized.Inputs {
				// Indexed fields are input, non-indexed ones are outputs//索引字段是输入，非索引字段是输出
				if input.Indexed {
					if input.Name == "" {
						normalized.Inputs[j].Name = fmt.Sprintf("arg%d", j)
					}
				}
			}
			// Append the event to the accumulator list//将事件附加到累加器列表
			events[original.Name] = &tmplEvent{Original: original, Normalized: normalized}
		}
		contracts[types[i]] = &tmplContract{
			Type:        capitalise(types[i]),
			InputABI:    strings.Replace(strippedABI, "\"", "\\\"", -1),
			InputBin:    strings.TrimSpace(bytecodes[i]),
			Constructor: evmABI.Constructor,
			Calls:       calls,
			Transacts:   transacts,
			Events:      events,
		}
	}
	// Generate the contract template data content and render it//生成合同模板数据内容并进行渲染
	data := &tmplData{
		Package:   pkg,
		Contracts: contracts,
	}
	buffer := new(bytes.Buffer)

	funcs := map[string]interface{}{
		"bindtype":      bindType[lang],
		"bindtopictype": bindTopicType[lang],
		"namedtype":     namedType[lang],
		"capitalise":    capitalise,
		"decapitalise":  decapitalise,
	}
	tmpl := template.Must(template.New("").Funcs(funcs).Parse(tmplSource[lang]))
	if err := tmpl.Execute(buffer, data); err != nil { //把数据按模板解析到buffer里
		return "", err
	} //对于Go绑定，通过goimports传递代码来清理它并仔细检查
	// For Go bindings pass the code through goimports to clean it up and double check
	if lang == LangGo {
		code, err := imports.Process(".", buffer.Bytes(), nil)
		if err != nil {
			return "", fmt.Errorf("%v\n%s", err, buffer)
		}
		return string(code), nil
	} //对于其他人来说，现在就回归
	// For all others just return as is for now
	return buffer.String(), nil
}

// bindType是一组类型绑定器，它将Solidity类型转换为某些受支持的编程语言类型。
// bindType is a set of type binders that convert Solidity types to some supported
// programming language types.
var bindType = map[Lang]func(kind abi.Type) string{
	LangGo:   bindTypeGo,
	LangJava: bindTypeJava,
}

//绑定生成器的辅助函数。
//它在内部类型匹配后读取不匹配的字符，
//（因为内部类型是总类型声明的前缀），
//查找包装内部类型的有效数组（可能是动态数组），并返回这些数组的大小。
//返回的数组大小与实体签名的顺序相同; 内部数组大小首先。
//数组大小也可以是“”，表示动态数组。
// Helper function for the binding generators.
// It reads the unmatched characters after the inner type-match,
//  (since the inner type is a prefix of the total type declaration),
//  looks for valid arrays (possibly a dynamic one) wrapping the inner type,
//  and returns the sizes of these arrays.
//
// Returned array sizes are in the same order as solidity signatures; inner array size first.
// Array sizes may also be "", indicating a dynamic array.
func wrapArray(stringKind string, innerLen int, innerMapping string) (string, []string) {
	remainder := stringKind[innerLen:]
	//find all the sizes//找到所有尺寸
	matches := regexp.MustCompile(`\[(\d*)\]`).FindAllStringSubmatch(remainder, -1)
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		//get group 1 from the regex match//从正则表达式匹配中获取组1
		parts = append(parts, match[1])
	}
	return innerMapping, parts
}

//将数组大小转换为内部类型（嵌套）数组的Go-lang声明。
//如果arraySizes为空，只需返回内部类型。
// Translates the array sizes to a Go-lang declaration of a (nested) array of the inner type.
// Simply returns the inner type if arraySizes is empty.
func arrayBindingGo(inner string, arraySizes []string) string {
	out := ""
	//prepend all array sizes, from outer (end arraySizes) to inner (start arraySizes)//前置所有数组大小，从outer（end arraySizes）到inner（start arraySizes）
	for i := len(arraySizes) - 1; i >= 0; i-- {
		out += "[" + arraySizes[i] + "]"
	}
	out += inner
	return out
}

// bindTypeGo将Solidity类型转换为Go one类型。 由于没有明确的映射
//从所有Solidity类型到Go（例如uint17），那些无法精确映射的类型将使用放大类型（例如* big.Int）。
// bindTypeGo converts a Solidity type to a Go one. Since there is no clear mapping
// from all Solidity types to Go ones (e.g. uint17), those that cannot be exactly
// mapped will use an upscaled type (e.g. *big.Int).
func bindTypeGo(kind abi.Type) string {
	stringKind := kind.String()
	innerLen, innerMapping := bindUnnestedTypeGo(stringKind)
	return arrayBindingGo(wrapArray(stringKind, innerLen, innerMapping))
}

// bindTypeGo的内部函数，它找到stringKind的内部类型。
//（或者只是类型本身，如果它不是数组或切片）
//返回匹配部分的长度，以及已翻译的类型。
// The inner function of bindTypeGo, this finds the inner type of stringKind.
// (Or just the type itself if it is not an array or slice)
// The length of the matched part is returned, with the the translated type.
func bindUnnestedTypeGo(stringKind string) (int, string) {

	switch {
	case strings.HasPrefix(stringKind, "address"):
		return len("address"), "common.Address"

	case strings.HasPrefix(stringKind, "bytes"):
		parts := regexp.MustCompile(`bytes([0-9]*)`).FindStringSubmatch(stringKind)
		return len(parts[0]), fmt.Sprintf("[%s]byte", parts[1])

	case strings.HasPrefix(stringKind, "int") || strings.HasPrefix(stringKind, "uint"):
		parts := regexp.MustCompile(`(u)?int([0-9]*)`).FindStringSubmatch(stringKind)
		switch parts[2] {
		case "8", "16", "32", "64":
			return len(parts[0]), fmt.Sprintf("%sint%s", parts[1], parts[2])
		}
		return len(parts[0]), "*big.Int"

	case strings.HasPrefix(stringKind, "bool"):
		return len("bool"), "bool"

	case strings.HasPrefix(stringKind, "string"):
		return len("string"), "string"

	default:
		return len(stringKind), stringKind
	}
}

//将数组大小转换为内部类型（嵌套）数组的Java声明。
//如果arraySizes为空，只需返回内部类型。
// Translates the array sizes to a Java declaration of a (nested) array of the inner type.
// Simply returns the inner type if arraySizes is empty.
func arrayBindingJava(inner string, arraySizes []string) string {
	// Java array type declarations do not include the length.// Java数组类型声明不包括长度。
	return inner + strings.Repeat("[]", len(arraySizes))
}

// bindTypeJava将Solidity类型转换为Java类型。 由于没有从所有Solidity类型到Java（例如uint17）的清晰映射，因此不能完全映射 mapped将使用一个放大的类型（例如BigDecimal）。
// bindTypeJava converts a Solidity type to a Java one. Since there is no clear mapping
// from all Solidity types to Java ones (e.g. uint17), those that cannot be exactly
// mapped will use an upscaled type (e.g. BigDecimal).
func bindTypeJava(kind abi.Type) string {
	stringKind := kind.String()
	innerLen, innerMapping := bindUnnestedTypeJava(stringKind)
	return arrayBindingJava(wrapArray(stringKind, innerLen, innerMapping))
}

// bindTypeJava的内部函数，它查找stringKind的内部类型。
//（或者只是类型本身，如果它不是数组或切片）
//返回匹配部分的长度，以及已翻译的类型。
// The inner function of bindTypeJava, this finds the inner type of stringKind.
// (Or just the type itself if it is not an array or slice)
// The length of the matched part is returned, with the the translated type.
func bindUnnestedTypeJava(stringKind string) (int, string) {

	switch {
	case strings.HasPrefix(stringKind, "address"):
		parts := regexp.MustCompile(`address(\[[0-9]*\])?`).FindStringSubmatch(stringKind)
		if len(parts) != 2 {
			return len(stringKind), stringKind
		}
		if parts[1] == "" {
			return len("address"), "Address"
		}
		return len(parts[0]), "Addresses"

	case strings.HasPrefix(stringKind, "bytes"):
		parts := regexp.MustCompile(`bytes([0-9]*)`).FindStringSubmatch(stringKind)
		if len(parts) != 2 {
			return len(stringKind), stringKind
		}
		return len(parts[0]), "byte[]"

	case strings.HasPrefix(stringKind, "int") || strings.HasPrefix(stringKind, "uint"):
		//注意uint和int（没有数字）也匹配，
		//这些大小为256，并将转换为BigInt（默认值）。
		//Note that uint and int (without digits) are also matched,
		// these are size 256, and will translate to BigInt (the default).
		parts := regexp.MustCompile(`(u)?int([0-9]*)`).FindStringSubmatch(stringKind)
		if len(parts) != 3 {
			return len(stringKind), stringKind
		}

		namedSize := map[string]string{
			"8":  "byte",
			"16": "short",
			"32": "int",
			"64": "long",
		}[parts[2]]

		//default to BigInt
		if namedSize == "" {
			namedSize = "BigInt"
		}
		return len(parts[0]), namedSize

	case strings.HasPrefix(stringKind, "bool"):
		return len("bool"), "boolean"

	case strings.HasPrefix(stringKind, "string"):
		return len("string"), "String"

	default:
		return len(stringKind), stringKind
	}
}

// bindTopicType是一组类型绑定器，它将Solidity类型转换为某些受支持的编程语言主题类型。
// bindTopicType is a set of type binders that convert Solidity types to some
// supported programming language topic types.
var bindTopicType = map[Lang]func(kind abi.Type) string{
	LangGo:   bindTopicTypeGo,
	LangJava: bindTopicTypeJava,
}

// bindTypeGo将Solidity主题类型转换为Go one。 它与简单类型几乎相同，但动态类型转换为哈希。
// bindTypeGo converts a Solidity topic type to a Go one. It is almost the same
// funcionality as for simple types, but dynamic types get converted to hashes.
func bindTopicTypeGo(kind abi.Type) string {
	bound := bindTypeGo(kind)
	if bound == "string" || bound == "[]byte" {
		bound = "common.Hash"
	}
	return bound
}

// bindTypeGo将Solidity主题类型转换为Java主题类型。 它与简单类型几乎相同，但动态类型转换为哈希。
// bindTypeGo converts a Solidity topic type to a Java one. It is almost the same
// funcionality as for simple types, but dynamic types get converted to hashes.
func bindTopicTypeJava(kind abi.Type) string {
	bound := bindTypeJava(kind)
	if bound == "String" || bound == "Bytes" {
		bound = "Hash"
	}
	return bound
}

// namedType是一组函数，它们将特定于语言的类型转换为我在方法名称中使用的命名版本。
// namedType is a set of functions that transform language specific types to
// named versions that my be used inside method names.
var namedType = map[Lang]func(string, abi.Type) string{
	LangGo:   func(string, abi.Type) string { panic("this shouldn't be needed") },
	LangJava: namedTypeJava,
}

// namedTypeJava将一些原始数据类型转换为可用作方法名称部分的命名变体。
// namedTypeJava converts some primitive data types to named variants that can
// be used as parts of method names.
func namedTypeJava(javaKind string, solKind abi.Type) string {
	switch javaKind {
	case "byte[]":
		return "Binary"
	case "byte[][]":
		return "Binaries"
	case "string":
		return "String"
	case "string[]":
		return "Strings"
	case "boolean":
		return "Bool"
	case "boolean[]":
		return "Bools"
	case "BigInt[]":
		return "BigInts"
	default:
		parts := regexp.MustCompile(`(u)?int([0-9]*)(\[[0-9]*\])?`).FindStringSubmatch(solKind.String())
		if len(parts) != 4 {
			return javaKind
		}
		switch parts[2] {
		case "8", "16", "32", "64":
			if parts[3] == "" {
				return capitalise(fmt.Sprintf("%sint%s", parts[1], parts[2]))
			}
			return capitalise(fmt.Sprintf("%sint%ss", parts[1], parts[2]))

		default:
			return javaKind
		}
	}
}

// methodNormalizer是一个名称转换器，它修改Solidity方法名称以符合目标语言命名概念。
// methodNormalizer is a name transformer that modifies Solidity method names to
// conform to target language naming concentions.
var methodNormalizer = map[Lang]func(string) string{
	LangGo:   capitalise,
	LangJava: decapitalise,
}

// capitalize创建一个以大写字符开头的camel-case字符串。
// capitalise makes a camel-case string which starts with an upper case character.
func capitalise(input string) string {
	for len(input) > 0 && input[0] == '_' {
		input = input[1:]
	}
	if len(input) == 0 {
		return ""
	}
	return toCamelCase(strings.ToUpper(input[:1]) + input[1:])
}

// decapitalise创建一个以小写字符开头的驼峰字符串。
// decapitalise makes a camel-case string which starts with a lower case character.
func decapitalise(input string) string {
	for len(input) > 0 && input[0] == '_' {
		input = input[1:]
	}
	if len(input) == 0 {
		return ""
	}
	return toCamelCase(strings.ToLower(input[:1]) + input[1:])
}

// toCamelCase将得分不足的字符串转换为camel-case字符串
// toCamelCase converts an under-score string to a camel-case string
func toCamelCase(input string) string {
	toupper := false

	result := ""
	for k, v := range input {
		switch {
		case k == 0:
			result = strings.ToUpper(string(input[0]))

		case toupper:
			result += strings.ToUpper(string(v))
			toupper = false

		case v == '_':
			toupper = true

		default:
			result += string(v)
		}
	}
	return result
}

//结构化检查ABI数据类型列表是否具有足够的信息来通过正确的Go结构进行操作，或者是否需要平坦返回。
// structured checks whether a list of ABI data types has enough information to
// operate through a proper Go struct or if flat returns are needed.
func structured(args abi.Arguments) bool {
	if len(args) < 2 {
		return false
	}
	exists := make(map[string]bool)
	for _, out := range args {
		// If the name is anonymous, we can't organize into a struct//如果名称是匿名的，我们无法组织成结构
		if out.Name == "" {
			return false
		} //如果在规范化或碰撞时字段名称为空（var，Var，_var，_Var），我们就无法组织成结构
		// If the field name is empty when normalized or collides (var, Var, _var, _Var),
		// we can't organize into a struct
		field := capitalise(out.Name)
		if field == "" || exists[field] {
			return false
		}
		exists[field] = true
	}
	return true
}
