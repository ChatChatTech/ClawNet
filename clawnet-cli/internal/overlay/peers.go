package overlay

// DefaultOverlayPeers contains well-known public overlay mesh peers
// that ClawNet connects to via the wire-compatible handshake.
// URI format: "tcp://host:port" or "tls://host:port"
// Selected for high uptime (≥97%) and geographic diversity.
//
// The links subsystem handles per-link exponential backoff automatically.
// 84 initial peers — typically 30-40 will be online at any time.
var DefaultOverlayPeers = []string{
	// === Asia-Pacific (closest to ClawNet USTC nodes) ===
	"tcp://yg-tyo.magicum.net:32334",    // Tokyo, Japan
	"tcp://yg-hkg.magicum.net:32334",    // Hong Kong
	"tcp://yg-sin.magicum.net:23901",    // Singapore
	"tcp://yg-syd.magicum.net:23701",    // Sydney, Australia
	"tcp://yg-mel.magicum.net:23801",    // Melbourne, Australia

	// === Europe - Germany ===
	"tcp://ygg1.mk16.de:1337",
	"tcp://ygg2.mk16.de:1337",
	"tcp://ygg3.mk16.de:1337",
	"tcp://ygg4.mk16.de:1337",
	"tcp://ygg5.mk16.de:1337",
	"tcp://ygg6.mk16.de:1337",
	"tcp://ygg7.mk16.de:1337",
	"tcp://ygg8.mk16.de:1337",
	"tcp://yggpeer.tilde.green:53299",
	"tcp://yggno.de:18226",
	"tcp://ygg.mkg20001.io:80",

	// === Europe - France ===
	"tcp://51.15.204.214:12345",
	"tcp://62.210.85.80:39565",
	"tcp://s2.i2pd.xyz:39565",

	// === Europe - Finland ===
	"tcp://ygg-hel-1.wgos.org:45170",

	// === Europe - Sweden ===
	"tcp://sto01.yggdrasil.hosted-by.skhron.eu:8883",
	"tcp://sysop.link:555",
	"tcp://ygg.ace.ctrl-c.liu.se:9998",

	// === Europe - UK ===
	"tcp://london.sabretruth.org:18473",

	// === Europe - Russia ===
	"tcp://188.225.9.167:18226",
	"tcp://box.paulll.cc:13337",
	"tcp://kem.txlyre.website:1337",
	"tcp://vpn.itrus.su:7991",
	"tcp://srv.itrus.su:7991",
	"tcp://ygg-msk-1.averyan.ru:8363",
	"tcp://yg-vvo.magicum.net:29330",    // Vladivostok
	"tcp://ip4.01.msk.ru.dioni.su:9002", // Moscow
	"tcp://ip4.01.ekb.ru.dioni.su:9002", // Ekaterinburg
	"tcp://ip4.01.tom.ru.dioni.su:9002", // Tomsk
	"tcp://yggdrasil.su:62486",
	"tcp://87.251.77.39:65535",
	"tcp://thatmaidguy.fvds.ru:7991",
	"tcp://cirno.nadeko.net:44441",
	"tcp://itcom.multed.com:7991",
	"tcp://ekb.itrus.su:7991",
	"tcp://146.103.111.53:65535",
	"tcp://146.103.107.222:65535",
	"tcp://89.110.116.167:65535",
	"tcp://212.34.131.160:65535",
	"tcp://88.210.10.78:65535",
	"tcp://5.35.70.181:65535",
	"tcp://109.107.177.127:65535",
	"tcp://ru2.cert.dev:7040",

	// === Europe - Ukraine ===
	"tcp://193.93.119.42:14244",
	"tcp://yggdrasil.sunsung.fun:4442",

	// === Europe - Netherlands ===
	"tcp://185.165.169.234:8880",
	"tcp://45.147.200.202:12402",

	// === Europe - Romania ===
	"tcp://89.44.86.85:65535",

	// === Europe - Other ===
	"tcp://ygg-1.okade.pro:20000",       // Kazakhstan
	"tcp://ygg.nadeko.net:44441",         // Japan/EU
	"tcp://satori.nadeko.net:44441",      // Japan/EU

	// === North America ===
	"tcp://mo.us.ygg.triplebit.org:9000",  // Missouri, US
	"tcp://mn.us.ygg.triplebit.org:9000",  // Minnesota, US
	"tcp://ygg-dc.lxak.net:8879",          // Washington DC, US
	"tcp://leo.node.3dt.net:9002",         // US
	"tcp://ygg-kcmo.incognet.io:8883",     // Kansas City, US
	"tcp://ygg-pa.incognet.io:8883",       // Pennsylvania, US
	"tcp://ygg-wa.incognet.io:8883",       // Washington, US
	"tcp://bode.theender.net:42069",       // US
	"tcp://marisa.nadeko.net:44441",        // US
	"tcp://longseason.1200bps.xyz:13121",  // US
	"tcp://srv.newsdeef.eu:9999",          // US/EU
	"tcp://ip4.nerdvm.mywire.org:8080",    // US
	"tcp://kisume.nadeko.net:44441",        // US

	// === Middle East ===
	"tcp://jed-peer.ygg.sy.sa:8441",      // Saudi Arabia

	// === Africa ===
	"tcp://y.zbin.eu:7743",               // Kenya
	"tcp://yggdrasil.deavmi.assigned.network:2000", // South Africa

	// === Other ===
	"tcp://37.186.113.100:1514",           // Armenia
	"tcp://rendezvous.anton.molyboha.me:50421", // Spain

	// === TLS peers (anti-fingerprinting, transport-layer encryption) ===
	"tls://ygg1.mk16.de:1338",            // Germany
	"tls://ygg2.mk16.de:1338",            // Germany
	"tls://ygg3.mk16.de:1338",            // Germany
	"tls://ygg4.mk16.de:1338",            // Germany
	"tls://ygg5.mk16.de:1338",            // Germany
	"tls://ygg6.mk16.de:1338",            // Germany
	"tls://ygg7.mk16.de:1338",            // Germany
	"tls://ygg8.mk16.de:1338",            // Germany
	"tls://yggno.de:18227",               // Germany
	"tls://vpn.itrus.su:7992",            // Russia
	"tls://srv.itrus.su:7992",            // Russia
	"tls://ekb.itrus.su:7992",            // Russia
	"tls://box.paulll.cc:13338",          // Russia
	"tls://kem.txlyre.website:1338",      // Russia
	"tls://yggdrasil.su:62487",           // Russia
	"tls://thatmaidguy.fvds.ru:7992",     // Russia
	"tls://itcom.multed.com:7992",        // Russia
	"tls://51.15.204.214:54321",          // France
	"tls://s2.i2pd.xyz:39575",            // France
	"tls://185.165.169.234:8881",         // Netherlands
	"tls://45.147.200.202:12403",         // Netherlands
	"tls://ygg-dc.lxak.net:8880",         // Washington DC, US
	"tls://ygg-kcmo.incognet.io:8884",    // Kansas City, US
	"tls://ygg-pa.incognet.io:8884",      // Pennsylvania, US
	"tls://ygg-wa.incognet.io:8884",      // Washington, US
	"tls://longseason.1200bps.xyz:13122", // US

	// Additional TLS peers (Europe)
	"tls://supergay.network:9001",        // Germany
	"tls://de-fsn-1.peer.v4.yggdrasil.chaz6.com:6010", // Germany
	"tls://pl1.servers.devices.cwinfo.net:58243",       // Poland
	"tls://athenobios.trwnh.com:3813",    // Luxembourg
	"tls://54.37.137.221:16920",          // France

	// Additional TLS peers (North America)
	"tls://ca1.servers.devices.cwinfo.net:58243",       // Canada
	"tls://102.223.180.74:993",           // Nigeria
	"tls://lax.yuetau.net:6789",          // Los Angeles, US
	"tls://supergay.network:9002",        // US
}
