package db

// ValidEffects defines the allowed rule effect values.
var ValidEffects = map[string]bool{
	"always_keep": true, "prefer_keep": true, "lean_keep": true,
	"lean_remove": true, "prefer_remove": true, "always_remove": true,
}

// ValidExecutionModes defines the allowed engine execution modes.
var ValidExecutionModes = map[string]bool{
	"dry-run": true, "approval": true, "auto": true,
}

// ValidTiebreakerMethods defines the allowed tiebreaker sort methods.
var ValidTiebreakerMethods = map[string]bool{
	"size_desc": true, "size_asc": true, "name_asc": true,
	"oldest_first": true, "newest_first": true,
}

// ValidLogLevels defines the allowed log level values.
var ValidLogLevels = map[string]bool{
	"debug": true, "info": true, "warn": true, "error": true,
}

// ValidIntegrationTypes defines the allowed integration type values.
var ValidIntegrationTypes = map[string]bool{
	"plex": true, "sonarr": true, "radarr": true, "lidarr": true,
	"readarr": true, "tautulli": true, "overseerr": true,
	"jellyfin": true, "emby": true,
}

// ValidNotificationChannelTypes defines the allowed notification channel types.
// In-app notifications are always-on and not user-configurable, so "inapp" is
// intentionally excluded here.
var ValidNotificationChannelTypes = map[string]bool{
	"discord": true, "slack": true,
}
