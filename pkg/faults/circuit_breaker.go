package faults

import (
	"time"

	"github.com/arun0009/go-resilience-mock/pkg/config"
)

// checkCircuitBreaker returns true if request is allowed, false if blocked (Open state)
func checkCircuitBreaker(s *config.Scenario) bool {
	s.CBState.Mutex.Lock()
	defer s.CBState.Mutex.Unlock()

	if s.CBState.State == "open" {
		if time.Since(s.CBState.LastTransition) > s.CircuitBreaker.Timeout {
			s.CBState.State = "half-open"
			s.CBState.LastTransition = time.Now()
			// Allow this request to pass through to test recovery
			return true
		}
		return false
	}
	return true
}

// updateCircuitBreaker updates the state based on request outcome
func updateCircuitBreaker(s *config.Scenario, success bool) {
	s.CBState.Mutex.Lock()
	defer s.CBState.Mutex.Unlock()

	if s.CBState.State == "open" {
		// Should not happen if checkCircuitBreaker blocked it, but if it was half-open:
		// Wait, if it was open and we are here, it means it transitioned to half-open in check?
		// No, checkCircuitBreaker changes state.
		return
	}

	if s.CBState.State == "half-open" {
		if success {
			s.CBState.Successes++
			if s.CBState.Successes >= s.CircuitBreaker.SuccessThreshold {
				s.CBState.State = "closed"
				s.CBState.Failures = 0
				s.CBState.Successes = 0
				s.CBState.LastTransition = time.Now()
			}
		} else {
			s.CBState.State = "open"
			s.CBState.LastTransition = time.Now()
		}
		return
	}

	// Closed State
	if !success {
		s.CBState.Failures++
		s.CBState.LastFailure = time.Now()
		if s.CBState.Failures >= s.CircuitBreaker.FailureThreshold {
			s.CBState.State = "open"
			s.CBState.LastTransition = time.Now()
		}
	} else {
		// Reset failures on success in closed state?
		// Usually yes, or use a sliding window. Simple counter reset for now.
		s.CBState.Failures = 0
	}
}
