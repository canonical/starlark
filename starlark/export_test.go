package starlark

func ThreadSafety(thread *Thread) Safety {
	return thread.requiredSafety
}

func (thread *Thread) SubtractExecutionSteps(delta uint64) {
	thread.steps -= delta
}

const Safe = safetyFlagsLimit - 1

const SafetyFlagsLimit = safetyFlagsLimit

var UniverseSafeties = &universeSafeties

var BytesMethods = bytesMethods
var BytesMethodSafeties = bytesMethodSafeties

var DictMethods = dictMethods
var DictMethodSafeties = dictMethodSafeties

var ListMethods = listMethods
var ListMethodSafeties = listMethodSafeties

var StringMethods = stringMethods
var StringMethodSafeties = stringMethodSafeties

var SetMethods = setMethods
var SetMethodSafeties = setMethodSafeties
