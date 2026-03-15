package matrix

import "time"

// DefaultHomeservers is the built-in list of public Matrix homeservers.
// Multiple homeservers provide redundancy: if one is blocked or down,
// others still allow peer discovery. Room messages federate across
// homeservers, so a node on matrix.org and a node on envs.net both
// see the same #clawnet-discovery room.
var DefaultHomeservers = []string{
	"https://matrix.org",
	"https://envs.net",
	"https://tchncs.de",
	"https://mozilla.org",
	"https://converser.eu",
}

const (
	// DiscoveryRoom is the room alias used for peer discovery.
	// Each homeserver will have its own version, federated together.
	DiscoveryRoomAlias = "#clawnet-discovery"

	// DefaultAnnounceInterval is how often we broadcast our multiaddrs.
	DefaultAnnounceInterval = 5 * time.Minute

	// SyncTimeoutMs is the long-poll timeout for /sync requests.
	SyncTimeoutMs = 30000

	// UsernamePrefix is prepended to the peer ID fragment for the Matrix username.
	UsernamePrefix = "clawnet_"
)
