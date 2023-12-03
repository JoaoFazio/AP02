package configs

import "errors"

var (
	errNoUIDMap   = errors.New("User namespaces enabled, but no uid mappings found.")
	errNoUserMap  = errors.New("User namespaces enabled, but no user mapping found.")
	errNoGIDMap   = errors.New("User namespaces enabled, but no gid mappings found.")
	errNoGroupMap = errors.New("User namespaces enabled, but no group mapping found.")
)

// Please check https://man7.org/linux/man-pages/man2/personality.2.html for const details.
// https://raw.githubusercontent.com/torvalds/linux/master/include/uapi/linux/personality.h
const (
	PerLinux   = 0x0000
	PerLinux32 = 0x0008
)

type LinuxPersonality struct {
	// Domain for the personality
	// can only contain values "LINUX" and "LINUX32"
	Domain int `json:"domain"`
}

// HostUID gets the translated uid for the process on host which could be
// different when user namespaces are enabled.
func (c Config) HostUID(containerId int) (int, error) {
	if c.Namespaces.Contains(NEWUSER) {
		if len(c.UIDMappings) == 0 {
			return -1, errNoUIDMap
		}
		id, found := c.hostIDFromMapping(containerId, c.UIDMappings)
		if !found {
			return -1, errNoUserMap
		}
		return id, nil
	}
	// Return unchanged id.
	return containerId, nil
}

// HostRootUID gets the root uid for the process on host which could be non-zero
// when user namespaces are enabled.
func (c Config) HostRootUID() (int, error) {
	return c.HostUID(0)
}

// HostGID gets the translated gid for the process on host which could be
// different when user namespaces are enabled.
func (c Config) HostGID(containerId int) (int, error) {
	if c.Namespaces.Contains(NEWUSER) {
		if len(c.GIDMappings) == 0 {
			return -1, errNoGIDMap
		}
		id, found := c.hostIDFromMapping(containerId, c.GIDMappings)
		if !found {
			return -1, errNoGroupMap
		}
		return id, nil
	}
	// Return unchanged id.
	return containerId, nil
}

// HostRootGID gets the root gid for the process on host which could be non-zero
// when user namespaces are enabled.
func (c Config) HostRootGID() (int, error) {
	return c.HostGID(0)
}

// Utility function that gets a host ID for a container ID from user namespace map
// if that ID is present in the map.
func (c Config) hostIDFromMapping(containerID int, uMap []IDMap) (int, bool) {
	for _, m := range uMap {
		if (containerID >= m.ContainerID) && (containerID <= (m.ContainerID + m.Size - 1)) {
			hostID := m.HostID + (containerID - m.ContainerID)
			return hostID, true
		}
	}
	return -1, false
}
