package starlark

func ThreadSafety(thread *Thread) Safety {
	return thread.requiredSafety
}

const Safe = safe
