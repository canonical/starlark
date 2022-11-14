package starlark

func ThreadSafety(thread *Thread) Safety {
	return thread.requiredSafety
}

func (thread *Thread) SubtractExecutionSteps(delta uint64) {
	thread.steps -= delta
}

const Safe = safetyFlagsLimit - 1

const SafetyFlagsLimit = safetyFlagsLimit

var BytesMethods = bytesMethods
var DictMethods = dictMethods
var ListMethods = listMethods
var StringMethods = stringMethods
var SetMethods = setMethods
