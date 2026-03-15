package matrix

import "time"

// DefaultHomeservers is the built-in list of public Matrix homeservers.
// Multiple homeservers provide redundancy: if one is blocked or down,
// others still allow peer discovery. Room messages federate across
// homeservers, so a node on matrix.org and a node on envs.net both
// see the same #clawnet-discovery room.
//
// Ordered by likelihood of accepting open registration.
// The discovery loop probes all, sorts by latency, and connects to the first 3 that succeed.
var DefaultHomeservers = []string{
	// Tier 1: community-run, commonly open registration
	"https://matrix.im",
	"https://nitro.chat",
	"https://catgirl.cloud",
	"https://halogen.city",
	"https://grin.hu",
	"https://pimux.de",
	"https://aguiarvieira.pt",
	"https://anonymousland.org",
	"https://matrix.sp-codes.de",
	"https://kyoto-server.org",
	"https://fosscord.com",
	"https://aria-net.org",
	"https://the-apothecary.club",
	"https://synapse.zelcon.net",
	"https://chat.usr.nz",
	// Tier 2: larger community servers
	"https://matrix.org",
	"https://envs.net",
	"https://tchncs.de",
	"https://converser.eu",
	"https://matrix.nohost.network",
	"https://chat.mistli.net",
	"https://matrix.radical.directory",
	"https://fairydust.space",
	"https://sibnsk.net",
	"https://matrix.phcn.de",
	// Tier 3: institutional / restricted
	"https://mozilla.org",
	"https://kde.org",
	"https://matrix.sibnsk.net",
	"https://perthchat.org",
	"https://matrix.fdn.fr",
	"https://buyvm.net",
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
