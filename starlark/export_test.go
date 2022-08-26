package starlark

func ThreadSafety(thread *Thread) SafetyFlags {
	return thread.requiredSafety
}

const Safe = safe
