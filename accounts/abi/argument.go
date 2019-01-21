// Copyright 2015 The go-ethereum Authors
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

package abi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Argument包含参数的名称和相应的类型。
//打包和测试参数时使用类型..
// Argument holds the name of the argument and the corresponding type.
// Types are used when packing and testing arguments.
type Argument struct {
	Name    string
	Type    Type
	Indexed bool // indexed is only used by events indexed 仅用于索引的事件
}

type Arguments []Argument

// UnmarshalJSON实现了json.Unmarshaler接口
// UnmarshalJSON implements json.Unmarshaler interface
func (argument *Argument) UnmarshalJSON(data []byte) error {
	var extarg struct {
		Name    string
		Type    string
		Indexed bool
	}
	err := json.Unmarshal(data, &extarg)
	if err != nil {
		return fmt.Errorf("argument json err: %v", err)
	}

	argument.Type, err = NewType(extarg.Type)
	if err != nil {
		return err
	}
	argument.Name = extarg.Name
	argument.Indexed = extarg.Indexed

	return nil
}

// LengthNonIndexed在不计算“索引”值时返回参数个数。 只有事件可以有“索引”参数，对于方法输入/输出的参数应该始终为false
// LengthNonIndexed returns the number of arguments when not counting 'indexed' ones. Only events
// can ever have 'indexed' arguments, it should always be false on arguments for method input/output
func (arguments Arguments) LengthNonIndexed() int {
	out := 0
	for _, arg := range arguments {
		if !arg.Indexed {
			out++
		}
	}
	return out
}

// NonIndexed返回带有索引参数的参数
// NonIndexed returns the arguments with indexed arguments filtered out
func (arguments Arguments) NonIndexed() Arguments {
	var ret []Argument
	for _, arg := range arguments {
		if !arg.Indexed {
			ret = append(ret, arg)
		}
	}
	return ret
}

// isTuple为非原子结构返回true，如（uint，uint）或uint []
// isTuple returns true for non-atomic constructs, like (uint,uint) or uint[]
func (arguments Arguments) isTuple() bool {
	return len(arguments) > 1
}

//unpack执行十六进制操作->GO格式
// Unpack performs the operation hexdata -> Go format
func (arguments Arguments) Unpack(v interface{}, data []byte) error {

	// make sure the passed value is arguments pointer//确保传递的值是参数指针
	if reflect.Ptr != reflect.ValueOf(v).Kind() {
		return fmt.Errorf("abi: Unpack(non-pointer %T)", v)
	}
	marshalledValues, err := arguments.UnpackValues(data)
	if err != nil {
		return err
	}
	if arguments.isTuple() {
		return arguments.unpackTuple(v, marshalledValues)
	}
	return arguments.unpackAtomic(v, marshalledValues)
}

func (arguments Arguments) unpackTuple(v interface{}, marshalledValues []interface{}) error {

	var (
		value = reflect.ValueOf(v).Elem()
		typ   = value.Type()
		kind  = value.Kind()
	)

	if err := requireUnpackKind(value, typ, kind, arguments); err != nil {
		return err
	}
	// If the output interface is a struct, make sure names don't collide//如果输出接口是结构，请确保名称不会发生冲突
	if kind == reflect.Struct {
		if err := requireUniqueStructFieldNames(arguments); err != nil {
			return err
		}
	}
	for i, arg := range arguments.NonIndexed() {

		reflectValue := reflect.ValueOf(marshalledValues[i])

		switch kind {
		case reflect.Struct:
			err := unpackStruct(value, reflectValue, arg)
			if err != nil {
				return err
			}
		case reflect.Slice, reflect.Array:
			if value.Len() < i {
				return fmt.Errorf("abi: insufficient number of arguments for unpack, want %d, got %d", len(arguments), value.Len())
			}
			v := value.Index(i)
			if err := requireAssignable(v, reflectValue); err != nil {
				return err
			}

			if err := set(v.Elem(), reflectValue, arg); err != nil {
				return err
			}
		default:
			return fmt.Errorf("abi:[2] cannot unmarshal tuple in to %v", typ)
		}
	}
	return nil
}

//解压缩Atomic unpack（十六进制数据 - > to）一个值
// unpackAtomic unpacks ( hexdata -> go ) a single value
func (arguments Arguments) unpackAtomic(v interface{}, marshalledValues []interface{}) error {
	if len(marshalledValues) != 1 {
		return fmt.Errorf("abi: wrong length, expected single value, got %d", len(marshalledValues))
	}
	elem := reflect.ValueOf(v).Elem()
	kind := elem.Kind()
	reflectValue := reflect.ValueOf(marshalledValues[0])

	if kind == reflect.Struct {
		//make sure names don't collide//确保名称不会发生碰撞
		if err := requireUniqueStructFieldNames(arguments); err != nil {
			return err
		}

		return unpackStruct(elem, reflectValue, arguments[0])
	}

	return set(elem, reflectValue, arguments.NonIndexed()[0])

}

//计算数组的完整大小;
//即计算嵌套数组，计算解包的大小。
// Computes the full size of an array;
// i.e. counting nested arrays, which count towards size for unpacking.
func getArraySize(arr *Type) int {
	size := arr.Size
	// Arrays can be nested, with each element being the same size
	arr = arr.Elem
	for arr.T == ArrayTy {
		// Keep multiplying by elem.Size while the elem is an array.
		size *= arr.Size
		arr = arr.Elem
	}
	// Now we have the full array size, including its children.
	return size
}

///UnPackValue可以根据ABI规范解压缩ABI编码的六进制数据，/而不提供要解压缩的结构。相反，此方法返回包含/值的列表。原子参数将是一个包含一个元素的列表。
// UnpackValues can be used to unpack ABI-encoded hexdata according to the ABI-specification,
// without supplying a struct to unpack into. Instead, this method returns a list containing the
// values. An atomic argument will be a list with one element.
func (arguments Arguments) UnpackValues(data []byte) ([]interface{}, error) {
	retval := make([]interface{}, 0, arguments.LengthNonIndexed())
	virtualArgs := 0
	for index, arg := range arguments.NonIndexed() {
		marshalledValue, err := toGoType((index+virtualArgs)*32, arg.Type, data)
		if arg.Type.T == ArrayTy {
			//如果我们有一个静态数组，比如[3]uint 256，它们被编码为/，就像uint 256，uint 256./这意味着当/我们从现在开始计算索引时，
			// 我们需要添加两个‘虚拟’参数。/数组值嵌套的多个层次嵌套的值也是内联编码的：/[2][3]uint 256：uint 256，uint 256/计算整个数组大小以得到正确的数组大小。
			// 将下一个参数的偏移量./减1，因为正常的索引增量仍在应用。
			// If we have a static array, like [3]uint256, these are coded as
			// just like uint256,uint256,uint256.
			// This means that we need to add two 'virtual' arguments when
			// we count the index from now on.
			//
			// Array values nested multiple levels deep are also encoded inline:
			// [2][3]uint256: uint256,uint256,uint256,uint256,uint256,uint256
			//
			// Calculate the full array size to get the correct offset for the next argument.
			// Decrement it by 1, as the normal index increment is still applied.
			virtualArgs += getArraySize(&arg.Type) - 1
		}
		if err != nil {
			return nil, err
		}
		retval = append(retval, marshalledValue)
	}
	return retval, nil
}

// PackValues执行操作Go格式 - > Hexdata
//这是UnpackValues的语义对立面
// PackValues performs the operation Go format -> Hexdata
// It is the semantic opposite of UnpackValues
func (arguments Arguments) PackValues(args []interface{}) ([]byte, error) {
	return arguments.Pack(args...)
}

// PackValues执行操作Go格式 - > Hexdata
// Pack performs the operation Go format -> Hexdata
func (arguments Arguments) Pack(args ...interface{}) ([]byte, error) {
	// Make sure arguments match up and pack them//确保参数匹配并打包它们
	abiArgs := arguments
	if len(args) != len(abiArgs) {
		return nil, fmt.Errorf("argument count mismatch: %d for %d", len(args), len(abiArgs))
	} //变量输入是附加在打包输出末尾的输出。 这用于字符串和字节类型输入。
	// variable input is the output appended at the end of packed
	// output. This is used for strings and bytes types input.
	var variableInput []byte
	// input offset是打包输出的字节偏移量
	// input offset is the bytes offset for packed output
	inputOffset := 0
	for _, abiArg := range abiArgs {
		if abiArg.Type.T == ArrayTy {
			inputOffset += 32 * abiArg.Type.Size
		} else {
			inputOffset += 32
		}
	}
	var ret []byte
	for i, a := range args {
		input := abiArgs[i]
		// pack the input
		packed, err := input.Type.pack(reflect.ValueOf(a)) //输入参数根据类型打包成byte
		if err != nil {
			return nil, err
		}
		// check for a slice type (string, bytes, slice)//检查切片类型（字符串，字节，切片）
		if input.Type.requiresLengthPrefix() {
			// calculate the offset//计算偏移量
			offset := inputOffset + len(variableInput)
			// set the offset//设置偏移量
			ret = append(ret, packNum(reflect.ValueOf(offset))...)
			//将打包输出附加到变量输入。 变量输入将附加在输入的末尾。
			// Append the packed output to the variable input. The variable input
			// will be appended at the end of the input.
			variableInput = append(variableInput, packed...)
		} else {
			// append the packed value to the input//将打包值附加到输入
			ret = append(ret, packed...)
		}
	}
	// append the variable input at the end of the packed input//在打包输入的末尾追加变量输入
	ret = append(ret, variableInput...)

	return ret, nil
}

// capitalize使字符串的第一个字符为大写，同时从变量名中删除任何前缀下划线。
// capitalise makes the first character of a string upper case, also removing any
// prefixing underscores from the variable names.
func capitalise(input string) string {
	for len(input) > 0 && input[0] == '_' {
		input = input[1:]
	}
	if len(input) == 0 {
		return ""
	}
	return strings.ToUpper(input[:1]) + input[1:]
}

// unpackStruct将每个参数提取到其对应的struct字段中
//unpackStruct extracts each argument into its corresponding struct field
func unpackStruct(value, reflectValue reflect.Value, arg Argument) error {
	name := capitalise(arg.Name)
	typ := value.Type()
	for j := 0; j < typ.NumField(); j++ {
		// TODO read tags: `abi:"fieldName"`
		if typ.Field(j).Name == name {
			if err := set(value.Field(j), reflectValue, arg); err != nil {
				return err
			}
		}
	}
	return nil
}
