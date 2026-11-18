package app

import (
	"log"
	"time"
)

func (s *Service) recordActivity(userID string, event string, count int) {
	if err := s.repo.RecordUserActivity(userID, event, count, time.Now()); err != nil {
		log.Printf("record user activity failed: event=%s count=%d error=%v", event, count, err)
	}
}
