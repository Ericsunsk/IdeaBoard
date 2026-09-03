package queue

func (p taskAssignmentPolicy) decideCandidatePool(taskID string, candidates []int64) taskAssignmentDecision {
	if len(candidates) == 0 {
		return taskAssignmentDecision{done: true, assignment: p.noCapacity(taskID)}
	}
	return taskAssignmentDecision{}
}
