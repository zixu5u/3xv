package tgbot

import (
	"sync"
	"time"

	"github.com/zixu5u/3xv/v3/internal/logger"
)

var (
	// schedulerMutex protects concurrent access to scheduler variables
	schedulerMutex sync.Mutex
	// reportTicker stores the ticker for periodic reports
	reportTicker *time.Ticker
	// reportDone is the done channel for the report goroutine
	reportDone chan struct{}
	// reportWG waits for the report goroutine to finish
	reportWG sync.WaitGroup
	// scheduledTime stores the scheduled time for daily reports (e.g., "08:00")
	scheduledTime string
)

// StartScheduler initializes and starts the daily scheduled report.
func (t *Tgbot) StartScheduler(scheduleTime string) {
	schedulerMutex.Lock()
	defer schedulerMutex.Unlock()

	// Stop any existing scheduler
	t.stopSchedulerLocked()

	if scheduleTime == "" {
		logger.Info("Schedule time is empty, scheduler not started")
		return
	}

	scheduledTime = scheduleTime
	reportDone = make(chan struct{})

	// Calculate the initial delay until the next scheduled time
	now := time.Now()
	nextRun := t.calculateNextRunTime(now, scheduleTime)
	initialDelay := nextRun.Sub(now)

	logger.Infof("Telegram bot scheduler started. Next scheduled report at %s (in %v)", nextRun.Format("2006-01-02 15:04:05"), initialDelay)

	reportWG.Add(1)
	go t.reportScheduler(initialDelay)
}

// StopScheduler stops the daily scheduled report.
func (t *Tgbot) StopScheduler() {
	schedulerMutex.Lock()
	defer schedulerMutex.Unlock()
	t.stopSchedulerLocked()
}

// stopSchedulerLocked stops the scheduler (assumes mutex is already held)
func (t *Tgbot) stopSchedulerLocked() {
	if reportDone != nil {
		close(reportDone)
		reportDone = nil
		reportWG.Wait()
		logger.Info("Telegram bot scheduler stopped")
	}
	if reportTicker != nil {
		reportTicker.Stop()
		reportTicker = nil
	}
}

// reportScheduler handles the scheduled report logic.
// It waits for the initial delay, then sends a report and sets up a daily ticker.
func (t *Tgbot) reportScheduler(initialDelay time.Duration) {
	defer reportWG.Done()

	// Wait for the initial delay or cancellation
	select {
	case <-reportDone:
		return
	case <-time.After(initialDelay):
		// Initial delay complete, send the first report
	}

	// Send the first report immediately
	t.SendReport()

	// Set up a daily ticker
	schedulerMutex.Lock()
	reportTicker = time.NewTicker(24 * time.Hour)
	schedulerMutex.Unlock()

	// Wait for the next reports
	for {
		select {
		case <-reportDone:
			return
		case <-reportTicker.C:
			// Verify that the current time matches the scheduled time
			// (to handle cases where the system clock changes)
			now := time.Now()
			nextRun := t.calculateNextRunTime(now, scheduledTime)
			timeDiff := nextRun.Sub(now)

			// If the difference is significant (more than 1 hour), recalculate
			if timeDiff > 1*time.Hour || timeDiff < -1*time.Hour {
				logger.Warningf("Detected significant time difference for scheduled report. Recalculating...")
				schedulerMutex.Lock()
				if reportTicker != nil {
					reportTicker.Stop()
					reportTicker = time.NewTicker(24 * time.Hour)
				}
				schedulerMutex.Unlock()
			}

			t.SendReport()
		}
	}
}

// calculateNextRunTime calculates the next run time based on the schedule (e.g., "08:00").
func (t *Tgbot) calculateNextRunTime(now time.Time, scheduleStr string) time.Time {
	// Parse the schedule string (format: "HH:MM")
	scheduledHour, scheduledMin := t.parseScheduleTime(scheduleStr)

	// Create a time for today at the scheduled time
	nextRun := now.Add(0)
	nextRun = time.Date(nextRun.Year(), nextRun.Month(), nextRun.Day(), scheduledHour, scheduledMin, 0, 0, nextRun.Location())

	// If the scheduled time has already passed today, move to tomorrow
	if nextRun.Before(now) {
		nextRun = nextRun.Add(24 * time.Hour)
	}

	return nextRun
}

// parseScheduleTime parses the schedule string (format: "HH:MM") and returns hour and minute.
func (t *Tgbot) parseScheduleTime(scheduleStr string) (int, int) {
	// Default to 08:00 if parsing fails
	hour, min := 8, 0

	if len(scheduleStr) >= 5 {
		_, err := parseTime(scheduleStr)
		if err == nil {
			// Extract hour and minute from schedule string
			parts := parseTimeComponents(scheduleStr)
			if len(parts) >= 2 {
				// Safe to convert, as parseTime already validated
				var h, m int
				_, _ = parseIntComponent(parts[0], &h)
				_, _ = parseIntComponent(parts[1], &m)
				hour, min = h, m
			}
		}
	}

	return hour, min
}

// parseTime validates a time string in HH:MM format
func parseTime(timeStr string) (time.Time, error) {
	return time.Parse("15:04", timeStr)
}

// parseTimeComponents splits a time string into components
func parseTimeComponents(timeStr string) []string {
	var components []string
	var current string
	for _, ch := range timeStr {
		if ch == ':' || ch == '-' {
			if current != "" {
				components = append(components, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		components = append(components, current)
	}
	return components
}

// parseIntComponent safely parses an integer component
func parseIntComponent(str string, value *int) (bool, error) {
	var result int
	for _, ch := range str {
		if ch >= '0' && ch <= '9' {
			result = result*10 + int(ch-'0')
		} else {
			return false, nil
		}
	}
	*value = result
	return true, nil
}
