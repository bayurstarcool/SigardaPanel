package store

import (
	"context"
	"log"
)

// Notify creates an in-app notification for a user.
// Logs errors but never fails — notifications are fire-and-forget.
func (s *Store) Notify(ctx context.Context, userID int64, notifType, title, message, severity, entityType string, entityID int64) {
	n := &Notification{
		UserID:     userID,
		Type:       notifType,
		Title:      title,
		Message:    message,
		Severity:   severity,
		EntityType: entityType,
		EntityID:   entityID,
	}
	if err := s.Notifications.Create(ctx, n); err != nil {
		log.Printf("[notification] failed to create: user=%d type=%s err=%v", userID, notifType, err)
	}
}

// NotifyAdmins sends a notification to all admin/super_admin users.
func (s *Store) NotifyAdmins(ctx context.Context, notifType, title, message, severity, entityType string, entityID int64) {
	admins, err := s.Users.List(ctx, UserListFilter{})
	if err != nil {
		log.Printf("[notification] failed to list admins: %v", err)
		return
	}
	for _, u := range admins {
		if u.Role == "admin" || u.Role == "super_admin" {
			s.Notify(ctx, u.ID, notifType, title, message, severity, entityType, entityID)
		}
	}
}

// NotifySiteUsers sends a notification to all users with access to a site.
// Falls back to notifying admins if user lookup fails.
func (s *Store) NotifySiteUsers(ctx context.Context, siteID int64, notifType, title, message, severity, entityType string, entityID int64) {
	userSites, err := s.UserSites.ListBySite(ctx, siteID)
	if err != nil || len(userSites) == 0 {
		s.NotifyAdmins(ctx, notifType, title, message, severity, entityType, entityID)
		return
	}
	for _, us := range userSites {
		s.Notify(ctx, us.UserID, notifType, title, message, severity, entityType, entityID)
	}
}
