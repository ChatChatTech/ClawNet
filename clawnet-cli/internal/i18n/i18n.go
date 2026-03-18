package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/geo"
	"github.com/ChatChatTech/ClawNet/clawnet-cli/internal/p2p"
)

// Supported languages.
const (
	LangEN = "en"
	LangZH = "zh"
	LangJA = "ja"
	LangKO = "ko"
	LangES = "es"
	LangFR = "fr"
	LangDE = "de"
	LangPT = "pt"
	LangRU = "ru"
)

var (
	currentLang = LangEN
	mu          sync.RWMutex
	// locales maps language code → key → translated string.
	locales = map[string]map[string]string{}
)

// Register adds or merges translations for a language.
func Register(lang string, msgs map[string]string) {
	if locales[lang] == nil {
		locales[lang] = make(map[string]string, len(msgs))
	}
	for k, v := range msgs {
		locales[lang][k] = v
	}
}

// SetLang sets the active language.
func SetLang(lang string) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := locales[lang]; ok {
		currentLang = lang
	}
}

// Lang returns the active language code.
func Lang() string {
	mu.RLock()
	defer mu.RUnlock()
	return currentLang
}

// T returns the translated string for key in the current language.
// Falls back to English, then returns the key itself.
func T(key string) string {
	mu.RLock()
	lang := currentLang
	mu.RUnlock()

	if m, ok := locales[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	// Fallback to English
	if lang != LangEN {
		if m, ok := locales[LangEN]; ok {
			if s, ok := m[key]; ok {
				return s
			}
		}
	}
	return key
}

// Tf returns a formatted translated string (like fmt.Sprintf).
func Tf(key string, args ...any) string {
	return fmt.Sprintf(T(key), args...)
}

// countryToLang maps ISO 3166-1 alpha-2 country codes to language codes.
var countryToLang = map[string]string{
	// Chinese
	"CN": LangZH, "TW": LangZH, "HK": LangZH, "MO": LangZH, "SG": LangZH,
	// Japanese
	"JP": LangJA,
	// Korean
	"KR": LangKO,
	// Spanish
	"ES": LangES, "MX": LangES, "AR": LangES, "CO": LangES, "CL": LangES,
	"PE": LangES, "VE": LangES, "EC": LangES, "GT": LangES, "CU": LangES,
	"BO": LangES, "HN": LangES, "PY": LangES, "SV": LangES, "NI": LangES,
	"CR": LangES, "PA": LangES, "UY": LangES, "PR": LangES,
	// Portuguese
	"BR": LangPT, "PT": LangPT,
	// French
	"FR": LangFR, "BE": LangFR, "CA": LangFR,
	// German
	"DE": LangDE, "AT": LangDE, "CH": LangDE,
	// Russian
	"RU": LangRU,
	// English (explicit — also the default)
	"US": LangEN, "GB": LangEN, "AU": LangEN, "NZ": LangEN, "IE": LangEN,
	"IN": LangEN, "PH": LangEN, "ZA": LangEN,
}

// LangForCountry returns the most likely language for a country code.
func LangForCountry(country string) string {
	if l, ok := countryToLang[strings.ToUpper(country)]; ok {
		return l
	}
	return LangEN
}

// Init detects and sets the language. Priority:
//  1. CLAWNET_LANG environment variable
//  2. Cached language file (~/.openclaw/clawnet/lang)
//  3. System locale (LANG / LC_ALL)
//  4. IP geolocation via STUN + ip2location (cached on success)
//  5. Default: English
func Init(dataDir string) {
	// 1. Env override
	if lang := os.Getenv("CLAWNET_LANG"); lang != "" {
		SetLang(lang)
		return
	}

	// 2. Cached from previous detection
	cachePath := filepath.Join(dataDir, "lang")
	if data, err := os.ReadFile(cachePath); err == nil {
		lang := strings.TrimSpace(string(data))
		if lang != "" {
			SetLang(lang)
			return
		}
	}

	// 3. System locale hints
	if lang := langFromSystemLocale(); lang != "" {
		SetLang(lang)
		writeCache(cachePath, lang)
		return
	}

	// 4. Geo-detect (STUN + ip2location) — 3s timeout
	if lang := detectFromGeo(dataDir); lang != "" {
		SetLang(lang)
		writeCache(cachePath, lang)
		return
	}

	// 5. Default
	SetLang(LangEN)
}

// langFromSystemLocale checks LANG / LC_ALL environment variables.
func langFromSystemLocale() string {
	for _, env := range []string{"LC_ALL", "LANG", "LANGUAGE"} {
		val := os.Getenv(env)
		if val == "" || val == "C" || val == "POSIX" {
			continue
		}
		// Parse locale like "zh_CN.UTF-8" → "zh"
		val = strings.Split(val, ".")[0] // strip encoding
		val = strings.Split(val, "@")[0] // strip modifier
		lang := strings.Split(val, "_")[0]
		lang = strings.ToLower(lang)
		if _, ok := locales[lang]; ok {
			return lang
		}
	}
	return ""
}

// detectFromGeo uses STUN to find external IP, then geo-locates it.
func detectFromGeo(dataDir string) string {
	type result struct {
		ip string
	}
	ch := make(chan result, 1)
	go func() {
		ip := p2p.DetectExternalIP()
		ch <- result{ip: ip}
	}()

	select {
	case r := <-ch:
		if r.ip == "" {
			return ""
		}
		loc, err := geo.NewLocator(dataDir)
		if err != nil {
			return ""
		}
		defer loc.Close()
		gi := loc.Lookup(r.ip)
		if gi == nil || gi.Country == "" {
			return ""
		}
		return LangForCountry(gi.Country)
	case <-time.After(3 * time.Second):
		return ""
	}
}

func writeCache(path, lang string) {
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte(lang+"\n"), 0600)
}
