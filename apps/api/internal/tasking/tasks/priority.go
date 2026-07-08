package tasks

const (
	TaskPriorityInteractive = 1
	TaskPriorityDefault     = 2
	TaskPriorityLowest      = 4
)

func EffectiveTaskPriority(priority int) int {
	if priority == 0 {
		return TaskPriorityDefault
	}
	return priority
}

func IsValidTaskPriority(priority int) bool {
	return priority >= TaskPriorityInteractive && priority <= TaskPriorityLowest
}
