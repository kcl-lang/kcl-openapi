package generator

// typeMapping contains a mapping of type name to kcl type
var typeMapping = map[string]string{
	// Standard formats with native, straightforward, mapping
	str:     "str",
	boolean: "bool",
	integer: "int",
	number:  "float",
	// TODO: OpenAPI spec does not support multi-types.
	//  k8s.json set type=string & format=int-or-str to support int | str multi-type.
	//  hack specially for int-or-string.
	intOrStr: "int | str",
}

// formatMapping contains a type-specific version of mapping of format to kcl type
var formatMapping = map[string]map[string]string{
	number: {
		"float": "float",
		"int":   "int",
		"int8":  "int",
		"int16": "int",
		"int32": "int",
	},
	integer: {
		"int":   "int",
		"int8":  "int",
		"int16": "int",
		"int32": "int",
	},
}
