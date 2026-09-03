package rewards

func (l *redemptionAttemptLimiter) trackedUserCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
