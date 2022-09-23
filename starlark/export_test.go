package starlark

func ThreadSafety(thread *Thread) Safety {
	return thread.requiredSafety
}

const Safe = safetyFlagsLimit - 1

const SafetyFlagsLimit = safetyFlagsLimit
