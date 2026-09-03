package common

const MainlandChinaPresentationOptionKey = "MainlandChinaPresentationEnabled"

// IsMainlandChinaPresentationEnabled reports whether public, static marketing
// surfaces should use the mainland-China presentation. It only controls
// presentation and does not change channel availability or model access.
func IsMainlandChinaPresentationEnabled() bool {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()
	return OptionMap[MainlandChinaPresentationOptionKey] == "true"
}
