package starlark

func ThreadSafety(thread *Thread) Safety {
	return thread.requiredSafety
}

func (thread *Thread) SubtractExecutionSteps(delta uint64) {
	thread.steps -= delta
}

func (thread *Thread) AllocsLocked() bool {
	ok := thread.allocsLock.TryLock()
	if ok {
		thread.allocsLock.Unlock()
	}
	return !ok
}

const Safe = safetyFlagsLimit - 1

const SafetyFlagsLimit = safetyFlagsLimit
