package starlark

func ThreadSafety(thread *Thread) Safety {
	return thread.requiredSafety
}

const Safe = safetyFlagsLimit - 1

const SafetyFlagsLimit = safetyFlagsLimit

var BytesMethods = bytesMethods
var DictMethods = dictMethods
var ListMethods = listMethods
var StringMethods = stringMethods
var SetMethods = setMethods
