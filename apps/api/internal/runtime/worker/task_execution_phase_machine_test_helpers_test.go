package worker

func (m *taskExecutionPhaseMachine) current() taskExecutionPhase {
	if m == nil {
		return ""
	}
	return m.phase
}
